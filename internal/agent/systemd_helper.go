//go:build linux

package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	systemddbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/godbus/dbus/v5"

	"github.com/retransmit/kite/internal/protocol"
)

// Scope says which manager a unit came from.
type Scope string

const (
	ScopeSystem Scope = "system"
	ScopeUser   Scope = "user"
)

const (
	// defaultSystemdTimeout bounds connecting to a bus and every single D-Bus
	// call, so a wedged systemd cannot stall the agent.
	defaultSystemdTimeout = 3 * time.Second

	serviceSuffix = ".service"

	// These bound what one manager may contribute. A user manager's socket
	// sits in a directory its user owns, so it can say whatever it likes;
	// without bounds one user could return enough to push every sample past
	// the hub's 1 MiB limit, which would take the whole machine offline.
	// maxServicesPerBus caps the units, maxDescription a description in
	// characters, maxState each state field in characters. A name longer than
	// maxUnitName (systemd's own limit) did not come from systemd, so that
	// unit is skipped rather than shortened into a different name.
	maxServicesPerBus = 512
	maxDescription    = 200
	maxUnitName       = 256
	maxState          = 32
)

// systemdBus is one systemd manager to read.
type systemdBus struct {
	Scope Scope
	Owner string // login name for a user manager
	Path  string // socket path
	// UID is the uid that must own the socket, or -1 for the system bus, which
	// is owned by root and reachable by everyone.
	UID int
	// Private is set for systemd's own socket, which has no bus daemon behind
	// it: the D-Bus Hello handshake does not apply.
	Private bool
}

// systemdConn is the part of SystemdHelper's connection this file uses. It
// exists so the queries can be tested without a running systemd.
type systemdConn interface {
	ListUnitsContext(ctx context.Context) ([]systemddbus.UnitStatus, error)
	Close()
}

// SystemdHelper answers questions about one systemd manager. It is read-only:
// it never starts, stops, enables or disables anything.
type SystemdHelper struct {
	conn    systemdConn
	path    string
	timeout time.Duration
}

// openSystemdBus connects to the manager described by b. Close the helper when
// done with it.
func openSystemdBus(b systemdBus, timeout time.Duration) (*SystemdHelper, error) {
	if err := checkBusSocket(b.Path, b.UID); err != nil {
		return nil, err
	}
	conn, err := dialBus(b, timeout)
	if err != nil {
		return nil, err
	}
	return &SystemdHelper{conn: conn, path: b.Path, timeout: timeout}, nil
}

// checkBusSocket verifies that path is a socket rather than a symlink, and that
// it belongs to the uid expected for that directory. The agent runs as root and
// takes this path from a directory its user controls: refusing symlinks stops
// that user from pointing root at any other socket, and the owner check stops a
// socket owned by someone else from being reported as this user's manager. It
// cannot stop the user from serving a fake manager of their own, which is why
// what any manager returns is bounded (see ListServices and collect).
func checkBusSocket(path string, wantUID int) error {
	// Lstat, not Stat: Stat follows the link and would defeat the check.
	fi, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("systemd socket %s: %w", path, err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("systemd socket %s: not a socket", path)
	}
	if wantUID < 0 {
		return nil
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("systemd socket %s: cannot read owner", path)
	}
	if int(st.Uid) != wantUID {
		return fmt.Errorf("systemd socket %s: owned by uid %d, expected %d", path, st.Uid, wantUID)
	}
	return nil
}

// dialBus opens both connections the library needs, one for calls and one for
// signals. godbus has no timeouts of its own: a socket that accepts and never
// answers blocks auth, Hello, or the AddMatch call the library makes while
// setting up, forever. So every socket carries a deadline until setup is done,
// and a setup that ran out of time is closed and reported rather than left
// blocked in the background, where each refresh would leak another goroutine
// and file descriptor. A timeout of zero or less means no deadline.
func dialBus(b systemdBus, timeout time.Duration) (*systemddbus.Conn, error) {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	var socks []net.Conn
	closeAll := func() {
		for _, s := range socks {
			s.Close()
		}
	}
	uid := strconv.Itoa(os.Getuid())
	conn, err := systemddbus.NewConnection(func() (*dbus.Conn, error) {
		d := net.Dialer{Deadline: deadline}
		sock, err := d.Dial("unix", b.Path)
		if err != nil {
			return nil, err
		}
		socks = append(socks, sock)
		sock.SetDeadline(deadline)
		conn, err := dbus.NewConn(sock)
		if err != nil {
			return nil, err
		}
		// EXTERNAL as the agent's own uid. Connect cannot be used: it insists
		// on Hello, which only a bus daemon answers, not a private socket.
		if err := conn.Auth([]dbus.Auth{dbus.AuthExternal(uid)}); err != nil {
			return nil, err
		}
		if !b.Private {
			if err := conn.Hello(); err != nil {
				return nil, err
			}
		}
		return conn, nil
	})
	if err != nil {
		closeAll()
		return nil, fmt.Errorf("systemd %s: %w", b.Path, err)
	}
	// The library ignores the result of its AddMatch call, so a manager that
	// stopped answering during setup still comes back as a connection.
	if !deadline.IsZero() && time.Now().After(deadline) {
		conn.Close()
		closeAll()
		return nil, fmt.Errorf("systemd %s: timed out after %s", b.Path, timeout)
	}
	for _, s := range socks {
		s.SetDeadline(time.Time{})
	}
	return conn, nil
}

// Close releases the bus connection.
func (h *SystemdHelper) Close() error {
	if h == nil || h.conn == nil {
		return nil
	}
	h.conn.Close()
	return nil
}

// ListServices returns the loaded .service units, sorted by name and capped at
// maxServicesPerBus. Other unit types (sockets, timers, mounts, ...) are left
// out. The bool reports that the manager had more units than were taken.
func (h *SystemdHelper) ListServices(ctx context.Context) ([]protocol.Service, bool, error) {
	ctx, cancel := h.withTimeout(ctx)
	defer cancel()
	units, err := h.conn.ListUnitsContext(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("systemd %s: list units: %w", h.path, err)
	}
	out := make([]protocol.Service, 0, len(units))
	truncated := false
	for _, u := range units {
		if !strings.HasSuffix(u.Name, serviceSuffix) || len(u.Name) > maxUnitName {
			continue
		}
		if len(out) >= maxServicesPerBus {
			truncated = true
			break
		}
		out = append(out, protocol.Service{
			Name:        u.Name,
			Description: truncate(u.Description, maxDescription),
			LoadState:   truncate(u.LoadState, maxState),
			ActiveState: truncate(u.ActiveState, maxState),
			SubState:    truncate(u.SubState, maxState),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, truncated, nil
}

// withTimeout gives one D-Bus call its own budget, so a caller's long-lived ctx
// cannot leave a query hanging.
func (h *SystemdHelper) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if h.timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, h.timeout)
}

// truncate shortens s to at most max characters without splitting a rune.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

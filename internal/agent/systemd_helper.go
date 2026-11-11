//go:build linux

package agent

import (
	"context"
	"fmt"
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

	// maxServicesPerBus bounds how many units one manager may contribute, and
	// maxDescription how long one unit's description may be. A session bus
	// socket sits in a directory its user owns, so its manager can say
	// whatever it likes; without bounds one user could return enough units to
	// push every sample past the hub's 1 MiB limit, which would take the whole
	// machine offline.
	maxServicesPerBus = 512
	maxDescription    = 200
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
	conn, err := dialWithin(b.Path, timeout, func() (*systemddbus.Conn, error) {
		return dialBus(b)
	})
	if err != nil {
		return nil, err
	}
	return &SystemdHelper{conn: conn, path: b.Path, timeout: timeout}, nil
}

// checkBusSocket verifies that path is a socket rather than a symlink, and that
// it belongs to the uid expected for that directory. The agent runs as root and
// takes this path from a directory its user controls, so both checks matter:
// a symlink would redirect root at any file, and a socket planted by the user
// would be their own server answering for the user manager.
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

// dialBus opens both connections the library needs. The uid is hardcoded as a
// number to avoid a username lookup.
func dialBus(b systemdBus) (*systemddbus.Conn, error) {
	uid := strconv.Itoa(os.Getuid())
	return systemddbus.NewConnection(func() (*dbus.Conn, error) {
		// Dial performs no handshake, so auth is done here. Connect cannot be
		// used for a private socket because it insists on Hello.
		conn, err := dbus.Dial("unix:path=" + b.Path)
		if err != nil {
			return nil, err
		}
		if err := conn.Auth([]dbus.Auth{dbus.AuthExternal(uid)}); err != nil {
			conn.Close()
			return nil, err
		}
		if !b.Private {
			if err := conn.Hello(); err != nil {
				conn.Close()
				return nil, err
			}
		}
		return conn, nil
	})
}

// dialWithin calls open but gives up after timeout. The connection cannot be
// created with a deadline context: the library keeps that context for the
// connection's lifetime and closes the connection when it is cancelled. So open
// gets a context that lives as long as the connection does, and the bound is
// applied here instead. A connection that arrives after the deadline is closed
// rather than leaked.
func dialWithin(what string, timeout time.Duration, open func() (*systemddbus.Conn, error)) (*systemddbus.Conn, error) {
	type dialed struct {
		conn *systemddbus.Conn
		err  error
	}
	ch := make(chan dialed, 1)
	go func() {
		conn, err := open()
		ch <- dialed{conn, err}
	}()
	if timeout <= 0 {
		r := <-ch
		return r.conn, r.err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-ch:
		return r.conn, r.err
	case <-timer.C:
		go func() {
			if r := <-ch; r.conn != nil {
				r.conn.Close()
			}
		}()
		return nil, fmt.Errorf("systemd %s: timed out after %s", what, timeout)
	}
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
		if !strings.HasSuffix(u.Name, serviceSuffix) {
			continue
		}
		if len(out) >= maxServicesPerBus {
			truncated = true
			break
		}
		out = append(out, protocol.Service{
			Name:        u.Name,
			Description: truncate(u.Description, maxDescription),
			LoadState:   u.LoadState,
			ActiveState: u.ActiveState,
			SubState:    u.SubState,
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

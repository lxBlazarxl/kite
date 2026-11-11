//go:build linux

package agent

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

const (
	// systemdEvery is how often the service list is refreshed. A typical host
	// has well over a hundred units and each refresh is a D-Bus round trip, so
	// this is far cheaper than querying it on every 5 s sample.
	systemdEvery = 15 * time.Second

	// maxServicesTotal bounds the whole section, so the sum over many logged-in
	// users cannot grow without limit either.
	maxServicesTotal = 1024

	systemBusSocket = "/run/dbus/system_bus_socket"
	userRunDir      = "/run/user"
)

// systemdLister is the part of SystemdHelper the collector needs.
type systemdLister interface {
	ListServices(ctx context.Context) ([]protocol.Service, bool, error)
	Close() error
}

// systemdCollector reports systemd service state. The list comes from a
// background refresh so a slow bus never delays a sample; only aggregate counts
// become history.
type systemdCollector struct {
	cache *cached[*protocol.Systemd]
	buses func() []systemdBus
	open  func(systemdBus, time.Duration) (systemdLister, error)

	// pool keeps one connection per manager across refreshes. Reconnecting
	// every 15 s would open two sockets per bus each time, which the library
	// needs for calls and signals separately. Only collect touches this.
	pool map[string]systemdLister

	mu   sync.Mutex
	warn string
}

// NewSystemd reports systemd service state, from the system manager and from
// every logged-in user's manager. hostRoot is where the host's / is mounted when
// the agent runs in a container, so the host's buses are read rather than the
// container's.
//
// It always returns a collector, even where systemd is missing: the reason then
// appears in Unavailable for that sample, and the machine starts reporting as
// soon as a manager answers, instead of staying off until the agent restarts.
func NewSystemd(hostRoot string) Collector {
	buses := func() []systemdBus { return systemdBuses(hostRoot) }
	// Wrapped so a nil helper on error does not become a non-nil interface.
	open := func(b systemdBus, timeout time.Duration) (systemdLister, error) {
		return openSystemdBus(b, timeout)
	}
	c := &systemdCollector{buses: buses, open: open, pool: map[string]systemdLister{}}
	cache := newAsyncCached(systemdEvery, c.collect)
	// An expired value is dropped rather than shown forever: if systemd stops
	// answering, the section disappears instead of reporting a stale count.
	cache.maxAge = 3 * systemdEvery
	c.cache = cache
	return c
}

// systemdBuses lists the managers to read: the system bus, then one manager per
// logged-in user (/run/user/<uid>).
func systemdBuses(hostRoot string) []systemdBus {
	// The system bus socket is world-writable and its daemon allows any uid, so
	// there is no owner to check.
	out := []systemdBus{{Scope: ScopeSystem, Path: filepath.Join(hostRoot, systemBusSocket), UID: -1}}
	dirs, _ := filepath.Glob(filepath.Join(hostRoot, userRunDir, "*"))
	for _, dir := range dirs {
		uid, err := strconv.Atoi(filepath.Base(dir))
		if err != nil {
			continue
		}
		if b, ok := userSystemdBus(dir, uid); ok {
			out = append(out, b)
		}
	}
	return out
}

// userSystemdBus picks how to reach one user's manager. systemd's own socket is
// preferred: a session bus running the default dbus-daemon policy admits only
// the uid that owns it, so an agent running as root is refused, while the
// private socket has no bus daemon to apply that policy. The session bus stays
// as a fallback for systems without the private socket.
func userSystemdBus(dir string, uid int) (systemdBus, bool) {
	owner := userName(strconv.Itoa(uid))
	if p := filepath.Join(dir, "systemd", "private"); isSocket(p) {
		return systemdBus{Scope: ScopeUser, Owner: owner, Path: p, UID: uid, Private: true}, true
	}
	if p := filepath.Join(dir, "bus"); isSocket(p) {
		return systemdBus{Scope: ScopeUser, Owner: owner, Path: p, UID: uid}, true
	}
	return systemdBus{}, false
}

// busLabel names a manager in a message: the user for a user manager, the scope
// otherwise.
func busLabel(b systemdBus) string {
	if b.Owner != "" {
		return b.Owner
	}
	return string(b.Scope)
}

// isSocket reports whether path is a socket, without following a symlink.
func isSocket(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSocket != 0
}

// userName turns a uid into a login name, falling back to the uid when the
// lookup fails (the user may be gone from the password database already).
func userName(uid string) string {
	if u, err := user.LookupId(uid); err == nil && u.Username != "" {
		return u.Username
	}
	return uid
}

// collect reads every manager and merges the units. A user's manager is
// optional: they may log out mid-refresh, which is not worth failing over, but
// it is recorded so a machine that only ever reports system units says why. The
// system bus failing is an error only when nothing at all was read.
func (c *systemdCollector) collect() (*protocol.Systemd, error) {
	out := &protocol.Systemd{Services: []protocol.Service{}}
	var problems []string
	var systemErr error
	seen := map[string]bool{}

	for _, b := range c.buses() {
		seen[b.Path] = true
		svcs, truncated, err := c.read(b)
		if err != nil {
			// Every failure is recorded, not just the optional ones: the cache
			// discards the returned error, so Unavailable is the only thing
			// that reaches the sample.
			problems = append(problems, fmt.Sprintf("%s: %v", busLabel(b), err))
			if b.Scope == ScopeSystem {
				systemErr = err
			}
			continue
		}
		if truncated {
			problems = append(problems, fmt.Sprintf("%s: only the first %d units are reported", b.Path, maxServicesPerBus))
		}
		for _, s := range svcs {
			if len(out.Services) >= maxServicesTotal {
				problems = append(problems, fmt.Sprintf("only the first %d units are reported", maxServicesTotal))
				break
			}
			s.Scope = string(b.Scope)
			s.Owner = b.Owner
			out.Services = append(out.Services, s)
		}
		if len(out.Services) >= maxServicesTotal {
			break
		}
	}

	// Drop connections for managers that are gone, which happens as users log
	// out, so their sockets are not held open forever.
	for path, l := range c.pool {
		if !seen[path] {
			l.Close()
			delete(c.pool, path)
		}
	}

	c.setWarn(strings.Join(problems, "; "))
	if len(out.Services) == 0 && systemErr != nil {
		return nil, systemErr
	}
	return out, nil
}

// read returns one manager's units, reusing its connection and dropping it on
// error so the next refresh reconnects. systemd restarted, or the user's manager
// replaced, leaves the old connection dead.
func (c *systemdCollector) read(b systemdBus) ([]protocol.Service, bool, error) {
	l, ok := c.pool[b.Path]
	if !ok {
		var err error
		l, err = c.open(b, defaultSystemdTimeout)
		if err != nil {
			return nil, false, err
		}
		c.pool[b.Path] = l
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultSystemdTimeout)
	defer cancel()
	svcs, truncated, err := l.ListServices(ctx)
	if err != nil {
		l.Close()
		delete(c.pool, b.Path)
		return nil, false, fmt.Errorf("%s: %w", b.Path, err)
	}
	return svcs, truncated, nil
}

func (c *systemdCollector) setWarn(w string) {
	c.mu.Lock()
	c.warn = w
	c.mu.Unlock()
}

func (c *systemdCollector) warning() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.warn
}

func (c *systemdCollector) Name() string { return "systemd" }

func (c *systemdCollector) Collect(_ context.Context, s *protocol.Sample) error {
	// A partial result is still useful, so the reason it is partial is reported
	// next to it rather than thrown away.
	if w := c.warning(); w != "" {
		if s.Unavailable == nil {
			s.Unavailable = map[string]string{}
		}
		s.Unavailable[c.Name()] = w
	}
	if v, ok := c.cache.get(s.Time); ok {
		s.Systemd = v
	}
	return nil
}

//go:build linux

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

// fakeLister stands in for SystemdHelper in the collector tests.
type fakeLister struct {
	mu        sync.Mutex
	svcs      []protocol.Service
	truncated bool
	err       error
	calls     int
	closed    bool
}

func (f *fakeLister) ListServices(context.Context) ([]protocol.Service, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, false, f.err
	}
	return f.svcs, f.truncated, nil
}

func (f *fakeLister) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeLister) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeLister) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// opener hands out one fake lister per bus path and counts how often each was
// dialled, which is what the connection-reuse test needs.
type opener struct {
	mu     sync.Mutex
	byPath map[string]*fakeLister
	fail   map[string]error
	opened map[string]int
}

func newOpener() *opener {
	return &opener{byPath: map[string]*fakeLister{}, fail: map[string]error{}, opened: map[string]int{}}
}

func (o *opener) open(b systemdBus, _ time.Duration) (systemdLister, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.opened[b.Path]++
	if err, bad := o.fail[b.Path]; bad {
		return nil, err
	}
	l := o.byPath[b.Path]
	if l == nil {
		l = &fakeLister{}
		o.byPath[b.Path] = l
	}
	return l, nil
}

func (o *opener) openCount(path string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.opened[path]
}

func (o *opener) setFail(path string, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.fail[path] = err
}

func waitForCalls(t *testing.T, f *fakeLister) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for f.callCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("collector never queried systemd")
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
}

func sampleAt(t time.Time) *protocol.Sample { return &protocol.Sample{Time: t} }

// collectAndWait starts the background refresh and then collects a second time,
// because the first sample only kicks the refresh off.
func collectAndWait(t *testing.T, c *systemdCollector, s *protocol.Sample, l *fakeLister) {
	t.Helper()
	if err := c.Collect(context.Background(), s); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	waitForCalls(t, l)
	if err := c.Collect(context.Background(), s); err != nil {
		t.Fatalf("Collect: %v", err)
	}
}

const (
	sysPath  = "/run/dbus/system_bus_socket"
	userPath = "/run/user/1000/systemd/private"
)

func twoBuses() []systemdBus {
	return []systemdBus{
		{Scope: ScopeSystem, Path: sysPath, UID: -1},
		{Scope: ScopeUser, Owner: "alice", Path: userPath, UID: 1000, Private: true},
	}
}

// collectorFor builds a collector with fixed buses, skipping the file-system
// probing that NewSystemd does.
func collectorFor(buses []systemdBus, o *opener, every time.Duration) *systemdCollector {
	c := &systemdCollector{
		buses: func() []systemdBus { return buses },
		open:  o.open,
		pool:  map[string]systemdLister{},
	}
	cache := newAsyncCached(every, c.collect)
	cache.maxAge = 3 * every
	c.cache = cache
	return c
}

func TestSystemdCollectorMergesScopes(t *testing.T) {
	o := newOpener()
	o.byPath[sysPath] = &fakeLister{svcs: []protocol.Service{
		{Name: "sshd.service", ActiveState: "active", SubState: "running"},
		{Name: "cron.service", ActiveState: "failed", SubState: "failed"},
	}}
	o.byPath[userPath] = &fakeLister{svcs: []protocol.Service{
		{Name: "pipewire.service", ActiveState: "active", SubState: "running"},
	}}
	c := collectorFor(twoBuses(), o, time.Hour)

	s := sampleAt(time.Now())
	collectAndWait(t, c, s, o.byPath[userPath])
	if s.Systemd == nil || len(s.Systemd.Services) != 3 {
		t.Fatalf("services = %+v", s.Systemd)
	}
	// Each unit must say where it came from, and only user units have an owner.
	got := map[string]protocol.Service{}
	for _, svc := range s.Systemd.Services {
		got[svc.Name] = svc
	}
	if svc := got["sshd.service"]; svc.Scope != "system" || svc.Owner != "" {
		t.Fatalf("system unit = %+v", svc)
	}
	if svc := got["pipewire.service"]; svc.Scope != "user" || svc.Owner != "alice" {
		t.Fatalf("user unit = %+v", svc)
	}
	if s.Unavailable != nil {
		t.Fatalf("unavailable = %v, want nothing", s.Unavailable)
	}
}

// A user logging out mid-refresh must not fail the sample, but it must be
// visible: a machine that only ever reports system units otherwise looks
// complete.
func TestSystemdCollectorReportsBrokenUserBus(t *testing.T) {
	o := newOpener()
	o.byPath[sysPath] = &fakeLister{svcs: []protocol.Service{{Name: "sshd.service", ActiveState: "active"}}}
	o.setFail(userPath, errors.New("permission denied"))
	c := collectorFor(twoBuses(), o, time.Hour)

	s := sampleAt(time.Now())
	collectAndWait(t, c, s, o.byPath[sysPath])
	if s.Systemd == nil || len(s.Systemd.Services) != 1 {
		t.Fatalf("services = %+v", s.Systemd)
	}
	why := s.Unavailable["systemd"]
	if why == "" {
		t.Fatal("a failing user bus was reported silently")
	}
	if !strings.Contains(why, "alice") || !strings.Contains(why, "permission denied") {
		t.Fatalf("reason %q does not name the user and the error", why)
	}
}

func TestSystemdCollectorFailsWhenNothingRead(t *testing.T) {
	o := newOpener()
	o.setFail(sysPath, errors.New("no systemd here"))
	c := collectorFor([]systemdBus{{Scope: ScopeSystem, Path: sysPath, UID: -1}}, o, time.Hour)

	s := sampleAt(time.Now())
	if err := c.Collect(context.Background(), s); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// Wait for the background refresh to finish, then read the reason off a
	// fresh sample.
	deadline := time.Now().Add(2 * time.Second)
	for o.openCount(sysPath) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("collector never tried the bus")
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	if err := c.Collect(context.Background(), s); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if why := s.Unavailable["systemd"]; !strings.Contains(why, "no systemd here") {
		t.Fatalf("nothing read, but reason = %q", why)
	}
	if s.Systemd != nil {
		t.Fatalf("systemd = %+v, want nil", s.Systemd)
	}
}

func TestSystemdCollectorNeverBlocks(t *testing.T) {
	o := newOpener()
	release := make(chan struct{})
	blocking := &blockingLister{release: release}
	c := collectorFor([]systemdBus{{Scope: ScopeSystem, Path: sysPath, UID: -1}}, o, time.Hour)
	c.open = func(systemdBus, time.Duration) (systemdLister, error) { return blocking, nil }

	done := make(chan struct{})
	go func() {
		c.Collect(context.Background(), sampleAt(time.Now()))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Collect blocked on a slow bus")
	}
	close(release)
}

// A wedged systemd must not leave the last counts on screen forever.
func TestSystemdCollectorDropsStaleValue(t *testing.T) {
	o := newOpener()
	l := &fakeLister{svcs: []protocol.Service{{Name: "a.service", ActiveState: "active"}}}
	o.byPath[sysPath] = l
	every := 50 * time.Millisecond
	c := collectorFor([]systemdBus{{Scope: ScopeSystem, Path: sysPath, UID: -1}}, o, every)

	now := time.Now()
	s := sampleAt(now)
	collectAndWait(t, c, s, l)
	if s.Systemd == nil {
		t.Fatal("no value on the first sample")
	}

	l.mu.Lock()
	l.err = errors.New("hung")
	l.mu.Unlock()
	// Well past maxAge, so the old value must be gone rather than repeated.
	later := sampleAt(now.Add(4 * every))
	c.Collect(context.Background(), later)
	if later.Systemd != nil {
		t.Fatalf("stale value kept: %+v", later.Systemd)
	}
}

func TestSystemdCollectorName(t *testing.T) {
	c := collectorFor(nil, newOpener(), time.Hour)
	if c.Name() != "systemd" {
		t.Fatalf("name = %q", c.Name())
	}
}

// Connections are kept between refreshes: the library opens two sockets per
// bus, and doing that every 15 s for every user adds up.
func TestSystemdCollectorReusesConnections(t *testing.T) {
	o := newOpener()
	l := &fakeLister{svcs: []protocol.Service{{Name: "a.service", ActiveState: "active"}}}
	o.byPath[sysPath] = l
	buses := []systemdBus{{Scope: ScopeSystem, Path: sysPath, UID: -1}}
	c := collectorFor(buses, o, time.Millisecond)

	c.collect()
	c.collect()
	c.collect()

	if n := o.openCount(sysPath); n != 1 {
		t.Fatalf("opened %d times, want 1", n)
	}
	if l.isClosed() {
		t.Fatal("connection closed while still in use")
	}
}

func TestSystemdCollectorReconnectsAfterError(t *testing.T) {
	o := newOpener()
	l := &fakeLister{svcs: []protocol.Service{{Name: "a.service", ActiveState: "active"}}}
	o.byPath[sysPath] = l
	buses := []systemdBus{{Scope: ScopeSystem, Path: sysPath, UID: -1}}
	c := collectorFor(buses, o, time.Millisecond)

	c.collect()
	l.mu.Lock()
	l.err = errors.New("systemd restarted")
	l.mu.Unlock()
	if _, _, err := c.read(buses[0]); err == nil {
		t.Fatal("want the error to surface")
	}
	l.mu.Lock()
	l.err = nil
	l.mu.Unlock()

	c.collect()
	if n := o.openCount(sysPath); n != 2 {
		t.Fatalf("opened %d times, want 2 (one reconnect)", n)
	}
}

// As users log out their sockets vanish; holding their connections open would
// leak one per session.
func TestSystemdCollectorClosesVanishedBuses(t *testing.T) {
	o := newOpener()
	user := &fakeLister{svcs: []protocol.Service{{Name: "a.service", ActiveState: "active"}}}
	o.byPath[userPath] = user
	buses := twoBuses()
	c := collectorFor(buses, o, time.Millisecond)

	c.collect()
	if user.isClosed() {
		t.Fatal("closed while the bus is still listed")
	}

	// The user logs out: only the system bus remains.
	c.buses = func() []systemdBus { return buses[:1] }
	c.collect()
	if !user.isClosed() {
		t.Fatal("connection kept for a bus that is gone")
	}
}

func TestSystemdCollectorCapsTotal(t *testing.T) {
	o := newOpener()
	var many []protocol.Service
	for i := 0; i < maxServicesTotal+200; i++ {
		many = append(many, protocol.Service{Name: fmt.Sprintf("u%04d.service", i)})
	}
	o.byPath[sysPath] = &fakeLister{svcs: many}
	c := collectorFor([]systemdBus{{Scope: ScopeSystem, Path: sysPath, UID: -1}}, o, time.Millisecond)

	got, err := c.collect()
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(got.Services) != maxServicesTotal {
		t.Fatalf("kept %d units, want %d", len(got.Services), maxServicesTotal)
	}
	if !strings.Contains(c.warning(), "only the first") {
		t.Fatalf("warning = %q", c.warning())
	}
}

func TestSystemdCollectorReportsTruncation(t *testing.T) {
	o := newOpener()
	o.byPath[sysPath] = &fakeLister{
		svcs:      []protocol.Service{{Name: "a.service"}},
		truncated: true,
	}
	c := collectorFor([]systemdBus{{Scope: ScopeSystem, Path: sysPath, UID: -1}}, o, time.Millisecond)
	c.collect()
	if !strings.Contains(c.warning(), "only the first") {
		t.Fatalf("warning = %q", c.warning())
	}
}

// A socket planted by the user must not be reported as their manager, and the
// bus list must point at the private socket, which a root agent can reach even
// though a default dbus-daemon session bus would refuse it.
func TestSystemdBusesPrefersPrivateSocket(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "run", "dbus"))
	mustMkdir(t, filepath.Join(root, "run", "user", "1000", "systemd"))
	userDir := filepath.Join(root, "run", "user", "1000")
	socketAt(t, filepath.Join(root, "run", "dbus"), "system_bus_socket")
	socketAt(t, userDir, "bus")
	socketAt(t, filepath.Join(userDir, "systemd"), "private")

	buses := systemdBuses(root)
	if len(buses) != 2 {
		t.Fatalf("buses = %+v", buses)
	}
	if buses[0].Scope != ScopeSystem || buses[0].Path != filepath.Join(root, "run", "dbus", "system_bus_socket") {
		t.Fatalf("system bus = %+v", buses[0])
	}
	if buses[0].UID != -1 {
		t.Fatalf("system bus has an owner check: %+v", buses[0])
	}
	u := buses[1]
	if !u.Private || u.Path != filepath.Join(userDir, "systemd", "private") {
		t.Fatalf("user bus = %+v, want the private socket", u)
	}
	if u.UID != 1000 {
		t.Fatalf("user bus uid = %d", u.UID)
	}
}

// Without a private socket the session bus is the only way in.
func TestSystemdBusesFallsBackToSessionBus(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "run", "dbus"))
	userDir := filepath.Join(root, "run", "user", "1000")
	mustMkdir(t, userDir)
	socketAt(t, userDir, "bus")

	buses := systemdBuses(root)
	if len(buses) != 2 {
		t.Fatalf("buses = %+v", buses)
	}
	if buses[1].Private || buses[1].Path != filepath.Join(userDir, "bus") {
		t.Fatalf("user bus = %+v", buses[1])
	}
}

// In Docker mode the host root is where the host's / is mounted, so the
// container's own buses must not be used.
func TestSystemdBusesUsesHostRoot(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "run", "dbus"))
	socketAt(t, filepath.Join(root, "run", "dbus"), "system_bus_socket")

	buses := systemdBuses(root)
	if len(buses) != 1 {
		t.Fatalf("buses = %+v", buses)
	}
	want := filepath.Join(root, "run", "dbus", "system_bus_socket")
	if buses[0].Path != want {
		t.Fatalf("path = %q, want %q", buses[0].Path, want)
	}
}

func TestSystemdBusesEmptyRootUsesRealPaths(t *testing.T) {
	buses := systemdBuses("")
	if len(buses) == 0 || buses[0].Path != systemBusSocket {
		t.Fatalf("buses = %+v", buses)
	}
}

func TestUserNameFallsBackToUid(t *testing.T) {
	if got := userName("", "1000"); got == "" {
		t.Fatal("empty name")
	}
	// A uid unlikely to exist must come back as the number itself.
	if got := userName("", "64999"); got != "64999" {
		t.Fatalf("got %q", got)
	}
}

func TestIsSocketRejectsNonSockets(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "regular")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if isSocket(file) {
		t.Fatal("regular file reported as a socket")
	}
	if isSocket(filepath.Join(dir, "absent")) {
		t.Fatal("missing path reported as a socket")
	}
	if !isSocket(socketAt(t, dir, "sock")) {
		t.Fatal("socket not recognised")
	}
}

type blockingLister struct{ release chan struct{} }

func (b *blockingLister) ListServices(context.Context) ([]protocol.Service, bool, error) {
	<-b.release
	return nil, false, nil
}

func (b *blockingLister) Close() error { return nil }

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

// In a container the agent's own /etc/passwd is the image's, so user names
// must come from the host's.
func TestUserNameReadsHostPasswd(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "etc"))
	passwd := "root:x:0:0:root:/root:/bin/sh\nalice:x:1000:1000::/home/alice:/bin/sh\n"
	if err := os.WriteFile(filepath.Join(root, "etc", "passwd"), []byte(passwd), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := userName(root, "1000"); got != "alice" {
		t.Fatalf("got %q, want alice", got)
	}
	if got := userName(root, "1001"); got != "1001" {
		t.Fatalf("unknown uid gave %q", got)
	}
}

// The unit cap alone does not bound the size: descriptions escape to several
// bytes per character in JSON. The section must stay well inside the hub's
// 1 MiB limit whatever the managers return.
func TestSystemdCollectorCapsBytes(t *testing.T) {
	o := newOpener()
	desc := strings.Repeat("<", maxDescription) // six bytes each once escaped
	var many []protocol.Service
	for i := 0; i < maxServicesTotal; i++ {
		many = append(many, protocol.Service{Name: fmt.Sprintf("u%04d.service", i), Description: desc})
	}
	o.byPath[sysPath] = &fakeLister{svcs: many}
	c := collectorFor([]systemdBus{{Scope: ScopeSystem, Path: sysPath, UID: -1}}, o, time.Millisecond)

	got, err := c.collect()
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	b, _ := json.Marshal(got)
	if len(b) > maxServicesBytes {
		t.Fatalf("section is %d bytes, cap is %d", len(b), maxServicesBytes)
	}
	if len(got.Services) == 0 {
		t.Fatal("cap dropped everything")
	}
	if !strings.Contains(c.warning(), "KiB") {
		t.Fatalf("warning = %q", c.warning())
	}
}

// A machine without systemd (another init system, WSL) has no system bus
// socket. That is not a fault, so it must not be flagged on every sample.
func TestSystemdCollectorQuietWithoutSystemd(t *testing.T) {
	o := newOpener()
	o.setFail(sysPath, fmt.Errorf("systemd socket %s: %w", sysPath, os.ErrNotExist))
	c := collectorFor([]systemdBus{{Scope: ScopeSystem, Path: sysPath, UID: -1}}, o, time.Millisecond)
	if _, err := c.collect(); err == nil {
		t.Fatal("collect succeeded with nothing read")
	}
	if w := c.warning(); w != "" {
		t.Fatalf("warning = %q, want none", w)
	}
}

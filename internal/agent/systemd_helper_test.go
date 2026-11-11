//go:build linux

package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	systemddbus "github.com/coreos/go-systemd/v22/dbus"

	"github.com/retransmit/kite/internal/protocol"
)

// fakeSystemd stands in for a bus connection.
type fakeSystemd struct {
	units      []systemddbus.UnitStatus
	listErr    error
	closed     bool
	deadlineIn time.Duration
}

func (f *fakeSystemd) ListUnitsContext(ctx context.Context) ([]systemddbus.UnitStatus, error) {
	if d, ok := ctx.Deadline(); ok {
		f.deadlineIn = time.Until(d)
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.units, nil
}

func (f *fakeSystemd) Close() { f.closed = true }

func newFakeHelper(f *fakeSystemd, timeout time.Duration) *SystemdHelper {
	return &SystemdHelper{conn: f, path: "/run/dbus/system_bus_socket", timeout: timeout}
}

func TestListServicesFiltersAndSorts(t *testing.T) {
	f := &fakeSystemd{units: []systemddbus.UnitStatus{
		{Name: "ssh.service", LoadState: "loaded", ActiveState: "active", SubState: "running", Description: "OpenSSH"},
		{Name: "dbus.socket", LoadState: "loaded", ActiveState: "active", SubState: "running"},
		{Name: "cron.service", LoadState: "loaded", ActiveState: "failed", SubState: "failed", Description: "Cron"},
		{Name: "app.service", LoadState: "loaded", ActiveState: "inactive", SubState: "dead"},
	}}
	svcs, truncated, err := newFakeHelper(f, time.Second).ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if truncated {
		t.Fatal("truncated with only four units")
	}
	want := []string{"app.service", "cron.service", "ssh.service"}
	var got []string
	for _, s := range svcs {
		got = append(got, s.Name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v, want %v (sockets must be dropped, names sorted)", got, want)
	}
	// The whole point of the rewrite: one type instead of two.
	var full protocol.Service
	for _, s := range svcs {
		if s.Name == "ssh.service" {
			full = s
		}
	}
	if full.ActiveState != "active" || full.SubState != "running" || full.LoadState != "loaded" || full.Description != "OpenSSH" {
		t.Fatalf("states not carried over: %+v", full)
	}
	// Scope is the collector's job, not the helper's.
	if full.Scope != "" || full.Owner != "" {
		t.Fatalf("helper set scope/owner: %+v", full)
	}
}

func TestListServicesEmptyIsNotNil(t *testing.T) {
	svcs, _, err := newFakeHelper(&fakeSystemd{}, time.Second).ListServices(context.Background())
	if err != nil || svcs == nil {
		t.Fatalf("err %v, svcs %v", err, svcs)
	}
	if len(svcs) != 0 {
		t.Fatalf("got %d services", len(svcs))
	}
}

func TestListServicesErrorIsWrapped(t *testing.T) {
	f := &fakeSystemd{listErr: errors.New("bus is on fire")}
	_, _, err := newFakeHelper(f, time.Second).ListServices(context.Background())
	if err == nil || !strings.Contains(err.Error(), "bus is on fire") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "/run/dbus/system_bus_socket") {
		t.Fatalf("error does not say which socket: %v", err)
	}
}

// A session bus socket lives in a directory its user controls, so one user
// could answer with thousands of units. The cap is what keeps the sample small
// enough for the hub to accept it.
func TestListServicesCapsUnitsPerBus(t *testing.T) {
	units := make([]systemddbus.UnitStatus, maxServicesPerBus+100)
	for i := range units {
		units[i] = systemddbus.UnitStatus{Name: fmt.Sprintf("unit%04d.service", i), ActiveState: "inactive"}
	}
	svcs, truncated, err := newFakeHelper(&fakeSystemd{units: units}, time.Second).ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if !truncated {
		t.Fatal("truncated = false with more units than the cap")
	}
	if len(svcs) != maxServicesPerBus {
		t.Fatalf("kept %d units, want %d", len(svcs), maxServicesPerBus)
	}
}

func TestListServicesCapsDescription(t *testing.T) {
	long := strings.Repeat("x", maxDescription*2)
	svcs, _, err := newFakeHelper(&fakeSystemd{units: []systemddbus.UnitStatus{
		{Name: "a.service", Description: long},
	}}, time.Second).ListServices(context.Background())
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if n := len([]rune(svcs[0].Description)); n != maxDescription {
		t.Fatalf("description is %d runes, want %d", n, maxDescription)
	}
}

func TestTruncateKeepsRunesIntact(t *testing.T) {
	// Cutting mid-rune would produce invalid UTF-8 in the JSON.
	s := strings.Repeat("é", 10)
	got := truncate(s, 5)
	if got != strings.Repeat("é", 5) {
		t.Fatalf("got %q", got)
	}
	if truncate("short", 5) != "short" {
		t.Fatal("short strings must be unchanged")
	}
}

func TestEachCallGetsItsOwnDeadline(t *testing.T) {
	f := &fakeSystemd{}
	if _, _, err := newFakeHelper(f, 2*time.Second).ListServices(context.Background()); err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if f.deadlineIn <= 0 || f.deadlineIn > 2*time.Second {
		t.Fatalf("deadline in %v, want at most 2s", f.deadlineIn)
	}
}

func TestZeroTimeoutMeansNoDeadline(t *testing.T) {
	f := &fakeSystemd{}
	if _, _, err := newFakeHelper(f, 0).ListServices(context.Background()); err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if f.deadlineIn != 0 {
		t.Fatalf("deadline in %v, want none", f.deadlineIn)
	}
}

func TestCloseIsSafe(t *testing.T) {
	var nilHelper *SystemdHelper
	if err := nilHelper.Close(); err != nil {
		t.Fatalf("nil helper: %v", err)
	}
	f := &fakeSystemd{}
	if err := newFakeHelper(f, time.Second).Close(); err != nil || !f.closed {
		t.Fatalf("err %v, closed %v", err, f.closed)
	}
}

// socketAt creates a real unix socket so ownership and mode can be inspected.
func socketAt(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return path
}

func TestCheckBusSocketAcceptsOwnedSocket(t *testing.T) {
	path := socketAt(t, t.TempDir(), "bus")
	if err := checkBusSocket(path, os.Getuid()); err != nil {
		t.Fatalf("owned socket rejected: %v", err)
	}
}

func TestCheckBusSocketSkipsOwnerCheckWhenUnknown(t *testing.T) {
	// The system bus is world-writable and owned by whoever runs its daemon.
	path := socketAt(t, t.TempDir(), "bus")
	if err := checkBusSocket(path, -1); err != nil {
		t.Fatalf("system socket rejected: %v", err)
	}
}

func TestCheckBusSocketRejectsOtherOwner(t *testing.T) {
	path := socketAt(t, t.TempDir(), "bus")
	err := checkBusSocket(path, os.Getuid()+1)
	if err == nil || !strings.Contains(err.Error(), "owned by uid") {
		t.Fatalf("err = %v", err)
	}
}

// A symlink would point the agent, which runs as root, at a file of the user's
// choosing, so it must be refused even though the target is a real socket.
func TestCheckBusSocketRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := socketAt(t, dir, "real")
	link := filepath.Join(dir, "bus")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := checkBusSocket(link, -1); err == nil || !strings.Contains(err.Error(), "not a socket") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckBusSocketRejectsRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus")
	if err := os.WriteFile(path, []byte("not a socket"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := checkBusSocket(path, -1); err == nil || !strings.Contains(err.Error(), "not a socket") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckBusSocketReportsMissingSocket(t *testing.T) {
	err := checkBusSocket(filepath.Join(t.TempDir(), "absent"), -1)
	if err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("err = %v", err)
	}
}

func TestDialWithinTimesOut(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	_, err := dialWithin("test bus", 20*time.Millisecond, func() (*systemddbus.Conn, error) {
		<-release
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
}

func TestDialWithinPropagatesError(t *testing.T) {
	boom := errors.New("refused")
	_, err := dialWithin("test bus", time.Second, func() (*systemddbus.Conn, error) {
		return nil, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestDialWithinReturnsConnection(t *testing.T) {
	conn, err := dialWithin("test bus", time.Second, func() (*systemddbus.Conn, error) {
		return nil, nil
	})
	if err != nil || conn != nil {
		t.Fatalf("conn %v, err %v", conn, err)
	}
}

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

const (
	amdRoot   = "testdata/sysfs/amd-laptop"
	intelRoot = "testdata/sysfs/intel-server"
	emptyRoot = "testdata/sysfs/empty"
)

func collect(t *testing.T, c Collector, s *protocol.Sample) *protocol.Sample {
	t.Helper()
	if s.Time.IsZero() {
		s.Time = time.Unix(100, 0)
	}
	if err := c.Collect(context.Background(), s); err != nil {
		t.Fatalf("%s: %v", c.Name(), err)
	}
	return s
}

func near(a, b float64) bool { d := a - b; return d < 1e-9 && d > -1e-9 }

func TestCPUExtraAMD(t *testing.T) {
	c, ok := NewCPUExtra(amdRoot)
	if !ok {
		t.Fatal("not detected")
	}
	s := collect(t, c, &protocol.Sample{CPU: &protocol.CPU{}})
	if s.CPU.FreqMHz == nil || *s.CPU.FreqMHz != 2000 || s.CPU.TempC == nil || !near(*s.CPU.TempC, 52.125) {
		t.Fatalf("got freq %v temp %v", s.CPU.FreqMHz, s.CPU.TempC)
	}
}

func TestCPUExtraIntel(t *testing.T) {
	c, _ := NewCPUExtra(intelRoot)
	s := collect(t, c, &protocol.Sample{CPU: &protocol.CPU{}})
	if *s.CPU.FreqMHz != 1600 || *s.CPU.TempC != 48 {
		t.Fatalf("got freq %v temp %v", *s.CPU.FreqMHz, *s.CPU.TempC)
	}
}

func TestBatteryAMD(t *testing.T) {
	c, ok := NewBattery(amdRoot)
	if !ok {
		t.Fatal("not detected")
	}
	b := collect(t, c, &protocol.Sample{}).Battery
	if b.Percent != 81 || b.State != "discharging" || b.Watts == nil || *b.Watts != -7.5 {
		t.Fatalf("got %+v watts %v", b, b.Watts)
	}
}

func TestBatteryIntel(t *testing.T) {
	c, _ := NewBattery(intelRoot)
	b := collect(t, c, &protocol.Sample{}).Battery
	if b.Percent != 95 || b.State != "charging" || *b.Watts != 12 {
		t.Fatalf("got %+v watts %v", b, *b.Watts)
	}
}

func TestPowerState(t *testing.T) {
	cases := []struct {
		root         string
		hasBat, onAC bool
	}{{amdRoot, true, false}, {intelRoot, true, true}, {emptyRoot, false, true}}
	for _, c := range cases {
		hb, ac := powerState(c.root)
		if hb != c.hasBat || ac != c.onAC {
			t.Errorf("%s: got (%v,%v) want (%v,%v)", c.root, hb, ac, c.hasBat, c.onAC)
		}
	}
}

func TestRAPL(t *testing.T) {
	root := copyTree(t, intelRoot)
	c, ok := NewRAPL(root)
	if !ok {
		t.Fatal("not detected")
	}
	s := collect(t, c, &protocol.Sample{Time: time.Unix(100, 0)})
	if s.Power != nil && (s.Power.CPUWatts != nil || s.Power.PlatformWatts != nil) {
		t.Fatalf("baseline sample must have no watts: %+v", s.Power)
	}
	write(t, root, "sys/class/powercap/intel-rapl:0/energy_uj", "6000000")
	write(t, root, "sys/class/powercap/intel-rapl:1/energy_uj", "12000000")
	s = collect(t, c, &protocol.Sample{Time: time.Unix(102, 0)})
	if s.Power == nil || *s.Power.CPUWatts != 2.5 || *s.Power.PlatformWatts != 5 {
		t.Fatalf("got %+v", s.Power)
	}
}

func TestRAPLWrap(t *testing.T) {
	if d := raplDelta(65532610000, 1000, 65532610987); d != 1987 {
		t.Fatalf("wrap delta %d", d)
	}
	if d := raplDelta(5, 10, 100); d != 5 {
		t.Fatalf("plain delta %d", d)
	}
}

func TestRAPLPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read anything")
	}
	root := copyTree(t, amdRoot)
	if err := os.Chmod(filepath.Join(root, "sys/class/powercap/intel-rapl:0/energy_uj"), 0); err != nil {
		t.Fatal(err)
	}
	c, ok := NewRAPL(root)
	if !ok {
		t.Fatal("RAPL present but unreadable must still be detected, to explain why")
	}
	err := c.Collect(context.Background(), &protocol.Sample{Time: time.Unix(1, 0)})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("got %v", err)
	}
}

func TestLinuxEmptyRoot(t *testing.T) {
	if _, ok := NewCPUExtra(emptyRoot); ok {
		t.Error("cpuextra detected on empty root")
	}
	if _, ok := NewBattery(emptyRoot); ok {
		t.Error("battery detected on empty root")
	}
	if _, ok := NewRAPL(emptyRoot); ok {
		t.Error("rapl detected on empty root")
	}
}

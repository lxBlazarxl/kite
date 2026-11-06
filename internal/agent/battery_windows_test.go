package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

func TestBatteryWithoutWMI(t *testing.T) {
	c := newWindowsBattery(readPowerStatus, func() (wmiBatteryStatus, error) { return wmiBatteryStatus{}, errors.New("wmi broken") })
	s := &protocol.Sample{Time: time.Now()}
	if err := c.Collect(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if s.Battery == nil || s.Battery.Watts != nil {
		t.Fatalf("battery %+v", s.Battery)
	}
	ps, _ := readPowerStatus()
	if ps.OnAC && s.Battery.State != "full" && s.Battery.State != "charging" {
		t.Fatalf("on AC but state %q", s.Battery.State)
	}
}

func TestWindowsBatteryReal(t *testing.T) {
	c, ok := NewWindowsBattery()
	if !ok {
		t.Fatal("not detected")
	}
	s := collectUntil(t, c, func(s *protocol.Sample) bool { return s.Battery != nil && s.Battery.Watts != nil })
	b := s.Battery
	if b == nil || b.Percent < 0 || b.Percent > 100 || b.Watts == nil {
		t.Fatalf("battery %+v", b)
	}
	t.Logf("battery %.0f%% %s %.1f W", b.Percent, b.State, *b.Watts)
}

func TestSystemTempReal(t *testing.T) {
	c, ok := NewSystemTemp()
	if !ok {
		t.Fatal("not detected")
	}
	s := collectUntil(t, c, func(s *protocol.Sample) bool { return s.SystemTempC != nil })
	if s.SystemTempC == nil || *s.SystemTempC < 10 || *s.SystemTempC > 110 {
		t.Fatalf("temp %v", s.SystemTempC)
	}
	t.Logf("system %.1f C", *s.SystemTempC)
}

// collectUntil collects repeatedly (background WMI refreshes land a moment
// later) until done reports true or two seconds pass.
func collectUntil(t *testing.T, c Collector, done func(*protocol.Sample) bool) *protocol.Sample {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		s := &protocol.Sample{Time: time.Now()}
		if err := c.Collect(context.Background(), s); err != nil {
			t.Fatal(err)
		}
		if done(s) || time.Now().After(deadline) {
			return s
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNewSystemTempAlwaysDetected(t *testing.T) {
	if _, ok := NewSystemTemp(); !ok {
		t.Fatal("thermal must be detected without a synchronous probe (retried lazily)")
	}
}

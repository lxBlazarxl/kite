package agent

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

type fakeCollector struct {
	name string
	fn   func(s *protocol.Sample) error
}

func (f fakeCollector) Name() string { return f.name }
func (f fakeCollector) Collect(_ context.Context, s *protocol.Sample) error {
	return f.fn(s)
}

func TestCollectOnceRecordsErrors(t *testing.T) {
	cfg := Config{Name: "pc", Interval: 5 * time.Second}
	now := time.Unix(1_700_000_000, 0)
	cols := []Collector{
		fakeCollector{"ok", func(s *protocol.Sample) error { s.CPU = &protocol.CPU{TotalPct: 1}; return nil }},
		fakeCollector{"bad", func(*protocol.Sample) error { return errors.New("boom") }},
	}
	s := CollectOnce(context.Background(), cfg, cols, now)
	if s.CPU == nil || s.CPU.TotalPct != 1 {
		t.Fatalf("cpu not set: %+v", s.CPU)
	}
	if s.Unavailable["bad"] != "boom" {
		t.Fatalf("unavailable = %v", s.Unavailable)
	}
	if s.IntervalSec != 5 || s.OS != runtime.GOOS || s.Version != protocol.Version || !s.Time.Equal(now) || s.Machine != "pc" {
		t.Fatalf("header wrong: %+v", s)
	}
}

func TestCPUAndMemoryReal(t *testing.T) {
	cfg := Config{Name: "pc", Interval: 5 * time.Second, Root: "/"}
	cols := []Collector{NewCPU(), NewMemory()}
	CollectOnce(context.Background(), cfg, cols, time.Now())
	time.Sleep(200 * time.Millisecond)
	s := CollectOnce(context.Background(), cfg, cols, time.Now())
	if s.CPU == nil || s.CPU.TotalPct < 0 || s.CPU.TotalPct > 100 {
		t.Fatalf("cpu: %+v", s.CPU)
	}
	if len(s.CPU.PerCorePct) != runtime.NumCPU() {
		t.Fatalf("per core len %d want %d", len(s.CPU.PerCorePct), runtime.NumCPU())
	}
	if s.Memory == nil || s.Memory.TotalBytes == 0 || s.Memory.UsedBytes > s.Memory.TotalBytes {
		t.Fatalf("memory: %+v", s.Memory)
	}
}

func TestCollectOnceBoundsHungCollector(t *testing.T) {
	cfg := Config{Name: "pc", Interval: 400 * time.Millisecond}
	hung := fakeCollector{"docker", func(*protocol.Sample) error { return nil }}
	cols := []Collector{ctxCollector{"docker"}, hung}
	start := time.Now()
	s := CollectOnce(context.Background(), cfg, cols, start)
	if el := time.Since(start); el > 300*time.Millisecond {
		t.Fatalf("hung collector held the sample for %v", el)
	}
	if s.Unavailable["docker"] == "" {
		t.Fatal("timeout not recorded in unavailable")
	}
}

// ctxCollector blocks until its context is done, like a hung Docker daemon.
type ctxCollector struct{ name string }

func (c ctxCollector) Name() string { return c.name }
func (c ctxCollector) Collect(ctx context.Context, _ *protocol.Sample) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestCollectTimed(t *testing.T) {
	cols := []Collector{
		fakeCollector{"fast", func(*protocol.Sample) error { return nil }},
		fakeCollector{"slow", func(*protocol.Sample) error { time.Sleep(20 * time.Millisecond); return nil }},
	}
	_, timings := CollectTimed(context.Background(), Config{Name: "pc", Interval: 5 * time.Second}, cols, time.Now())
	if len(timings) != 2 || timings[0].Name != "fast" || timings[1].Name != "slow" || timings[1].Took < 20*time.Millisecond {
		t.Fatalf("got %+v", timings)
	}
}

package agent

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestHostReal(t *testing.T) {
	cfg := Config{Name: "pc", Interval: 5 * time.Second}
	s := CollectOnce(context.Background(), cfg, []Collector{NewHost()}, time.Now())
	if s.UptimeSec == 0 {
		t.Fatalf("uptime missing (unavailable %v)", s.Unavailable)
	}
	if runtime.GOOS == "linux" && s.Platform == "" {
		t.Fatal("platform missing on linux")
	}
}

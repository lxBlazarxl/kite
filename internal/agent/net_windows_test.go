package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

func TestNetWindowsReal(t *testing.T) {
	c := NewNetWindows(nil, splitList(defaultNetExclude("windows")))
	c.Collect(context.Background(), &protocol.Sample{Time: time.Now()})
	time.Sleep(time.Second)
	s := &protocol.Sample{Time: time.Now()}
	if err := c.Collect(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, n := range s.Net {
		names[n.Name] = true
		if strings.Contains(n.Name, "Filter") || strings.HasPrefix(n.Name, "Local Area Connection*") {
			t.Errorf("should be hidden: %q", n.Name)
		}
	}
	if !names["Ethernet"] || !names["Tailscale"] {
		t.Fatalf("missing real interfaces: %v", names)
	}
	const n = 20
	start := time.Now()
	for i := 0; i < n; i++ {
		c.Collect(context.Background(), &protocol.Sample{Time: time.Now()})
	}
	avg := time.Since(start) / n
	t.Logf("interfaces %v, average collect %v", names, avg)
	if avg > 3*time.Millisecond {
		t.Fatalf("too slow: %v", avg)
	}
}

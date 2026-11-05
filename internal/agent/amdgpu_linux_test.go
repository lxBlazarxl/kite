package agent

import (
	"testing"

	"github.com/retransmit/kite/internal/protocol"
)

func TestAMDGPU(t *testing.T) {
	c, ok := NewAMDGPU(amdRoot)
	if !ok {
		t.Fatal("not detected")
	}
	s := collect(t, c, &protocol.Sample{})
	if len(s.GPUs) != 1 {
		t.Fatalf("want 1 gpu, got %+v", s.GPUs)
	}
	g := s.GPUs[0]
	if g.Index != 0 || g.Vendor != "amd" || *g.UtilPct != 17 || *g.VRAMUsedBytes != 536870912 ||
		*g.VRAMTotalBytes != 2147483648 || *g.PowerW != 4 || *g.TempC != 45 {
		t.Fatalf("got %+v", g)
	}
}

func TestAMDGPUIntelOnly(t *testing.T) {
	if _, ok := NewAMDGPU(intelRoot); ok {
		t.Fatal("intel iGPU must not be detected as AMD")
	}
}

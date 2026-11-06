package agent

import (
	"context"
	"testing"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

const (
	pathGPU3D   = `\GPU Engine(*engtype_3D)\Utilization Percentage`
	pathGPUVRAM = `\GPU Adapter Memory(*)\Dedicated Usage`
)

func TestPDHGPUCountersReal(t *testing.T) {
	q, err := newPDHQuery(pathGPU3D, pathGPUVRAM)
	if err != nil {
		t.Fatal(err)
	}
	defer q.close()
	time.Sleep(time.Second)
	vals, err := q.collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 2 || len(vals[1]) == 0 {
		t.Fatalf("got %d paths, vram instances %d", len(vals), len(vals[1]))
	}
	for _, set := range vals {
		for _, v := range set {
			if _, ok := luidFromInstance(v.Instance); !ok {
				t.Fatalf("instance without luid: %q", v.Instance)
			}
		}
	}
	t.Logf("3D per luid %v, vram per luid %v", sumByLUID(vals[0], 100), sumByLUID(vals[1], 0))
}

func TestCPUFreqReal(t *testing.T) {
	c, ok := NewCPUFreq()
	if !ok {
		t.Fatal("not detected")
	}
	time.Sleep(time.Second)
	s := &protocol.Sample{CPU: &protocol.CPU{}}
	if err := c.Collect(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if s.CPU.FreqMHz == nil || *s.CPU.FreqMHz < 100 || *s.CPU.FreqMHz > 10000 {
		t.Fatalf("freq %v", s.CPU.FreqMHz)
	}
	t.Logf("freq %.0f MHz", *s.CPU.FreqMHz)
}

func TestPDHCollectCost(t *testing.T) {
	q, err := newPDHQuery(pathGPU3D, pathGPUVRAM)
	if err != nil {
		t.Fatal(err)
	}
	defer q.close()
	time.Sleep(200 * time.Millisecond)
	const n = 50
	start := time.Now()
	for i := 0; i < n; i++ {
		if _, err := q.collect(); err != nil {
			t.Fatal(err)
		}
	}
	avg := time.Since(start) / n
	t.Logf("average collect %v", avg)
	if avg > 5*time.Millisecond {
		t.Fatalf("collect too slow: %v", avg)
	}
}

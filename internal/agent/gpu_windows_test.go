package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

func collectGPU(t *testing.T, c Collector) *protocol.Sample {
	t.Helper()
	s := &protocol.Sample{Time: time.Now()}
	if err := c.Collect(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewWindowsGPUReal(t *testing.T) {
	c, ok := NewWindowsGPU()
	if !ok {
		t.Fatal("not detected")
	}
	collectGPU(t, c)
	time.Sleep(time.Second)
	s := collectGPU(t, c)
	if len(s.GPUs) != 2 || s.GPUs[0].Vendor != "amd" || s.GPUs[1].Vendor != "nvidia" || s.GPUs[0].UtilPct == nil || s.GPUs[1].UtilPct == nil {
		t.Fatalf("gpus %+v", s.GPUs)
	}
	if s.Unavailable["gpu.nvidia"] == "" && s.GPUs[1].TempC == nil {
		t.Fatal("nvidia awake on AC but no temperature")
	}
	t.Logf("amd util %.1f, nvidia util %.1f temp %v, unavailable %v", *s.GPUs[0].UtilPct, *s.GPUs[1].UtilPct, deref(s.GPUs[1].TempC), s.Unavailable)
}

func TestNewWindowsGPUWithoutNVML(t *testing.T) {
	adapters, err := enumAdapters()
	if err != nil {
		t.Fatal(err)
	}
	noNVML := func() (nvmlReader, error) { return nil, errors.New("nvml.dll: not found") }
	c, err := newWindowsGPU(orderAdapters(adapters), noNVML, readPowerStatus, nvidiaAwake)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	s := collectGPU(t, c)
	if len(s.GPUs) != 2 || s.GPUs[1].TempC != nil {
		t.Fatalf("gpus %+v", s.GPUs)
	}
	switch s.Unavailable["gpu.nvidia"] {
	case "on battery", "asleep", "nvml.dll not available":
	default:
		t.Fatalf("unexpected unavailable %q", s.Unavailable["gpu.nvidia"])
	}
}

type fakeNVML struct{ reads int }

func (f *fakeNVML) read() ([]NVMLReading, error) {
	f.reads++
	return []NVMLReading{{TempC: f64p(55)}}, nil
}

func TestNVMLLoadedOnlyWhenGateOpens(t *testing.T) {
	adapters, err := enumAdapters()
	if err != nil {
		t.Fatal(err)
	}
	loads := 0
	fake := &fakeNVML{}
	load := func() (nvmlReader, error) { loads++; return fake, nil }
	onAC := false
	power := func() (PowerStatus, error) { return PowerStatus{HasBattery: true, OnAC: onAC}, nil }
	awake := func() (bool, error) { return true, nil }
	c, err := newWindowsGPU(orderAdapters(adapters), load, power, awake)
	if err != nil {
		t.Fatal(err)
	}
	s := collectGPU(t, c)
	if loads != 0 || s.Unavailable["gpu.nvidia"] != "on battery" {
		t.Fatalf("on battery: loads %d unavailable %v", loads, s.Unavailable)
	}
	onAC = true
	s = collectGPU(t, c)
	collectGPU(t, c)
	if loads != 1 || fake.reads != 2 || s.GPUs[1].TempC == nil || *s.GPUs[1].TempC != 55 {
		t.Fatalf("on AC: loads %d reads %d gpu %+v", loads, fake.reads, s.GPUs[1])
	}
}

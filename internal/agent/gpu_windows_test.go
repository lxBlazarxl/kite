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
	if len(s.GPUs) == 0 {
		t.Fatal("no GPUs reported")
	}
	for _, g := range s.GPUs {
		if g.UtilPct == nil {
			t.Fatalf("gpu without usage: %+v", g)
		}
		if g.Vendor == "nvidia" && s.Unavailable["gpu.nvidia"] == "" && g.TempC == nil {
			t.Fatal("nvidia awake on AC but no temperature")
		}
	}
	t.Logf("gpus %d, unavailable %v", len(s.GPUs), s.Unavailable)
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
	if len(s.GPUs) == 0 {
		t.Fatal("no GPUs reported without NVML")
	}
	for _, g := range s.GPUs {
		if g.TempC != nil {
			t.Fatalf("temperature without NVML: %+v", g)
		}
	}
	if !hasVendor(s.GPUs, "nvidia") {
		return
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
	adapters = orderAdapters(adapters)
	nv := -1
	for i, a := range adapters {
		if a.VendorID == vendorNVIDIA {
			nv = i
			break
		}
	}
	if nv < 0 {
		t.Skip("no NVIDIA adapter on this machine")
	}
	loads := 0
	fake := &fakeNVML{}
	load := func() (nvmlReader, error) { loads++; return fake, nil }
	onAC := false
	power := func() (PowerStatus, error) { return PowerStatus{HasBattery: true, OnAC: onAC}, nil }
	awake := func() (bool, error) { return true, nil }
	c, err := newWindowsGPU(adapters, load, power, awake)
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
	if loads != 1 || fake.reads != 2 || s.GPUs[nv].TempC == nil || *s.GPUs[nv].TempC != 55 {
		t.Fatalf("on AC: loads %d reads %d gpu %+v", loads, fake.reads, s.GPUs[nv])
	}
}

func hasVendor(gpus []protocol.GPU, vendor string) bool {
	for _, g := range gpus {
		if g.Vendor == vendor {
			return true
		}
	}
	return false
}

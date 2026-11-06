package agent

import (
	"testing"

	"github.com/retransmit/kite/internal/protocol"
)

func f64p(v float64) *float64 { return &v }
func u64p(v uint64) *uint64   { return &v }

var laptopAdapters = []Adapter{
	{Name: "AMD Radeon(TM) Graphics", VendorID: 0x1002, LUID: "l2"},
	{Name: "NVIDIA GeForce RTX 4060 Laptop GPU", VendorID: 0x10DE, LUID: "l1"},
}

func TestBuildGPUsLaptop(t *testing.T) {
	gpus := buildGPUs(laptopAdapters, map[string]float64{"l1": 26.5, "l2": 0}, map[string]float64{"l1": 537e6, "l2": 2e8})
	if len(gpus) != 2 {
		t.Fatalf("got %+v", gpus)
	}
	a, n := gpus[0], gpus[1]
	if a.Index != 0 || a.Vendor != "amd" || a.UtilPct == nil || *a.UtilPct != 0 || *a.VRAMUsedBytes != 2e8 || a.Name != "AMD Radeon(TM) Graphics" {
		t.Errorf("amd %+v", a)
	}
	if n.Index != 1 || n.Vendor != "nvidia" || *n.UtilPct != 26.5 || *n.VRAMUsedBytes != 537e6 {
		t.Errorf("nvidia %+v", n)
	}
}

func TestBuildGPUsNoNvidia(t *testing.T) {
	gpus := buildGPUs([]Adapter{{Name: "Intel UHD", VendorID: 0x8086, LUID: "l3"}}, map[string]float64{}, map[string]float64{})
	if len(gpus) != 1 || gpus[0].Vendor != "intel" || gpus[0].UtilPct != nil || gpus[0].VRAMUsedBytes != nil {
		t.Fatalf("got %+v", gpus)
	}
}

func TestApplyNVML(t *testing.T) {
	gpus := buildGPUs(laptopAdapters, map[string]float64{"l1": 20, "l2": 1}, map[string]float64{})
	applyNVML(gpus, []NVMLReading{{TempC: f64p(63), PowerW: f64p(12.14), VRAMTotal: u64p(4 << 30), VRAMUsed: u64p(600e6), UtilPct: f64p(34)}})
	n := gpus[1]
	if *n.TempC != 63 || *n.PowerW != 12.14 || *n.UtilPct != 34 || *n.VRAMTotalBytes != 4<<30 || *n.VRAMUsedBytes != 600e6 {
		t.Errorf("nvidia %+v", n)
	}
	if a := gpus[0]; a.TempC != nil || *a.UtilPct != 1 {
		t.Errorf("amd touched: %+v", a)
	}
	var none []protocol.GPU
	applyNVML(none, []NVMLReading{{}}) // no panic without GPUs
}

func TestNVMLGate(t *testing.T) {
	if got := nvmlSkipReason(nvmlGate{OnAC: false, Awake: true}); got != "on battery" {
		t.Errorf("battery: %q", got)
	}
	if got := nvmlSkipReason(nvmlGate{OnAC: true, Awake: false}); got != "asleep" {
		t.Errorf("asleep: %q", got)
	}
	if got := nvmlSkipReason(nvmlGate{OnAC: true, Awake: true}); got != "" {
		t.Errorf("ok: %q", got)
	}
}

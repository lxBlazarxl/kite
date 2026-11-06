package agent

import "github.com/retransmit/kite/internal/protocol"

// buildGPUs turns ordered adapters plus per-LUID counter values into the
// sample's GPU list. A LUID without counters leaves the value nil.
func buildGPUs(adapters []Adapter, util, vramUsed map[string]float64) []protocol.GPU {
	gpus := make([]protocol.GPU, 0, len(adapters))
	for i, a := range adapters {
		g := protocol.GPU{Index: i, Vendor: vendorName(a.VendorID), Name: a.Name}
		if v, ok := util[a.LUID]; ok {
			g.UtilPct = &v
		}
		if v, ok := vramUsed[a.LUID]; ok {
			b := uint64(v)
			g.VRAMUsedBytes = &b
		}
		gpus = append(gpus, g)
	}
	return gpus
}

// applyNVML adds NVML's details to the NVIDIA GPUs, matched in order.
func applyNVML(gpus []protocol.GPU, readings []NVMLReading) {
	next := 0
	for i := range gpus {
		if gpus[i].Vendor != "nvidia" || next >= len(readings) {
			continue
		}
		r := readings[next]
		next++
		g := &gpus[i]
		if r.TempC != nil {
			g.TempC = r.TempC
		}
		if r.PowerW != nil {
			g.PowerW = r.PowerW
		}
		if r.VRAMTotal != nil {
			g.VRAMTotalBytes = r.VRAMTotal
		}
		if r.VRAMUsed != nil {
			g.VRAMUsedBytes = r.VRAMUsed
		}
		if r.UtilPct != nil {
			g.UtilPct = r.UtilPct
		}
	}
}

// nvmlGate decides whether NVML may be queried: never on battery and never
// while the GPU sleeps, since querying wakes it.
type nvmlGate struct{ OnAC, Awake bool }

func nvmlSkipReason(g nvmlGate) string {
	switch {
	case !g.OnAC:
		return "on battery"
	case !g.Awake:
		return "asleep"
	}
	return ""
}

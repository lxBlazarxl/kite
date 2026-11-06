package agent

import "sort"

// Adapter is a display adapter as Windows DXGI reports it.
type Adapter struct {
	Name     string
	VendorID uint32
	DeviceID uint32
	LUID     string // "0x%08x_0x%08x" (high, low), as in GPU counter instance names
	Software bool
}

const (
	vendorAMD       = 0x1002
	vendorNVIDIA    = 0x10DE
	vendorIntel     = 0x8086
	vendorMicrosoft = 0x1414 // Basic Render Driver, Remote Display
)

func vendorName(id uint32) string {
	switch id {
	case vendorAMD:
		return "amd"
	case vendorNVIDIA:
		return "nvidia"
	case vendorIntel:
		return "intel"
	}
	return "other"
}

func vendorRank(id uint32) int {
	switch id {
	case vendorAMD:
		return 0
	case vendorNVIDIA:
		return 1
	}
	return 2
}

// orderAdapters drops software and Microsoft adapters and duplicate LUIDs,
// then orders AMD, NVIDIA, others (keeping DXGI order within each group) so
// GPU numbering matches the Linux agent on dual-boot machines.
func orderAdapters(as []Adapter) []Adapter {
	seen := map[string]bool{}
	var out []Adapter
	for _, a := range as {
		if a.Software || a.VendorID == vendorMicrosoft || seen[a.LUID] {
			continue
		}
		seen[a.LUID] = true
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return vendorRank(out[i].VendorID) < vendorRank(out[j].VendorID) })
	return out
}

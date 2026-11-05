package agent

import (
	"context"
	"path/filepath"
	"regexp"

	"github.com/retransmit/kite/internal/protocol"
)

var drmCard = regexp.MustCompile(`^card\d+$`)

type amdGPUCollector struct{ devices []string }

// NewAMDGPU reports usage, VRAM, power and temperature of AMD GPUs via sysfs.
func NewAMDGPU(root string) (Collector, bool) {
	cards, _ := filepath.Glob(filepath.Join(root, "sys/class/drm/card*"))
	c := &amdGPUCollector{}
	for _, card := range cards {
		if !drmCard.MatchString(filepath.Base(card)) {
			continue
		}
		dev := filepath.Join(card, "device")
		if v, _ := readStr(filepath.Join(dev, "vendor")); v == "0x1002" {
			c.devices = append(c.devices, dev)
		}
	}
	return c, len(c.devices) > 0
}

func (c *amdGPUCollector) Name() string { return "gpu.amd" }

func (c *amdGPUCollector) Collect(_ context.Context, s *protocol.Sample) error {
	for _, dev := range c.devices {
		g := protocol.GPU{Index: len(s.GPUs), Vendor: "amd", Name: "AMD GPU"}
		if v, err := readInt(filepath.Join(dev, "gpu_busy_percent")); err == nil {
			f := float64(v)
			g.UtilPct = &f
		}
		if v, err := readUint(filepath.Join(dev, "mem_info_vram_used")); err == nil {
			g.VRAMUsedBytes = &v
		}
		if v, err := readUint(filepath.Join(dev, "mem_info_vram_total")); err == nil {
			g.VRAMTotalBytes = &v
		}
		hw, _ := filepath.Glob(filepath.Join(dev, "hwmon/hwmon*"))
		for _, h := range hw {
			for _, pf := range []string{"power1_average", "power1_input"} {
				if uw, err := readInt(filepath.Join(h, pf)); err == nil {
					w := float64(uw) / 1e6
					g.PowerW = &w
					break
				}
			}
			if mc, err := readInt(filepath.Join(h, "temp1_input")); err == nil {
				t := float64(mc) / 1000
				g.TempC = &t
			}
		}
		s.GPUs = append(s.GPUs, g)
	}
	return nil
}

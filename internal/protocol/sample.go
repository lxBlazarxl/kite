// Package protocol defines the data an agent sends to the hub and how a
// sample is flattened into numeric metric series.
package protocol

import (
	"regexp"
	"strconv"
	"time"
)

// Version is the wire format version carried in every sample.
const Version = 1

// Sample is one snapshot of a machine, sent by the agent every interval.
// Optional values are pointers: nil means the agent could not read them,
// which is different from a reading of zero.
type Sample struct {
	Version      int               `json:"v"`
	Machine      string            `json:"machine"`
	OS           string            `json:"os"`
	Hostname     string            `json:"hostname"`
	Time         time.Time         `json:"time"`
	IntervalSec  float64           `json:"interval_sec"`
	UptimeSec    uint64            `json:"uptime_sec,omitempty"`
	Platform     string            `json:"platform,omitempty"` // distro or Windows edition, e.g. "arch"
	AgentVersion string            `json:"agent_version,omitempty"`
	SystemTempC  *float64          `json:"system_temp_c,omitempty"` // motherboard/ACPI sensor where no CPU sensor exists (Windows)
	CPU          *CPU              `json:"cpu,omitempty"`
	Memory       *Memory           `json:"memory,omitempty"`
	GPUs         []GPU             `json:"gpus,omitempty"`
	Power        *Power            `json:"power,omitempty"`
	Battery      *Battery          `json:"battery,omitempty"`
	Disks        []Disk            `json:"disks,omitempty"`
	Net          []NetIf           `json:"net,omitempty"`
	Docker       *Docker           `json:"docker,omitempty"`
	Unavailable  map[string]string `json:"unavailable,omitempty"`
}

type CPU struct {
	Model      string    `json:"model,omitempty"`
	TotalPct   float64   `json:"total_pct"`
	PerCorePct []float64 `json:"per_core_pct"`
	FreqMHz    *float64  `json:"freq_mhz,omitempty"`
	TempC      *float64  `json:"temp_c,omitempty"`
}

type Memory struct {
	TotalBytes     uint64 `json:"total_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	SwapTotalBytes uint64 `json:"swap_total_bytes"`
	SwapUsedBytes  uint64 `json:"swap_used_bytes"`
}

type GPU struct {
	Index          int      `json:"index"`
	Vendor         string   `json:"vendor"`
	Name           string   `json:"name"`
	UtilPct        *float64 `json:"util_pct,omitempty"`
	VRAMUsedBytes  *uint64  `json:"vram_used_bytes,omitempty"`
	VRAMTotalBytes *uint64  `json:"vram_total_bytes,omitempty"`
	TempC          *float64 `json:"temp_c,omitempty"`
	PowerW         *float64 `json:"power_w,omitempty"`
}

type Power struct {
	CPUWatts      *float64 `json:"cpu_w,omitempty"`
	PlatformWatts *float64 `json:"platform_w,omitempty"`
}

// Battery.Watts is negative while discharging and positive while charging.
type Battery struct {
	Percent float64  `json:"percent"`
	State   string   `json:"state"`
	Watts   *float64 `json:"watts,omitempty"`
}

type Disk struct {
	Mount      string   `json:"mount"`
	Device     string   `json:"device"`
	FSType     string   `json:"fs_type"`
	TotalBytes uint64   `json:"total_bytes"`
	UsedBytes  uint64   `json:"used_bytes"`
	ReadBps    *float64 `json:"read_bps,omitempty"`
	WriteBps   *float64 `json:"write_bps,omitempty"`
}

type NetIf struct {
	Name  string  `json:"name"`
	RxBps float64 `json:"rx_bps"`
	TxBps float64 `json:"tx_bps"`
}

type Docker struct {
	Containers []Container `json:"containers"`
}

type Container struct {
	Name     string   `json:"name"`
	Image    string   `json:"image"`
	State    string   `json:"state"`
	CPUPct   *float64 `json:"cpu_pct,omitempty"`
	MemBytes *uint64  `json:"mem_bytes,omitempty"`
}

// Point is one numeric value of a flattened sample.
type Point struct {
	Key   string
	Value float64
}

// Flatten turns a sample into metric points. Nil values produce no point.
func Flatten(s *Sample) []Point {
	var pts []Point
	add := func(k string, v float64) { pts = append(pts, Point{k, v}) }
	addF := func(k string, v *float64) {
		if v != nil {
			add(k, *v)
		}
	}
	addU := func(k string, v *uint64) {
		if v != nil {
			add(k, float64(*v))
		}
	}

	if c := s.CPU; c != nil {
		add("cpu.total", c.TotalPct)
		for i, v := range c.PerCorePct {
			add("cpu.core."+strconv.Itoa(i), v)
		}
		addF("cpu.freq_mhz", c.FreqMHz)
		addF("cpu.temp_c", c.TempC)
	}
	addF("system.temp_c", s.SystemTempC)
	if m := s.Memory; m != nil {
		add("mem.total", float64(m.TotalBytes))
		add("mem.used", float64(m.UsedBytes))
		add("swap.total", float64(m.SwapTotalBytes))
		add("swap.used", float64(m.SwapUsedBytes))
	}
	for _, g := range s.GPUs {
		p := "gpu." + strconv.Itoa(g.Index) + "."
		addF(p+"util", g.UtilPct)
		addU(p+"vram_used", g.VRAMUsedBytes)
		addU(p+"vram_total", g.VRAMTotalBytes)
		addF(p+"temp_c", g.TempC)
		addF(p+"power_w", g.PowerW)
	}
	if p := s.Power; p != nil {
		addF("power.cpu_w", p.CPUWatts)
		addF("power.platform_w", p.PlatformWatts)
	}
	if b := s.Battery; b != nil {
		add("battery.pct", b.Percent)
		addF("battery.w", b.Watts)
	}
	for _, d := range s.Disks {
		p := "disk." + d.Mount + "."
		add(p+"used", float64(d.UsedBytes))
		add(p+"total", float64(d.TotalBytes))
		addF(p+"read_bps", d.ReadBps)
		addF(p+"write_bps", d.WriteBps)
	}
	for _, n := range s.Net {
		p := "net." + n.Name + "."
		add(p+"rx_bps", n.RxBps)
		add(p+"tx_bps", n.TxBps)
	}
	if d := s.Docker; d != nil {
		for _, c := range d.Containers {
			p := "docker." + c.Name + "."
			running := 0.0
			if c.State == "running" {
				running = 1
			}
			add(p+"running", running)
			addF(p+"cpu_pct", c.CPUPct)
			addU(p+"mem", c.MemBytes)
		}
	}
	return pts
}

var machineNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidMachineName reports whether name is usable as a machine identity.
func ValidMachineName(name string) bool {
	return machineNameRe.MatchString(name)
}

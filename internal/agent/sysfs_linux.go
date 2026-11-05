package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/retransmit/kite/internal/protocol"
)

func readStr(path string) (string, error) {
	b, err := os.ReadFile(path)
	return strings.TrimSpace(string(b)), err
}

func readInt(path string) (int64, error) {
	s, err := readStr(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(s, 10, 64)
}

func readUint(path string) (uint64, error) {
	s, err := readStr(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(s, 10, 64)
}

// --- CPU frequency and temperature ---

var cpuTempSensors = []string{"k10temp", "coretemp", "zenpower", "cpu_thermal"}

type cpuExtraCollector struct {
	freqFiles []string
	tempFile  string
}

// NewCPUExtra reports CPU frequency (mean of all cores) and package
// temperature from hwmon. It must run after the cpu collector.
func NewCPUExtra(root string) (Collector, bool) {
	c := &cpuExtraCollector{}
	c.freqFiles, _ = filepath.Glob(filepath.Join(root, "sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq"))
	c.tempFile = findHwmonTemp(root)
	return c, len(c.freqFiles) > 0 || c.tempFile != ""
}

func findHwmonTemp(root string) string {
	names, _ := filepath.Glob(filepath.Join(root, "sys/class/hwmon/hwmon*/name"))
	for _, want := range cpuTempSensors {
		for _, n := range names {
			if v, _ := readStr(n); v == want {
				f := filepath.Join(filepath.Dir(n), "temp1_input")
				if _, err := os.Stat(f); err == nil {
					return f
				}
			}
		}
	}
	return ""
}

func (c *cpuExtraCollector) Name() string { return "cpuextra" }

func (c *cpuExtraCollector) Collect(_ context.Context, s *protocol.Sample) error {
	if s.CPU == nil {
		s.CPU = &protocol.CPU{}
	}
	var sum float64
	var n int
	for _, f := range c.freqFiles {
		if khz, err := readInt(f); err == nil {
			sum += float64(khz)
			n++
		}
	}
	if n > 0 {
		mhz := sum / float64(n) / 1000
		s.CPU.FreqMHz = &mhz
	}
	if c.tempFile != "" {
		if milli, err := readInt(c.tempFile); err == nil {
			t := float64(milli) / 1000
			s.CPU.TempC = &t
		}
	}
	return nil
}

// --- power supplies ---

// powerState reports whether the machine has a battery and whether it is on
// external power. Machines without a battery always count as on AC.
func powerState(root string) (hasBattery, onAC bool) {
	dirs, _ := filepath.Glob(filepath.Join(root, "sys/class/power_supply/*"))
	for _, d := range dirs {
		typ, _ := readStr(filepath.Join(d, "type"))
		switch typ {
		case "Battery":
			hasBattery = true
		case "Mains", "USB":
			if v, _ := readStr(filepath.Join(d, "online")); v == "1" {
				onAC = true
			}
		}
	}
	if !hasBattery {
		onAC = true
	}
	return hasBattery, onAC
}

type batteryCollector struct{ dir string }

// NewBattery reports charge, state and charge/discharge power of the first battery.
func NewBattery(root string) (Collector, bool) {
	dirs, _ := filepath.Glob(filepath.Join(root, "sys/class/power_supply/*"))
	for _, d := range dirs {
		if typ, _ := readStr(filepath.Join(d, "type")); typ == "Battery" {
			return &batteryCollector{dir: d}, true
		}
	}
	return nil, false
}

func (c *batteryCollector) Name() string { return "battery" }

func (c *batteryCollector) Collect(_ context.Context, s *protocol.Sample) error {
	pct, err := readInt(filepath.Join(c.dir, "capacity"))
	if err != nil {
		return err
	}
	status, _ := readStr(filepath.Join(c.dir, "status"))
	b := &protocol.Battery{Percent: float64(pct), State: strings.ToLower(status)}

	var watts float64
	have := false
	if uw, err := readInt(filepath.Join(c.dir, "power_now")); err == nil {
		watts, have = float64(uw)/1e6, true
	} else if ua, err1 := readInt(filepath.Join(c.dir, "current_now")); err1 == nil {
		if uv, err2 := readInt(filepath.Join(c.dir, "voltage_now")); err2 == nil {
			watts, have = float64(ua)*float64(uv)/1e12, true
		}
	}
	if have {
		if watts < 0 {
			watts = -watts
		}
		switch b.State {
		case "discharging":
			watts = -watts
		case "charging":
		default:
			watts = 0
		}
		b.Watts = &watts
	}
	s.Battery = b
	return nil
}

// --- RAPL CPU and platform power ---

var raplTop = regexp.MustCompile(`^intel-rapl:\d+$`)

type raplDomain struct {
	energyFile string
	max        uint64
	platform   bool // psys rather than a CPU package
}

type raplCollector struct {
	domains []raplDomain
	prev    map[string]uint64
	prevAt  int64 // unix nanos of previous reading, 0 if none
}

// NewRAPL reports CPU package power (and platform power where the psys
// domain exists) from energy counters. Unreadable counters still count as
// detected, so the reason shows up in the sample.
func NewRAPL(root string) (Collector, bool) {
	dirs, _ := filepath.Glob(filepath.Join(root, "sys/class/powercap/intel-rapl:*"))
	c := &raplCollector{prev: map[string]uint64{}}
	for _, d := range dirs {
		if !raplTop.MatchString(filepath.Base(d)) {
			continue
		}
		name, err := readStr(filepath.Join(d, "name"))
		if err != nil {
			continue
		}
		dom := raplDomain{energyFile: filepath.Join(d, "energy_uj")}
		switch {
		case strings.HasPrefix(name, "package"):
		case name == "psys":
			dom.platform = true
		default:
			continue
		}
		dom.max, _ = readUint(filepath.Join(d, "max_energy_range_uj"))
		c.domains = append(c.domains, dom)
	}
	return c, len(c.domains) > 0
}

// raplDelta returns the energy used between two counter readings, allowing
// for one wrap of the counter at max.
func raplDelta(prev, cur, max uint64) uint64 {
	if cur >= prev {
		return cur - prev
	}
	return max - prev + cur
}

func (c *raplCollector) Name() string { return "power.cpu" }

func (c *raplCollector) Collect(_ context.Context, s *protocol.Sample) error {
	now := s.Time.UnixNano()
	secs := float64(now-c.prevAt) / 1e9
	first := c.prevAt == 0 || secs <= 0
	var cpuW, platW float64
	var haveCPU, havePlat bool
	for _, d := range c.domains {
		cur, err := readUint(d.energyFile)
		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				return fmt.Errorf("%w (CPU power needs root)", err)
			}
			return err
		}
		prev, seen := c.prev[d.energyFile]
		c.prev[d.energyFile] = cur
		if first || !seen {
			continue
		}
		w := float64(raplDelta(prev, cur, d.max)) / 1e6 / secs
		if d.platform {
			platW += w
			havePlat = true
		} else {
			cpuW += w
			haveCPU = true
		}
	}
	c.prevAt = now
	if !haveCPU && !havePlat {
		return nil
	}
	p := &protocol.Power{}
	if haveCPU {
		p.CPUWatts = &cpuW
	}
	if havePlat {
		p.PlatformWatts = &platW
	}
	s.Power = p
	return nil
}

// nvidiaAsleep reports whether any NVIDIA PCI device is runtime-suspended,
// in which case querying it would wake it up.
func nvidiaAsleep(root string) bool {
	devs, _ := filepath.Glob(filepath.Join(root, "sys/bus/pci/devices/*"))
	for _, d := range devs {
		if v, _ := readStr(filepath.Join(d, "vendor")); v != "0x10de" {
			continue
		}
		if st, _ := readStr(filepath.Join(d, "power/runtime_status")); st == "suspended" {
			return true
		}
	}
	return false
}

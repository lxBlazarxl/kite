package agent

import (
	"context"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

// gpuCounterEvery throttles the GPU performance counters if they ever prove
// costly during games: 0 reads them every sample, otherwise the last values
// are reused until this much time has passed.
var gpuCounterEvery time.Duration

const (
	pathGPU3DUtil   = `\GPU Engine(*engtype_3D)\Utilization Percentage`
	pathGPUVRAMUsed = `\GPU Adapter Memory(*)\Dedicated Usage`
)

// nvmlReader is the part of NVML the collector uses (a fake in tests).
type nvmlReader interface {
	read() ([]NVMLReading, error)
}

// nvmlRetry is how long to wait before trying to load NVML again after a
// failure (for example the driver not ready yet early in boot).
const nvmlRetry = time.Minute

type windowsGPU struct {
	adapters  []Adapter
	q         *pdhQuery
	load      func() (nvmlReader, error)
	nvml      nvmlReader
	nvmlTried time.Time
	hasNvidia bool
	readPower func() (PowerStatus, error)
	awake     func() (bool, error)

	lastRead time.Time
	util     map[string]float64
	vram     map[string]float64
}

// NewWindowsGPU reports every GPU's usage and VRAM from Windows performance
// counters (these never wake a sleeping GPU), plus NVIDIA temperature,
// power and details through nvml.dll when on AC and the GPU is awake.
func NewWindowsGPU() (Collector, bool) {
	raw, err := enumAdapters()
	if err != nil {
		return nil, false
	}
	adapters := orderAdapters(raw)
	if len(adapters) == 0 {
		return nil, false
	}
	// NVML is loaded lazily, only once on AC with the GPU awake: its init
	// touches the device, and the library costs about 22 MB.
	load := func() (nvmlReader, error) {
		n, err := loadNVML()
		if err != nil {
			return nil, err
		}
		return n, nil
	}
	c, err := newWindowsGPU(adapters, load, readPowerStatus, nvidiaAwake)
	if err != nil {
		return nil, false
	}
	return c, true
}

func newWindowsGPU(adapters []Adapter, load func() (nvmlReader, error), readPower func() (PowerStatus, error), awake func() (bool, error)) (*windowsGPU, error) {
	q, err := newPDHQuery(pathGPU3DUtil, pathGPUVRAMUsed)
	if err != nil {
		return nil, err
	}
	c := &windowsGPU{adapters: adapters, q: q, load: load, readPower: readPower, awake: awake}
	for _, a := range adapters {
		if a.VendorID == vendorNVIDIA {
			c.hasNvidia = true
		}
	}
	return c, nil
}

func (c *windowsGPU) Name() string { return "gpu" }

func (c *windowsGPU) Collect(_ context.Context, s *protocol.Sample) error {
	if c.util == nil || gpuCounterEvery == 0 || s.Time.Sub(c.lastRead) >= gpuCounterEvery {
		vals, err := c.q.collect()
		if err != nil {
			return err
		}
		c.util, c.vram, c.lastRead = sumByLUID(vals[0], 100), sumByLUID(vals[1], 0), s.Time
	}
	gpus := buildGPUs(c.adapters, c.util, c.vram)
	if c.hasNvidia {
		if why := c.nvidiaSkipReason(s.Time); why != "" {
			setUnavailable(s, "gpu.nvidia", why)
		} else if rs, err := c.nvml.read(); err != nil {
			setUnavailable(s, "gpu.nvidia", err.Error())
		} else {
			applyNVML(gpus, rs)
		}
	}
	s.GPUs = append(s.GPUs, gpus...)
	return nil
}

func (c *windowsGPU) nvidiaSkipReason(now time.Time) string {
	ps, err := c.readPower()
	gate := nvmlGate{OnAC: err != nil || ps.OnAC}
	if awake, err := c.awake(); err == nil {
		gate.Awake = awake
	}
	if why := nvmlSkipReason(gate); why != "" {
		return why
	}
	if c.nvml == nil && (c.nvmlTried.IsZero() || now.Sub(c.nvmlTried) >= nvmlRetry) {
		c.nvmlTried = now
		c.nvml, _ = c.load() // missing driver is fine: usage still comes from PDH
	}
	if c.nvml == nil {
		return "nvml.dll not available"
	}
	return ""
}

func setUnavailable(s *protocol.Sample, key, why string) {
	if s.Unavailable == nil {
		s.Unavailable = map[string]string{}
	}
	s.Unavailable[key] = why
}

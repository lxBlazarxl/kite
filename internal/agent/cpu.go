package agent

import (
	"context"

	"github.com/retransmit/kite/internal/protocol"
	"github.com/shirou/gopsutil/v4/cpu"
)

type cpuCollector struct{ model string }

// NewCPU reports total and per-core CPU usage.
func NewCPU() Collector {
	c := &cpuCollector{}
	if info, err := cpu.Info(); err == nil && len(info) > 0 {
		c.model = info[0].ModelName
	}
	// Prime the usage counters so the first real sample is a short-window delta.
	cpu.Percent(0, true)
	cpu.Percent(0, false)
	return c
}

func (c *cpuCollector) Name() string { return "cpu" }

func (c *cpuCollector) Collect(ctx context.Context, s *protocol.Sample) error {
	per, err := cpu.PercentWithContext(ctx, 0, true)
	if err != nil {
		return err
	}
	total, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return err
	}
	s.CPU = &protocol.CPU{Model: c.model, PerCorePct: per}
	if len(total) > 0 {
		s.CPU.TotalPct = total[0]
	}
	return nil
}

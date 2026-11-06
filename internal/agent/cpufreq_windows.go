package agent

import (
	"context"

	"github.com/retransmit/kite/internal/protocol"
	"github.com/shirou/gopsutil/v4/cpu"
)

type cpuFreqCollector struct {
	baseMHz float64
	q       *pdhQuery
}

// NewCPUFreq reports the current CPU clock on Windows from
// "% Processor Performance" (which can exceed 100 % when boosting) times
// the base clock. It must run after the cpu collector.
func NewCPUFreq() (Collector, bool) {
	info, err := cpu.Info()
	if err != nil || len(info) == 0 || info[0].Mhz <= 0 {
		return nil, false
	}
	q, err := newPDHQuery(`\Processor Information(_Total)\% Processor Performance`)
	if err != nil {
		return nil, false
	}
	return &cpuFreqCollector{baseMHz: info[0].Mhz, q: q}, true
}

func (c *cpuFreqCollector) Name() string { return "cpuextra" }

func (c *cpuFreqCollector) Collect(_ context.Context, s *protocol.Sample) error {
	vals, err := c.q.collect()
	if err != nil {
		return err
	}
	if len(vals) == 0 || len(vals[0]) == 0 {
		return nil
	}
	if s.CPU == nil {
		return nil // the cpu collector failed; never invent a 0 % reading
	}
	mhz := freqFromPerf(vals[0][0].Value, c.baseMHz)
	s.CPU.FreqMHz = &mhz
	return nil
}

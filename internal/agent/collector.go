// Package agent collects machine metrics and sends them to a kite hub.
package agent

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

// Collector fills one part of a sample. Collectors must use s.Time as the
// current time for any rate calculation.
type Collector interface {
	Name() string
	Collect(ctx context.Context, s *protocol.Sample) error
}

// collectorTimeout bounds one collector so a hung source (Docker daemon,
// nvidia-smi) costs only its own section, not the whole sample.
func collectorTimeout(interval time.Duration) time.Duration {
	return min(interval/2, 3*time.Second)
}

// CollectOnce builds one sample by running every collector in order. A
// collector error is recorded in Unavailable under the collector's name.
func CollectOnce(ctx context.Context, cfg Config, cols []Collector, now time.Time) *protocol.Sample {
	host, _ := os.Hostname()
	s := &protocol.Sample{
		Version:     protocol.Version,
		Machine:     cfg.Name,
		OS:          runtime.GOOS,
		Hostname:    host,
		Time:        now,
		IntervalSec: cfg.Interval.Seconds(),
	}
	for _, c := range cols {
		cctx, cancel := context.WithTimeout(ctx, collectorTimeout(cfg.Interval))
		err := c.Collect(cctx, s)
		cancel()
		if err != nil {
			if s.Unavailable == nil {
				s.Unavailable = map[string]string{}
			}
			s.Unavailable[c.Name()] = err.Error()
		}
	}
	return s
}

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
		if err := c.Collect(ctx, s); err != nil {
			if s.Unavailable == nil {
				s.Unavailable = map[string]string{}
			}
			s.Unavailable[c.Name()] = err.Error()
		}
	}
	return s
}

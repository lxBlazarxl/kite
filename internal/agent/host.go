package agent

import (
	"context"

	"github.com/retransmit/kite/internal/protocol"
	"github.com/shirou/gopsutil/v4/host"
)

type hostCollector struct{ platform string }

// NewHost reports uptime and the OS platform (distro or Windows edition).
func NewHost() Collector {
	c := &hostCollector{}
	if info, err := host.Info(); err == nil {
		c.platform = info.Platform
	}
	return c
}

func (c *hostCollector) Name() string { return "host" }

func (c *hostCollector) Collect(ctx context.Context, s *protocol.Sample) error {
	s.Platform = c.platform
	up, err := host.UptimeWithContext(ctx)
	if err != nil {
		return err
	}
	s.UptimeSec = up
	return nil
}

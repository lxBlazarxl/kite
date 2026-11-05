package agent

import (
	"context"

	"github.com/retransmit/kite/internal/protocol"
	"github.com/shirou/gopsutil/v4/mem"
)

type memoryCollector struct{}

// NewMemory reports RAM and swap. Used RAM is total minus available.
func NewMemory() Collector { return memoryCollector{} }

func (memoryCollector) Name() string { return "memory" }

func (memoryCollector) Collect(ctx context.Context, s *protocol.Sample) error {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return err
	}
	m := &protocol.Memory{TotalBytes: vm.Total, UsedBytes: vm.Total - vm.Available}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		m.SwapTotalBytes, m.SwapUsedBytes = sw.Total, sw.Used
	}
	s.Memory = m
	return nil
}

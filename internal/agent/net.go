package agent

import (
	"context"
	"path"
	"sort"

	"github.com/retransmit/kite/internal/protocol"
	"github.com/shirou/gopsutil/v4/net"
)

// matchIface reports whether an interface should be shown: it must match an
// include glob when any are given, and must match no exclude glob.
func matchIface(name string, include, exclude []string) bool {
	if len(include) > 0 && !matchAny(name, include) {
		return false
	}
	return !matchAny(name, exclude)
}

func matchAny(name string, globs []string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(g, name); ok {
			return true
		}
	}
	return false
}

type netCollector struct {
	include, exclude []string
	rates            *Rates
}

// NewNet reports receive and transmit throughput per network interface.
func NewNet(include, exclude []string) Collector {
	return &netCollector{include: include, exclude: exclude, rates: NewRates()}
}

func (c *netCollector) Name() string { return "net" }

func (c *netCollector) Collect(ctx context.Context, s *protocol.Sample) error {
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return err
	}
	for _, n := range counters {
		if !matchIface(n.Name, c.include, c.exclude) {
			continue
		}
		rx, ok1 := c.rates.Rate("net."+n.Name+".rx", n.BytesRecv, s.Time)
		tx, ok2 := c.rates.Rate("net."+n.Name+".tx", n.BytesSent, s.Time)
		if ok1 && ok2 {
			s.Net = append(s.Net, protocol.NetIf{Name: n.Name, RxBps: rx, TxBps: tx})
		}
	}
	sort.Slice(s.Net, func(i, j int) bool { return s.Net[i].Name < s.Net[j].Name })
	return nil
}

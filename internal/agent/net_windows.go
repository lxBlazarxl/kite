package agent

import (
	"context"
	"sort"
	"unsafe"

	"github.com/retransmit/kite/internal/protocol"
	"golang.org/x/sys/windows"
)

type netWindows struct {
	include, exclude []string
	rates            *Rates
}

// NewNetWindows reads all interface counters with one GetIfTable2 call.
// gopsutil's Windows path lists adapters on every call, which blocks for
// about 15 ms; this takes well under a millisecond.
func NewNetWindows(include, exclude []string) Collector {
	return &netWindows{include: include, exclude: exclude, rates: NewRates()}
}

func (c *netWindows) Name() string { return "net" }

func (c *netWindows) Collect(_ context.Context, s *protocol.Sample) error {
	var tab *windows.MibIfTable2
	if err := windows.GetIfTable2Ex(windows.MibIfEntryNormal, &tab); err != nil {
		return err
	}
	defer windows.FreeMibTable(unsafe.Pointer(tab))
	if tab.NumEntries == 0 {
		return nil
	}
	for _, r := range unsafe.Slice(&tab.Table[0], tab.NumEntries) {
		if !ifRowUsable(r.InterfaceAndOperStatusFlags, r.OperStatus, r.Type) {
			continue
		}
		name := windows.UTF16ToString(r.Alias[:])
		if !matchIface(name, c.include, c.exclude) {
			continue
		}
		rx, ok1 := c.rates.Rate("net."+name+".rx", r.InOctets, s.Time)
		tx, ok2 := c.rates.Rate("net."+name+".tx", r.OutOctets, s.Time)
		if ok1 && ok2 {
			s.Net = append(s.Net, protocol.NetIf{Name: name, RxBps: rx, TxBps: tx})
		}
	}
	sort.Slice(s.Net, func(i, j int) bool { return s.Net[i].Name < s.Net[j].Name })
	return nil
}

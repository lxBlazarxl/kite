package agent

import (
	"context"
	"time"

	"github.com/retransmit/kite/internal/protocol"
	"github.com/yusufpapurcu/wmi"
)

const wmiEvery = 30 * time.Second

type windowsBattery struct {
	readPower func() (PowerStatus, error)
	wmi       *cached[wmiBatteryStatus]
}

// NewWindowsBattery reports charge every sample (cheap kernel call) and
// charge/discharge watts from WMI, refreshed every 30 s.
func NewWindowsBattery() (Collector, bool) {
	ps, err := readPowerStatus()
	if err != nil || !ps.HasBattery {
		return nil, false
	}
	return newWindowsBattery(readPowerStatus, queryBatteryStatus), true
}

func newWindowsBattery(readPower func() (PowerStatus, error), fetch func() (wmiBatteryStatus, error)) *windowsBattery {
	return &windowsBattery{readPower: readPower, wmi: newAsyncCached(wmiEvery, fetch)}
}

func queryBatteryStatus() (wmiBatteryStatus, error) {
	var rows []wmiBatteryStatus
	err := wmi.QueryNamespace("SELECT PowerOnline, Charging, Discharging, ChargeRate, DischargeRate FROM BatteryStatus", &rows, `root\wmi`)
	if err == nil && len(rows) == 0 {
		err = errNoRows
	}
	if err != nil {
		return wmiBatteryStatus{}, err
	}
	return rows[0], nil
}

func (b *windowsBattery) Name() string { return "battery" }

func (b *windowsBattery) Collect(_ context.Context, s *protocol.Sample) error {
	ps, err := b.readPower()
	if err != nil {
		return err
	}
	if !ps.HasBattery || ps.Percent == nil {
		return nil
	}
	bat := &protocol.Battery{Percent: *ps.Percent}
	if st, ok := b.wmi.get(s.Time); ok {
		state, w := batteryFromWMI(st)
		bat.State, bat.Watts = state, &w
	} else {
		switch {
		case !ps.OnAC:
			bat.State = "discharging"
		case *ps.Percent >= 100:
			bat.State = "full"
		default:
			bat.State = "charging"
		}
	}
	s.Battery = bat
	return nil
}

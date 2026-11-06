package agent

import (
	"context"
	"errors"

	"github.com/retransmit/kite/internal/protocol"
	"github.com/yusufpapurcu/wmi"
)

var errNoRows = errors.New("wmi: no rows")

type wmiThermalZone struct{ CurrentTemperature uint32 }

type systemTemp struct{ wmi *cached[float64] }

// NewSystemTemp reports the first ACPI thermal zone (a motherboard sensor;
// Windows exposes no CPU temperature without vendor drivers), refreshed
// every 30 s. Shown as "System", never as CPU.
func NewSystemTemp() (Collector, bool) {
	c := &systemTemp{wmi: newAsyncCached(wmiEvery, queryThermalZone)}
	if _, err := queryThermalZone(); err != nil {
		return nil, false
	}
	return c, true
}

func queryThermalZone() (float64, error) {
	var rows []wmiThermalZone
	if err := wmi.QueryNamespace("SELECT CurrentTemperature FROM MSAcpi_ThermalZoneTemperature", &rows, `root\wmi`); err != nil {
		return 0, err
	}
	if len(rows) == 0 || rows[0].CurrentTemperature == 0 {
		return 0, errNoRows
	}
	return kelvinTenthsToC(rows[0].CurrentTemperature), nil
}

func (c *systemTemp) Name() string { return "thermal" }

func (c *systemTemp) Collect(_ context.Context, s *protocol.Sample) error {
	if v, ok := c.wmi.get(s.Time); ok {
		s.SystemTempC = &v
	}
	return nil
}

package agent

// wmiBatteryStatus mirrors the root\wmi BatteryStatus fields we read.
type wmiBatteryStatus struct {
	PowerOnline   bool
	Charging      bool
	Discharging   bool
	ChargeRate    int32 // mW
	DischargeRate int32 // mW
}

// batteryFromWMI gives the battery state and watts (negative while
// discharging, positive while charging).
func batteryFromWMI(b wmiBatteryStatus) (state string, watts float64) {
	switch {
	case b.Discharging:
		return "discharging", -float64(b.DischargeRate) / 1000
	case b.Charging:
		return "charging", float64(b.ChargeRate) / 1000
	case b.PowerOnline:
		return "full", 0
	}
	return "idle", 0
}

// kelvinTenthsToC converts ACPI thermal zone readings (tenths of kelvin).
func kelvinTenthsToC(v uint32) float64 {
	return float64(v)/10 - 273.15
}

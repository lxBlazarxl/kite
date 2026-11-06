package agent

// PowerStatus is the machine's power source and battery charge.
type PowerStatus struct {
	HasBattery bool
	OnAC       bool
	Percent    *float64
}

// decodePowerStatus interprets the fields of Windows SYSTEM_POWER_STATUS:
// battery flag 128 means "no system battery" and 255 "unknown"; a machine
// without a battery always counts as on AC.
func decodePowerStatus(acLineStatus, batteryFlag, lifePercent byte) PowerStatus {
	ps := PowerStatus{HasBattery: batteryFlag != 128 && batteryFlag != 255}
	ps.OnAC = acLineStatus == 1 || !ps.HasBattery
	if ps.HasBattery && lifePercent <= 100 {
		p := float64(lifePercent)
		ps.Percent = &p
	}
	return ps
}

package agent

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetSystemPowerStatus = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

// systemPowerStatus mirrors SYSTEM_POWER_STATUS.
type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// readPowerStatus is cheap (no WMI), so it runs every sample.
func readPowerStatus() (PowerStatus, error) {
	var st systemPowerStatus
	if r, _, err := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&st))); r == 0 {
		return PowerStatus{}, err
	}
	return decodePowerStatus(st.ACLineStatus, st.BatteryFlag, st.BatteryLifePercent), nil
}

func powerState(root string) (hasBattery, onAC bool) {
	ps, err := readPowerStatus()
	if err != nil {
		return false, true
	}
	return ps.HasBattery, ps.OnAC
}

// nvidiaAsleep is not used on Windows: the Windows GPU collector checks the
// device power state itself.
func nvidiaAsleep(root string) bool { return false }

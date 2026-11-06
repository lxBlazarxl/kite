package agent

import "encoding/binary"

// Device power states from DEVICE_POWER_STATE.
const powerDeviceD0 = 1

// powerStateFromData reads PD_MostRecentPowerState from a CM_POWER_DATA
// buffer (PD_Size uint32, then the state uint32).
func powerStateFromData(b []byte) (uint32, bool) {
	if len(b) < 8 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b[4:8]), true
}

// NVMLReading is what NVML reports for one NVIDIA GPU. Nil fields failed
// to read.
type NVMLReading struct {
	Name      string
	UtilPct   *float64
	VRAMUsed  *uint64
	VRAMTotal *uint64
	TempC     *float64
	PowerW    *float64
}

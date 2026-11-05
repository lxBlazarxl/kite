//go:build !linux

package agent

// powerState on non-Linux systems is not implemented yet (Plan 2 adds
// Windows); report "no battery, on AC" so nothing is gated.
func powerState(root string) (hasBattery, onAC bool) { return false, true }

func nvidiaAsleep(root string) bool { return false }

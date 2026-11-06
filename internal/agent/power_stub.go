//go:build !linux && !windows

package agent

// powerState on other systems (macOS and the like) is not implemented:
// report "no battery, on AC" so nothing is gated.
func powerState(root string) (hasBattery, onAC bool) { return false, true }

func nvidiaAsleep(root string) bool { return false }

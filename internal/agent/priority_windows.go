package agent

import "golang.org/x/sys/windows"

// LowerPriority runs the agent below normal priority so a game or build
// always wins the CPU over monitoring.
func LowerPriority() error {
	return windows.SetPriorityClass(windows.CurrentProcess(), windows.BELOW_NORMAL_PRIORITY_CLASS)
}

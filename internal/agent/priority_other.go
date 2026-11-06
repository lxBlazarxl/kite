//go:build !windows

package agent

// LowerPriority is a no-op outside Windows (systemd sets Nice= instead).
func LowerPriority() error { return nil }

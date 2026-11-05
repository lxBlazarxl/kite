//go:build !linux

package agent

// Detect returns the collectors available on this machine, in sample order.
func Detect(cfg Config) []Collector {
	return []Collector{NewCPU(), NewMemory()}
}

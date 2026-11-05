//go:build linux

package agent

// Detect returns the collectors available on this machine, in sample order.
func Detect(cfg Config) []Collector {
	cols := []Collector{NewCPU()}
	if c, ok := NewCPUExtra(cfg.Root); ok {
		cols = append(cols, c)
	}
	cols = append(cols, NewMemory(), NewDisk(), NewNet(cfg.NetInclude, cfg.NetExclude))
	if c, ok := NewBattery(cfg.Root); ok {
		cols = append(cols, c)
	}
	if c, ok := NewRAPL(cfg.Root); ok {
		cols = append(cols, c)
	}
	return cols
}

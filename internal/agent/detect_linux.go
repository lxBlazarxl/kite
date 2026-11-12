//go:build linux

package agent

// Detect returns the collectors available on this machine, in sample order.
func Detect(cfg Config) []Collector {
	cols := []Collector{NewHost(), NewCPU()}
	if c, ok := NewCPUExtra(cfg.Root); ok {
		cols = append(cols, c)
	}
	cols = append(cols, NewMemory(), NewDiskAt(cfg.HostRoot), NewNet(cfg.NetInclude, cfg.NetExclude))
	if c, ok := NewBattery(cfg.Root); ok {
		cols = append(cols, c)
	}
	if c, ok := NewRAPL(cfg.Root); ok {
		cols = append(cols, c)
	}
	if c, ok := NewAMDGPU(cfg.Root); ok {
		cols = append(cols, c)
	}
	if c, ok := NewNvidia(cfg.Root); ok {
		cols = append(cols, c)
	}
	if c, ok := NewDocker(cfg.DockerHost); ok {
		cols = append(cols, c)
	}
	cols = append(cols, NewSystemd(cfg.HostRoot))
	return cols
}

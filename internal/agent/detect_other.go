//go:build !linux && !windows

package agent

// Detect returns the collectors available on this machine, in sample order.
func Detect(cfg Config) []Collector {
	cols := []Collector{NewHost(), NewCPU(), NewMemory(), NewDisk(), NewNet(cfg.NetInclude, cfg.NetExclude)}
	if c, ok := NewNvidia(cfg.Root); ok {
		cols = append(cols, c)
	}
	if c, ok := NewDocker(cfg.DockerHost); ok {
		cols = append(cols, c)
	}
	return cols
}

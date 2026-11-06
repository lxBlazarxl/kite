package agent

import "time"

// Detect returns the collectors available on this machine, in sample order.
func Detect(cfg Config) []Collector {
	cols := []Collector{NewHost(), NewCPU()}
	if c, ok := NewCPUFreq(); ok {
		cols = append(cols, c)
	}
	cols = append(cols, NewMemory(), NewDisk(), NewNetWindows(cfg.NetInclude, cfg.NetExclude))
	if c, ok := NewWindowsBattery(); ok {
		cols = append(cols, c)
	}
	if c, ok := NewSystemTemp(); ok {
		cols = append(cols, c)
	}
	if c, ok := NewWindowsGPU(); ok {
		cols = append(cols, c)
	}
	if c, ok := NewDocker(cfg.DockerHost); ok {
		if path, isPipe := npipePath(cfg.DockerHost); isPipe {
			c = newLazyDocker(c, func() bool { return pipeExists(path) }, 60*time.Second, time.Now)
			c = newBackgroundDocker(c, 15*time.Second)
		}
		cols = append(cols, c)
	}
	return cols
}

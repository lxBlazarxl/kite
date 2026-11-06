package agent

// Detect returns the collectors available on this machine, in sample order.
func Detect(cfg Config) []Collector {
	cols := []Collector{NewHost(), NewCPU()}
	if c, ok := NewCPUFreq(); ok {
		cols = append(cols, c)
	}
	cols = append(cols, NewMemory(), NewDisk(), NewNet(cfg.NetInclude, cfg.NetExclude))
	if c, ok := NewWindowsGPU(); ok {
		cols = append(cols, c)
	}
	return cols
}

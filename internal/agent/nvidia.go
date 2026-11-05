package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/retransmit/kite/internal/protocol"
)

const nvidiaQuery = "--query-gpu=index,name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw"

type runFunc func(ctx context.Context) ([]byte, error)

type nvidiaCollector struct {
	root string
	run  runFunc
}

// NewNvidia reports NVIDIA GPUs through nvidia-smi when it is installed.
func NewNvidia(root string) (Collector, bool) {
	bin, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil, false
	}
	run := func(ctx context.Context) ([]byte, error) {
		return exec.CommandContext(ctx, bin, nvidiaQuery, "--format=csv,noheader,nounits").Output()
	}
	return newNvidia(root, run), true
}

func newNvidia(root string, run runFunc) Collector { return &nvidiaCollector{root: root, run: run} }

func (c *nvidiaCollector) Name() string { return "gpu.nvidia" }

func (c *nvidiaCollector) Collect(ctx context.Context, s *protocol.Sample) error {
	// Querying a sleeping laptop dGPU wakes it and drains the battery.
	if hasBat, onAC := powerState(c.root); hasBat && !onAC {
		return c.skip(s, "on battery")
	}
	if nvidiaAsleep(c.root) {
		return c.skip(s, "asleep")
	}
	out, err := c.run(ctx)
	if err != nil {
		return fmt.Errorf("nvidia-smi: %w", err)
	}
	gpus, err := parseNvidiaCSV(out)
	if err != nil {
		return err
	}
	for _, g := range gpus {
		g.Index = len(s.GPUs)
		s.GPUs = append(s.GPUs, g)
	}
	return nil
}

func (c *nvidiaCollector) skip(s *protocol.Sample, why string) error {
	if s.Unavailable == nil {
		s.Unavailable = map[string]string{}
	}
	s.Unavailable[c.Name()] = why
	return nil
}

// parseNvidiaCSV parses nvidia-smi output for nvidiaQuery with
// --format=csv,noheader,nounits. "[N/A]" fields become nil.
func parseNvidiaCSV(out []byte) ([]protocol.GPU, error) {
	var gpus []protocol.GPU
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, ",")
		if len(f) != 7 {
			return nil, fmt.Errorf("nvidia-smi: want 7 fields, got %d in %q", len(f), line)
		}
		for i := range f {
			f[i] = strings.TrimSpace(f[i])
		}
		num := func(s string) *float64 {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return nil
			}
			return &v
		}
		mib := func(s string) *uint64 {
			v := num(s)
			if v == nil {
				return nil
			}
			b := uint64(*v) * 1048576
			return &b
		}
		gpus = append(gpus, protocol.GPU{
			Vendor:         "nvidia",
			Name:           f[1],
			UtilPct:        num(f[2]),
			VRAMUsedBytes:  mib(f[3]),
			VRAMTotalBytes: mib(f[4]),
			TempC:          num(f[5]),
			PowerW:         num(f[6]),
		})
	}
	return gpus, nil
}

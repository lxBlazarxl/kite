package agent

import (
	"context"
	"testing"

	"github.com/retransmit/kite/internal/protocol"
)

const nvidiaRoot = "testdata/sysfs/nvidia-laptop"

func countingRun(calls *int) runFunc {
	return func(context.Context) ([]byte, error) {
		*calls++
		return []byte(nvidiaCSV), nil
	}
}

func TestNvidiaSkippedOnBattery(t *testing.T) {
	calls := 0
	s := collect(t, newNvidia(nvidiaRoot, countingRun(&calls)), &protocol.Sample{})
	if calls != 0 || s.Unavailable["gpu.nvidia"] != "on battery" || len(s.GPUs) != 0 {
		t.Fatalf("calls %d unavailable %v gpus %v", calls, s.Unavailable, s.GPUs)
	}
}

func TestNvidiaAsleep(t *testing.T) {
	root := copyTree(t, nvidiaRoot)
	write(t, root, "sys/class/power_supply/AC/online", "1")
	write(t, root, "sys/bus/pci/devices/0000:01:00.0/power/runtime_status", "suspended")
	calls := 0
	s := collect(t, newNvidia(root, countingRun(&calls)), &protocol.Sample{})
	if calls != 0 || s.Unavailable["gpu.nvidia"] != "asleep" {
		t.Fatalf("calls %d unavailable %v", calls, s.Unavailable)
	}
}

func TestNvidiaOnAC(t *testing.T) {
	root := copyTree(t, nvidiaRoot)
	write(t, root, "sys/class/power_supply/AC/online", "1")
	calls := 0
	s := &protocol.Sample{GPUs: []protocol.GPU{{Index: 0, Vendor: "amd"}}}
	s = collect(t, newNvidia(root, countingRun(&calls)), s)
	if calls != 1 || len(s.GPUs) != 3 || s.GPUs[1].Index != 1 || s.GPUs[2].Index != 2 || s.GPUs[1].Vendor != "nvidia" {
		t.Fatalf("calls %d gpus %+v", calls, s.GPUs)
	}
}

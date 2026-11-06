package agent

import (
	"context"
	"testing"
	"time"
)

func TestDiskRealWindows(t *testing.T) {
	cfg := Config{Name: "pc", Interval: time.Second}
	cols := []Collector{NewDisk()}
	CollectOnce(context.Background(), cfg, cols, time.Now())
	time.Sleep(1100 * time.Millisecond)
	s := CollectOnce(context.Background(), cfg, cols, time.Now())
	for _, d := range s.Disks {
		if d.Mount == "C:" {
			if d.TotalBytes == 0 || d.ReadBps == nil {
				t.Fatalf("C: incomplete: %+v", d)
			}
			return
		}
	}
	t.Fatalf("no C: in %+v (unavailable %v)", s.Disks, s.Unavailable)
}

package agent

import (
	"strings"
	"testing"
	"time"
)

func TestNVMLReal(t *testing.T) {
	n, err := loadNVML()
	if err != nil {
		t.Skipf("no NVIDIA driver: %v", err)
	}
	rs, err := n.read()
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) == 0 || !strings.Contains(rs[0].Name, "NVIDIA") || rs[0].VRAMTotal == nil || *rs[0].VRAMTotal == 0 {
		t.Fatalf("got %+v", rs)
	}
	t.Logf("%s temp %v power %v util %v", rs[0].Name, deref(rs[0].TempC), deref(rs[0].PowerW), deref(rs[0].UtilPct))
}

func TestNvidiaAwakeReal(t *testing.T) {
	awake, err := nvidiaAwake()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("nvidia awake: %v", awake)
}

func TestNvidiaAwakeDoesNotWake(t *testing.T) {
	awake, err := nvidiaAwake()
	if err != nil || awake {
		t.Skipf("GPU awake (%v) or error (%v): cannot check that polling keeps it asleep", awake, err)
	}
	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
		nvidiaAwake()
	}
	if awake, _ := nvidiaAwake(); awake {
		t.Fatal("polling the power state woke the GPU")
	}
}

func deref(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

package protocol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func fp(v float64) *float64 { return &v }
func up(v uint64) *uint64   { return &v }

func TestFlattenKeys(t *testing.T) {
	s := &Sample{
		CPU:     &CPU{TotalPct: 12.5, PerCorePct: []float64{10, 15}, TempC: fp(55)},
		Memory:  &Memory{TotalBytes: 8e9, UsedBytes: 4e9},
		GPUs:    []GPU{{Index: 0, UtilPct: fp(30), PowerW: fp(12.5)}},
		Power:   &Power{CPUWatts: fp(9)},
		Battery: &Battery{Percent: 80, State: "discharging", Watts: fp(-7.5)},
		Disks:   []Disk{{Mount: "/data", UsedBytes: 100, TotalBytes: 200, ReadBps: fp(1024)}},
		Net:     []NetIf{{Name: "wlan0", RxBps: 500, TxBps: 250}},
		Docker:  &Docker{Containers: []Container{{Name: "proxy", State: "running", CPUPct: fp(3), MemBytes: up(1000)}}},
	}
	want := map[string]float64{
		"cpu.total": 12.5, "cpu.core.0": 10, "cpu.core.1": 15, "cpu.temp_c": 55,
		"mem.total": 8e9, "mem.used": 4e9, "swap.total": 0, "swap.used": 0,
		"gpu.0.util": 30, "gpu.0.power_w": 12.5, "power.cpu_w": 9,
		"battery.pct": 80, "battery.w": -7.5,
		"disk./data.used": 100, "disk./data.total": 200, "disk./data.read_bps": 1024,
		"net.wlan0.rx_bps": 500, "net.wlan0.tx_bps": 250,
		"docker.proxy.running": 1, "docker.proxy.cpu_pct": 3, "docker.proxy.mem": 1000,
	}
	got := map[string]float64{}
	for _, p := range Flatten(s) {
		got[p.Key] = p.Value
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestFlattenStoppedContainer(t *testing.T) {
	s := &Sample{Docker: &Docker{Containers: []Container{{Name: "old", State: "exited"}}}}
	pts := Flatten(s)
	if len(pts) != 1 || pts[0].Key != "docker.old.running" || pts[0].Value != 0 {
		t.Fatalf("got %v", pts)
	}
}

func TestFlattenEmpty(t *testing.T) {
	if n := len(Flatten(&Sample{})); n != 0 {
		t.Fatalf("want 0 points, got %d", n)
	}
}

func TestValidMachineName(t *testing.T) {
	for _, n := range []string{"server-01", "my-pc.lan", "PC_2"} {
		if !ValidMachineName(n) {
			t.Errorf("%q should be valid", n)
		}
	}
	for _, n := range []string{"", "a b", "../x", "_x", strings.Repeat("a", 65)} {
		if ValidMachineName(n) {
			t.Errorf("%q should be invalid", n)
		}
	}
}

func TestSampleJSONOmitsNil(t *testing.T) {
	s := Sample{CPU: &CPU{TotalPct: 1}, Unavailable: map[string]string{"docker": "no socket"}}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	if strings.Contains(js, "temp_c") || strings.Contains(js, "gpus") {
		t.Fatalf("nil fields leaked: %s", js)
	}
	if !strings.Contains(js, `"unavailable"`) {
		t.Fatalf("unavailable missing: %s", js)
	}
	var back Sample
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Unavailable["docker"] != "no socket" {
		t.Fatalf("round trip lost unavailable: %v", back.Unavailable)
	}
}

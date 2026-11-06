package agent

import (
	"context"
	"testing"
	"time"
)

func TestMatchIface(t *testing.T) {
	ex := splitList(defaultNetExclude("linux"))
	for _, n := range []string{"lo", "veth69bb6c5", "br-0ad566af6b5e", "docker0", "virbr0"} {
		if matchIface(n, nil, ex) {
			t.Errorf("%s should be hidden", n)
		}
	}
	for _, n := range []string{"tailscale0", "wg0", "wlan0", "enx001122334455"} {
		if !matchIface(n, nil, ex) {
			t.Errorf("%s should be shown", n)
		}
	}
	if matchIface("eth0", []string{"wl*"}, ex) || !matchIface("wlan0", []string{"wl*"}, ex) {
		t.Error("include list not applied")
	}
}

func TestNetReal(t *testing.T) {
	cfg := Config{Name: "pc", Interval: time.Second, Root: "/"}
	cols := []Collector{NewNet(nil, splitList(defaultNetExclude("linux")))}
	CollectOnce(context.Background(), cfg, cols, time.Now())
	time.Sleep(200 * time.Millisecond)
	s := CollectOnce(context.Background(), cfg, cols, time.Now())
	if len(s.Net) == 0 {
		t.Fatal("no interfaces reported")
	}
	for _, n := range s.Net {
		if n.Name == "lo" {
			t.Fatal("lo must be filtered")
		}
	}
}

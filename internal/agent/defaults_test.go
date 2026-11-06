package agent

import (
	"slices"
	"testing"
)

func TestDefaultsPerOS(t *testing.T) {
	if got := defaultDockerHost("windows"); got != "npipe:////./pipe/docker_engine" {
		t.Errorf("windows docker host %q", got)
	}
	if got := defaultDockerHost("linux"); got != "unix:///var/run/docker.sock" {
		t.Errorf("linux docker host %q", got)
	}
	win := splitList(defaultNetExclude("windows"))
	for _, want := range []string{"Loopback*", "Local Area Connection*", "vEthernet*", "isatap*", "Teredo*", "Bluetooth*"} {
		if !slices.Contains(win, want) {
			t.Errorf("windows exclude missing %q: %v", want, win)
		}
	}
	if got := defaultNetExclude("linux"); got != "lo,veth*,br-*,docker0,virbr*" {
		t.Errorf("linux exclude changed: %q", got)
	}
}

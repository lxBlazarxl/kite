package agent

import (
	"os"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfigDefaults(t *testing.T) {
	c, err := LoadConfig([]string{"-once"}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	host, _ := os.Hostname()
	if c.Interval != 5*time.Second || c.Name != host || len(c.NetExclude) != 5 ||
		c.DockerHost != "unix:///var/run/docker.sock" || c.Root != "/" || !c.Once {
		t.Fatalf("defaults wrong: %+v", c)
	}
}

func TestLoadConfigFlagBeatsEnv(t *testing.T) {
	c, err := LoadConfig([]string{"-once", "-name", "flagname"}, env(map[string]string{"AGENT_NAME": "envname"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "flagname" {
		t.Fatalf("name %q", c.Name)
	}
}

func TestLoadConfigEnv(t *testing.T) {
	c, err := LoadConfig(nil, env(map[string]string{
		"HUB_URL": "http://hub:8420", "AGENT_TOKEN": "tok", "AGENT_NAME": "pc",
		"INTERVAL": "10s", "NET_INCLUDE": "wl*, en*", "DOCKER_HOST": "tcp://x:1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.HubURL != "http://hub:8420" || c.Token != "tok" || c.Name != "pc" || c.Interval != 10*time.Second ||
		len(c.NetInclude) != 2 || c.NetInclude[1] != "en*" || c.DockerHost != "tcp://x:1" {
		t.Fatalf("env not applied: %+v", c)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	cases := [][]string{
		{},
		{"-once", "-name", "a b"},
		{"-once", "-interval", "500ms"},
	}
	for _, args := range cases {
		if _, err := LoadConfig(args, env(nil)); err == nil {
			t.Errorf("args %v: want error", args)
		}
	}
}

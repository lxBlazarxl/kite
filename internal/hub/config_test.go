package hub

import (
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestHubConfigDefaults(t *testing.T) {
	c, err := LoadConfig(nil, env(map[string]string{"AGENT_TOKEN": "tok"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":8420" || c.DBPath != "kite.db" || c.RawRetention != 24*time.Hour ||
		c.RollupRetention != 168*time.Hour || c.AgentToken != "tok" || c.WebPassword != "" {
		t.Fatalf("got %+v", c)
	}
}

func TestHubConfigOverrides(t *testing.T) {
	c, err := LoadConfig([]string{"-listen", ":9000", "-raw-retention", "12h"},
		env(map[string]string{"AGENT_TOKEN": "tok", "HUB_LISTEN": ":1", "DB_PATH": "/data/k.db", "WEB_PASSWORD": "pw", "ROLLUP_RETENTION": "336h"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":9000" || c.DBPath != "/data/k.db" || c.WebPassword != "pw" ||
		c.RawRetention != 12*time.Hour || c.RollupRetention != 336*time.Hour {
		t.Fatalf("got %+v", c)
	}
}

func TestHubConfigErrors(t *testing.T) {
	if _, err := LoadConfig(nil, env(nil)); err == nil {
		t.Error("missing token must fail")
	}
	if _, err := LoadConfig([]string{"-tls-cert", "c.pem"}, env(map[string]string{"AGENT_TOKEN": "t"})); err == nil {
		t.Error("cert without key must fail")
	}
	if _, err := LoadConfig([]string{"-raw-retention", "nope"}, env(map[string]string{"AGENT_TOKEN": "t"})); err == nil {
		t.Error("bad duration must fail")
	}
}

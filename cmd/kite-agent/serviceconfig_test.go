package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInstallArgsToEnv(t *testing.T) {
	env, err := installArgsToEnv([]string{"-hub", "http://h:8420", "-token", "t", "-name", "pc-01"})
	if err != nil {
		t.Fatal(err)
	}
	if env["HUB_URL"] != "http://h:8420" || env["AGENT_TOKEN"] != "t" || env["AGENT_NAME"] != "pc-01" || len(env) != 3 {
		t.Fatalf("got %v", env)
	}
	if _, err := installArgsToEnv([]string{"-hub", "http://h:8420"}); err == nil {
		t.Error("missing token must fail")
	}
	if _, err := installArgsToEnv([]string{"-hub", "http://h:8420", "-token", "t", "-name", "a b"}); err == nil {
		t.Error("bad name must fail")
	}
}

func TestServiceConfigFromEnvFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "agent.env")
	os.WriteFile(p, []byte("HUB_URL=http://h:8420\nAGENT_TOKEN=t\nAGENT_NAME=pc-01\n"), 0o600)
	cfg, err := serviceConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HubURL != "http://h:8420" || cfg.Token != "t" || cfg.Name != "pc-01" || cfg.Interval != 5*time.Second {
		t.Fatalf("got %+v", cfg)
	}
}

func TestServiceConfigMissingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "agent.env")
	_, err := serviceConfig(p)
	if err == nil || !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), "install") {
		t.Fatalf("got %v", err)
	}
}

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/retransmit/kite/internal/agent"
)

// installArgsToEnv turns `install` flags into agent.env entries and checks
// they make a valid agent config before anything is written.
func installArgsToEnv(args []string) (map[string]string, error) {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	hub := fs.String("hub", "", "hub URL")
	token := fs.String("token", "", "agent token")
	name := fs.String("name", "", "machine name (default: hostname)")
	interval := fs.String("interval", "", "sample interval (default 5s)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *hub == "" || *token == "" {
		return nil, errors.New("usage: kite-agent install -hub <url> -token <token> [-name <name>] [-interval 5s]")
	}
	env := map[string]string{"HUB_URL": *hub, "AGENT_TOKEN": *token}
	if *name != "" {
		env["AGENT_NAME"] = *name
	}
	if *interval != "" {
		env["INTERVAL"] = *interval
	}
	if _, err := agent.LoadConfig(nil, lookup(env)); err != nil {
		return nil, err
	}
	return env, nil
}

// serviceConfig loads the agent config from the env file written by install.
func serviceConfig(envPath string) (agent.Config, error) {
	f, err := os.Open(envPath)
	if err != nil {
		return agent.Config{}, fmt.Errorf("read %s: %w (run kite-agent.exe install first)", envPath, err)
	}
	defer f.Close()
	env, err := agent.ReadEnvFile(f)
	if err != nil {
		return agent.Config{}, fmt.Errorf("read %s: %w", envPath, err)
	}
	return agent.LoadConfig(nil, lookup(env))
}

func lookup(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

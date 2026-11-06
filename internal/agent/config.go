package agent

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

// Config holds agent settings from flags and environment variables.
type Config struct {
	HubURL     string
	Token      string
	Name       string
	Interval   time.Duration
	NetInclude []string
	NetExclude []string
	DockerHost string
	Root       string
	Once       bool
	// DebugTiming logs how long each collector takes, once per sample.
	DebugTiming bool
}

// LoadConfig reads env vars through getenv, then lets flags in args override them.
func LoadConfig(args []string, getenv func(string) string) (Config, error) {
	or := func(k, def string) string {
		if v := getenv(k); v != "" {
			return v
		}
		return def
	}
	host, _ := os.Hostname()
	c := Config{Root: "/"}
	interval := or("INTERVAL", "5s")

	fs := flag.NewFlagSet("kite-agent", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.HubURL, "hub", getenv("HUB_URL"), "hub URL, e.g. http://hub:8420")
	fs.StringVar(&c.Token, "token", getenv("AGENT_TOKEN"), "agent token")
	fs.StringVar(&c.Name, "name", or("AGENT_NAME", host), "machine name")
	fs.StringVar(&interval, "interval", interval, "sample interval")
	fs.BoolVar(&c.Once, "once", false, "print one sample as JSON and exit")
	fs.BoolVar(&c.DebugTiming, "debug-timing", false, "log how long each collector takes")
	if err := fs.Parse(args); err != nil {
		return c, err
	}

	d, err := time.ParseDuration(interval)
	if err != nil {
		return c, fmt.Errorf("bad interval %q: %w", interval, err)
	}
	if d < time.Second {
		return c, errors.New("interval must be at least 1s")
	}
	c.Interval = d
	c.NetInclude = splitList(getenv("NET_INCLUDE"))
	c.NetExclude = splitList(or("NET_EXCLUDE", defaultNetExclude(runtime.GOOS)))
	c.DockerHost = or("DOCKER_HOST", defaultDockerHost(runtime.GOOS))
	c.HubURL = strings.TrimRight(c.HubURL, "/")

	if !protocol.ValidMachineName(c.Name) {
		return c, fmt.Errorf("invalid machine name %q (letters, digits, '.', '_', '-'; max 64)", c.Name)
	}
	if !c.Once && (c.HubURL == "" || c.Token == "") {
		return c, errors.New("hub URL and token are required (-hub/HUB_URL, -token/AGENT_TOKEN)")
	}
	return c, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

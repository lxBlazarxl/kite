package hub

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"time"
)

// Config holds hub settings from flags and environment variables.
type Config struct {
	Listen          string
	AgentToken      string
	WebPassword     string
	DBPath          string
	TLSCert         string
	TLSKey          string
	RawRetention    time.Duration
	RollupRetention time.Duration
}

// LoadConfig reads env vars through getenv, then lets flags in args override them.
func LoadConfig(args []string, getenv func(string) string) (Config, error) {
	or := func(k, def string) string {
		if v := getenv(k); v != "" {
			return v
		}
		return def
	}
	var c Config
	raw, rollup := or("RAW_RETENTION", "24h"), or("ROLLUP_RETENTION", "168h")
	fs := flag.NewFlagSet("kite-hub", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.Listen, "listen", or("HUB_LISTEN", ":8420"), "listen address")
	fs.StringVar(&c.AgentToken, "token", getenv("AGENT_TOKEN"), "token agents must send")
	fs.StringVar(&c.WebPassword, "password", getenv("WEB_PASSWORD"), "optional web UI password")
	fs.StringVar(&c.DBPath, "db", or("DB_PATH", "kite.db"), "SQLite database path")
	fs.StringVar(&raw, "raw-retention", raw, "how long to keep full-detail samples")
	fs.StringVar(&rollup, "rollup-retention", rollup, "how long to keep 1-minute averages")
	fs.StringVar(&c.TLSCert, "tls-cert", getenv("TLS_CERT"), "TLS certificate file")
	fs.StringVar(&c.TLSKey, "tls-key", getenv("TLS_KEY"), "TLS key file")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	var err error
	if c.RawRetention, err = time.ParseDuration(raw); err != nil {
		return c, fmt.Errorf("bad raw retention: %w", err)
	}
	if c.RollupRetention, err = time.ParseDuration(rollup); err != nil {
		return c, fmt.Errorf("bad rollup retention: %w", err)
	}
	if c.AgentToken == "" {
		return c, errors.New("agent token is required (-token or AGENT_TOKEN)")
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return c, errors.New("set both -tls-cert and -tls-key, or neither")
	}
	return c, nil
}

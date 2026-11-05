// Command kite-agent collects machine metrics and pushes them to a kite hub.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/retransmit/kite/internal/agent"
)

func main() {
	cfg, err := agent.LoadConfig(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kite-agent:", err)
		os.Exit(2)
	}
	cols := agent.Detect(cfg)
	ctx := context.Background()

	if cfg.Once {
		// Collect twice so rate-based values (disk, network, power) are filled.
		agent.CollectOnce(ctx, cfg, cols, time.Now())
		time.Sleep(time.Second)
		s := agent.CollectOnce(ctx, cfg, cols, time.Now())
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(s); err != nil {
			fmt.Fprintln(os.Stderr, "kite-agent:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Fprintln(os.Stderr, "kite-agent: run loop not implemented")
	os.Exit(1)
}

// Command kite-agent collects machine metrics and pushes them to a kite hub.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/retransmit/kite/internal/agent"
	"github.com/retransmit/kite/internal/version"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "-version" || os.Args[1] == "--version") {
		fmt.Println("kite-agent", version.Version)
		return
	}
	if len(os.Args) > 1 {
		var err error
		handled := true
		switch os.Args[1] {
		case "install":
			err = installService(os.Args[2:])
		case "uninstall":
			err = uninstallService()
		case "service":
			err = runService()
		default:
			handled = false
		}
		if handled {
			if err != nil {
				fmt.Fprintln(os.Stderr, "kite-agent:", err)
				os.Exit(2)
			}
			return
		}
	}

	cfg, err := agent.LoadConfig(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kite-agent:", err)
		os.Exit(2)
	}
	if err := agent.LowerPriority(); err != nil {
		log.Printf("kite-agent: could not lower priority: %v", err)
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

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("kite-agent %q sending to %s every %s (%d collectors)", cfg.Name, cfg.HubURL, cfg.Interval, len(cols))
	sender := agent.NewSender(cfg.HubURL, cfg.Token, 5*time.Second)
	agent.Run(ctx, cfg, cols, sender.Send, log.Printf)
}

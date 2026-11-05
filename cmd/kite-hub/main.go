// Command kite-hub receives samples from kite agents, keeps their history
// and serves it over HTTP.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/retransmit/kite/internal/hub"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "remove-machine" {
		os.Exit(removeMachine(os.Args[2:]))
	}
	cfg, err := hub.LoadConfig(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kite-hub:", err)
		os.Exit(2)
	}
	st, err := hub.OpenStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("kite-hub: open %s: %v", cfg.DBPath, err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := hub.NewServer(st, cfg, time.Now)
	go srv.RunMaintenance(ctx)

	httpSrv := &http.Server{Addr: cfg.Listen, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdown)
	}()
	log.Printf("kite-hub listening on %s (db %s)", cfg.Listen, cfg.DBPath)
	if cfg.TLSCert != "" {
		err = httpSrv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
	} else {
		err = httpSrv.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("kite-hub: %v", err)
	}
}

func removeMachine(args []string) int {
	fs := flag.NewFlagSet("remove-machine", flag.ExitOnError)
	db := fs.String("db", envOr("DB_PATH", "kite.db"), "SQLite database path")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: kite-hub remove-machine [-db path] <name>")
		return 2
	}
	name := fs.Arg(0)
	st, err := hub.OpenStore(*db)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kite-hub:", err)
		return 1
	}
	defer st.Close()
	ok, err := st.DeleteMachine(context.Background(), name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kite-hub:", err)
		return 1
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "no machine named %s\n", name)
		return 1
	}
	fmt.Printf("removed %s\n", name)
	return 0
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

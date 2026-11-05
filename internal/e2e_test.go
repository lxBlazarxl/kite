package internal_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/retransmit/kite/internal/agent"
	"github.com/retransmit/kite/internal/hub"
)

func TestEndToEnd(t *testing.T) {
	st, err := hub.OpenStore(filepath.Join(t.TempDir(), "kite.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := httptest.NewServer(hub.NewServer(st, hub.Config{AgentToken: "tok", RawRetention: 24 * time.Hour}, time.Now).Handler())
	defer srv.Close()

	cfg, err := agent.LoadConfig([]string{"-hub", srv.URL, "-token", "tok", "-name", "e2e-box"}, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	cols := agent.Detect(cfg)
	send := agent.NewSender(cfg.HubURL, cfg.Token, 5*time.Second)
	for i := 0; i < 2; i++ {
		if err := send.Send(context.Background(), agent.CollectOnce(context.Background(), cfg, cols, time.Now())); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1100 * time.Millisecond)
	}

	var ms []struct {
		Name   string `json:"name"`
		Online bool   `json:"online"`
	}
	getJSON(t, srv.URL+"/api/v1/machines", &ms)
	if len(ms) != 1 || ms[0].Name != "e2e-box" || !ms[0].Online {
		t.Fatalf("machines %+v", ms)
	}
	var keys []string
	getJSON(t, srv.URL+"/api/v1/machines/e2e-box/keys", &keys)
	if !slices.Contains(keys, "cpu.total") || !slices.Contains(keys, "mem.used") {
		t.Fatalf("keys %v", keys)
	}
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	r, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

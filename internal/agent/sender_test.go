package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

func TestSenderStatus(t *testing.T) {
	var gotAuth string
	var gotName string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/ingest" || r.Method != "POST" {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		var s protocol.Sample
		json.NewDecoder(r.Body).Decode(&s)
		gotName = s.Machine
		if gotAuth != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	if err := NewSender(srv.URL, "tok", time.Second).Send(context.Background(), &protocol.Sample{Machine: "pc"}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok" || gotName != "pc" {
		t.Fatalf("auth %q name %q", gotAuth, gotName)
	}
	err := NewSender(srv.URL, "bad", time.Second).Send(context.Background(), &protocol.Sample{Machine: "pc"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
}

func TestSenderOtherStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	err := NewSender(srv.URL, "tok", time.Second).Send(context.Background(), &protocol.Sample{})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("got %v", err)
	}
}

func TestSenderTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	defer close(release)
	start := time.Now()
	err := NewSender(srv.URL, "tok", 200*time.Millisecond).Send(context.Background(), &protocol.Sample{})
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("err %v after %v", err, time.Since(start))
	}
}

func TestRunNoOverlap(t *testing.T) {
	var cur, max, calls int32
	send := func(ctx context.Context, _ *protocol.Sample) error {
		n := atomic.AddInt32(&cur, 1)
		for {
			m := atomic.LoadInt32(&max)
			if n <= m || atomic.CompareAndSwapInt32(&max, m, n) {
				break
			}
		}
		atomic.AddInt32(&calls, 1)
		time.Sleep(200 * time.Millisecond)
		atomic.AddInt32(&cur, -1)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	cfg := Config{Name: "pc", Interval: 50 * time.Millisecond}
	err := Run(ctx, cfg, nil, send, func(string, ...any) {})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run returned %v", err)
	}
	if max != 1 || calls < 2 || calls > 4 {
		t.Fatalf("max concurrent %d calls %d", max, calls)
	}
}

func TestRunLogsStreaks(t *testing.T) {
	var mu sync.Mutex
	var logs []string
	logf := func(f string, a ...any) { mu.Lock(); logs = append(logs, fmt.Sprintf(f, a...)); mu.Unlock() }
	var n int32
	send := func(context.Context, *protocol.Sample) error {
		if atomic.AddInt32(&n, 1) <= 3 {
			return errors.New("connection refused")
		}
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for atomic.LoadInt32(&n) < 6 {
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
	}()
	Run(ctx, Config{Name: "pc", Interval: 10 * time.Millisecond}, nil, send, logf)
	mu.Lock()
	defer mu.Unlock()
	if len(logs) != 2 || !strings.Contains(logs[0], "connection refused") || !strings.Contains(logs[1], "hub reachable again") {
		t.Fatalf("logs %q", logs)
	}
}

func TestRunHungCollectorStillSends(t *testing.T) {
	sent := make(chan error, 4)
	send := func(ctx context.Context, _ *protocol.Sample) error {
		sent <- ctx.Err()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Run(ctx, Config{Name: "pc", Interval: 200 * time.Millisecond}, []Collector{ctxCollector{"docker"}}, send, func(string, ...any) {})
	select {
	case err := <-sent:
		if err != nil {
			t.Fatalf("send got an already-expired context: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no sample sent while a collector hangs")
	}
}

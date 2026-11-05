package hub

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBrokerNeverBlocks(t *testing.T) {
	b := NewBroker()
	ch, cancel := b.Subscribe()
	defer cancel()
	start := time.Now()
	for i := 0; i < 100; i++ {
		b.Publish([]byte("x"))
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("publish blocked on a slow subscriber")
	}
	if len(ch) != 16 {
		t.Fatalf("buffer holds %d, want 16", len(ch))
	}
}

func TestBrokerUnsubscribe(t *testing.T) {
	b := NewBroker()
	_, cancel := b.Subscribe()
	cancel()
	b.Publish([]byte("x")) // must not panic or block
	if n := b.count(); n != 0 {
		t.Fatalf("%d subscribers left", n)
	}
}

func TestStreamReceivesSample(t *testing.T) {
	h := newTestHub(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", h.ts.URL+"/api/v1/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type %q", ct)
	}
	if v := resp.Header.Get("X-Accel-Buffering"); v != "no" {
		t.Fatalf("X-Accel-Buffering %q", v)
	}
	for h.srv.broker.count() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	h.ingest(sample("pc", "linux", 1))
	sc := bufio.NewScanner(resp.Body)
	sawEvent := false
	for sc.Scan() {
		line := sc.Text()
		if line == "event: sample" {
			sawEvent = true
			continue
		}
		if sawEvent && strings.HasPrefix(line, "data:") {
			if !strings.Contains(line, `"machine":"pc"`) || !strings.Contains(line, `"online":true`) ||
				!strings.Contains(line, fmt.Sprintf(`"ts":%d`, t0.Unix())) {
				t.Fatalf("data %s", line)
			}
			return
		}
	}
	t.Fatalf("no sample event: %v", sc.Err())
}

func TestPublishStatus(t *testing.T) {
	h := newTestHub(t, nil)
	h.ingest(sample("pc", "linux", 1))
	ch, cancel := h.srv.broker.Subscribe()
	defer cancel()
	h.now = t0.Add(16 * time.Second)
	h.srv.PublishStatus(context.Background())
	select {
	case msg := <-ch:
		s := string(msg)
		if !strings.HasPrefix(s, "event: status\n") || !strings.Contains(s, `"name":"pc"`) || !strings.Contains(s, `"online":false`) {
			t.Fatalf("got %q", s)
		}
	case <-time.After(time.Second):
		t.Fatal("no status event")
	}
}

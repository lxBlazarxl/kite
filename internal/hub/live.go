package hub

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

const subscriberBuffer = 16

// Broker fans messages out to live subscribers. Publishing never blocks: a
// subscriber whose buffer is full misses messages.
type Broker struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func NewBroker() *Broker { return &Broker{subs: map[chan []byte]struct{}{}} }

// Subscribe returns a message channel and a function that unsubscribes.
func (b *Broker) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, subscriberBuffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()
		})
	}
}

func (b *Broker) Publish(msg []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (b *Broker) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

func sseFrame(event string, v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return []byte("event: " + event + "\ndata: " + string(data) + "\n\n")
}

// afterIngest publishes a live sample. ts is the hub's receive time, the
// same clock history uses, so live and historical points line up even when
// the agent's clock is wrong.
func (s *Server) afterIngest(smp *protocol.Sample, receivedAt time.Time) {
	s.broker.Publish(sseFrame("sample", map[string]any{"machine": smp.Machine, "online": true, "ts": receivedAt.Unix(), "sample": smp}))
}

type statusOut struct {
	Name   string `json:"name"`
	Online bool   `json:"online"`
}

// PublishStatus sends every machine's online state to live subscribers.
func (s *Server) PublishStatus(ctx context.Context) {
	ms, err := s.st.Machines(ctx)
	if err != nil {
		log.Printf("status: %v", err)
		return
	}
	now := s.now()
	out := make([]statusOut, 0, len(ms))
	for _, m := range ms {
		out = append(out, statusOut{m.Name, isOnline(m.LastSeen, now, m.IntervalSec)})
	}
	s.broker.Publish(sseFrame("status", out))
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		httpError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // stop nginx from buffering the stream
	w.WriteHeader(http.StatusOK)
	fl.Flush()

	ch, cancel := s.broker.Subscribe()
	defer cancel()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			if _, err := w.Write(msg); err != nil {
				return
			}
			fl.Flush()
		case <-ping.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

// ErrUnauthorized means the hub rejected the agent token.
var ErrUnauthorized = errors.New("hub rejected agent token")

// Sender posts samples to a hub.
type Sender struct {
	client *http.Client
	url    string
	token  string
}

func NewSender(hubURL, token string, timeout time.Duration) *Sender {
	return &Sender{client: &http.Client{Timeout: timeout}, url: hubURL + "/api/v1/ingest", token: token}
}

// Send posts one sample. It returns ErrUnauthorized for a 401.
func (s *Sender) Send(ctx context.Context, smp *protocol.Sample) error {
	body, err := json.Marshal(smp)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrUnauthorized
	default:
		return fmt.Errorf("hub returned %s", resp.Status)
	}
}

// Run collects and sends a sample every interval until ctx ends. Each tick
// runs to completion before the next; ticks missed while a send is slow are
// dropped, so nothing queues up. Only the first failure of a streak and the
// recovery are logged.
func Run(ctx context.Context, cfg Config, cols []Collector, send func(context.Context, *protocol.Sample) error, logf func(format string, args ...any)) error {
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	failing := false
	tick := func() {
		// CollectOnce bounds each collector; the send gets its own budget
		// (the Sender's timeout) so a slow collector cannot starve it.
		smp, timings := CollectTimed(ctx, cfg, cols, time.Now())
		if cfg.DebugTiming {
			logf("%s", formatTimings(timings))
		}
		err := send(ctx, smp)
		switch {
		case err != nil && !failing:
			failing = true
			logf("sending to hub failed: %v (will keep retrying quietly)", err)
		case err == nil && failing:
			failing = false
			logf("hub reachable again")
		}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			tick()
		}
	}
}

// formatTimings renders one sample's collector durations as
// "timing total=1.2ms host=10µs cpu=300µs ...".
func formatTimings(ts []CollectorTiming) string {
	var total time.Duration
	var b strings.Builder
	for _, t := range ts {
		total += t.Took
		fmt.Fprintf(&b, " %s=%s", t.Name, t.Took.Round(time.Microsecond))
	}
	return fmt.Sprintf("timing total=%s%s", total.Round(time.Microsecond), b.String())
}

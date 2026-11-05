package hub

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

const (
	maxIngestBytes = 1 << 20
	maxSeriesKeys  = 64
)

var bodyReadTimeout = 10 * time.Second

// NewHTTPServer wraps a handler in an http.Server.
func NewHTTPServer(addr string, h http.Handler) *http.Server {
	// No server-wide ReadTimeout/WriteTimeout: they would cut SSE streams.
	// Request bodies get a per-handler deadline instead (bodyReadTimeout).
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
}

// Server serves the hub HTTP API.
type Server struct {
	st     *Store
	cfg    Config
	now    func() time.Time
	broker *Broker
	key    []byte
}

// NewServer builds a server. now is the clock used for receive times and
// online checks.
func NewServer(st *Store, cfg Config, now func() time.Time) *Server {
	srv := &Server{st: st, cfg: cfg, now: now, broker: NewBroker()}
	if cfg.WebPassword != "" {
		srv.key = sessionKey(st, cfg.WebPassword)
	}
	return srv
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/ingest", s.handleIngest)
	mux.HandleFunc("POST /api/v1/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/logout", s.handleLogout)
	mux.HandleFunc("GET /api/v1/machines", s.requireSession(s.handleMachines))
	mux.HandleFunc("GET /api/v1/machines/{name}/keys", s.requireSession(s.handleKeys))
	mux.HandleFunc("GET /api/v1/machines/{name}/series", s.requireSession(s.handleSeries))
	mux.HandleFunc("DELETE /api/v1/machines/{name}", s.requireSession(s.handleDelete))
	mux.HandleFunc("GET /api/v1/stream", s.requireSession(s.handleStream))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("kite hub is running; web UI arrives in a later release\n"))
	})
	return mux
}

// RunMaintenance rolls up every minute, prunes every 10 minutes and
// publishes machine status every 5 seconds until ctx ends.
func (s *Server) RunMaintenance(ctx context.Context) {
	rollup := time.NewTicker(time.Minute)
	prune := time.NewTicker(10 * time.Minute)
	defer rollup.Stop()
	defer prune.Stop()
	status := time.NewTicker(5 * time.Second)
	defer status.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-rollup.C:
			if err := s.st.Rollup(ctx, s.now()); err != nil {
				log.Printf("rollup: %v", err)
			}
		case <-status.C:
			s.PublishStatus(ctx)
		case <-prune.C:
			if err := s.st.Prune(ctx, s.now(), s.cfg.RawRetention, s.cfg.RollupRetention); err != nil {
				log.Printf("prune: %v", err)
			}
		}
	}
}

// isOnline reports whether a machine has reported within 3 intervals.
func isOnline(lastSeen, now time.Time, intervalSec float64) bool {
	return now.Sub(lastSeen) <= time.Duration(3*intervalSec*float64(time.Second))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	http.Error(w, msg, code)
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || subtle.ConstantTimeCompare([]byte(tok), []byte(s.cfg.AgentToken)) != 1 {
		httpError(w, http.StatusUnauthorized, "bad agent token")
		return
	}
	http.NewResponseController(w).SetReadDeadline(time.Now().Add(bodyReadTimeout))
	var smp protocol.Sample
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIngestBytes)).Decode(&smp); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			httpError(w, http.StatusRequestEntityTooLarge, "sample too large")
			return
		}
		httpError(w, http.StatusBadRequest, "bad sample: "+err.Error())
		return
	}
	if !protocol.ValidMachineName(smp.Machine) {
		httpError(w, http.StatusBadRequest, "invalid machine name")
		return
	}
	if smp.IntervalSec <= 0 {
		smp.IntervalSec = 5 // agents always send it; guard against a hub that never sees a machine as online
	}
	receivedAt := s.now()
	if err := s.st.Ingest(r.Context(), &smp, receivedAt); err != nil {
		log.Printf("ingest %s: %v", smp.Machine, err)
		httpError(w, http.StatusInternalServerError, "store failed")
		return
	}
	s.afterIngest(&smp, receivedAt)
	w.WriteHeader(http.StatusNoContent)
}

type machineOut struct {
	Name        string          `json:"name"`
	OS          string          `json:"os"`
	Online      bool            `json:"online"`
	LastSeen    int64           `json:"last_seen"`
	IntervalSec float64         `json:"interval_sec"`
	Latest      json.RawMessage `json:"latest"`
}

func (s *Server) handleMachines(w http.ResponseWriter, r *http.Request) {
	ms, err := s.st.Machines(r.Context())
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := s.now()
	out := make([]machineOut, 0, len(ms))
	for _, m := range ms {
		out = append(out, machineOut{m.Name, m.OS, isOnline(m.LastSeen, now, m.IntervalSec), m.LastSeen.Unix(), m.IntervalSec, m.Latest})
	}
	writeJSON(w, out)
}

func (s *Server) storeError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrUnknownMachine) {
		httpError(w, http.StatusNotFound, "unknown machine")
		return
	}
	httpError(w, http.StatusInternalServerError, err.Error())
}

func (s *Server) handleKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.st.Keys(r.Context(), r.PathValue("name"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, keys)
}

func (s *Server) handleSeries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	keys := q["key"]
	if len(keys) == 0 {
		httpError(w, http.StatusBadRequest, "at least one key parameter is required")
		return
	}
	if len(keys) > maxSeriesKeys {
		httpError(w, http.StatusBadRequest, fmt.Sprintf("at most %d keys per request", maxSeriesKeys))
		return
	}
	from, err1 := strconv.ParseInt(q.Get("from"), 10, 64)
	to, err2 := strconv.ParseInt(q.Get("to"), 10, 64)
	if err1 != nil || err2 != nil || from > to {
		httpError(w, http.StatusBadRequest, "from and to must be unix seconds with from <= to")
		return
	}
	fromT, toT := time.Unix(from, 0), time.Unix(to, 0)
	res := Resolution(q.Get("res"))
	switch res {
	case "":
		res = ChooseResolution(fromT, toT, s.now(), s.cfg.RawRetention)
	case ResRaw, Res1m, Res10m:
	default:
		httpError(w, http.StatusBadRequest, "res must be raw, 1m or 10m")
		return
	}
	name := r.PathValue("name")
	interval, err := s.st.machineInterval(r.Context(), name)
	if err != nil {
		s.storeError(w, err)
		return
	}

	// Stream one key at a time so a week of many series is never held in
	// memory at once.
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"resolution":%q,"step":%s,"series":{`, res, strconv.FormatFloat(stepFor(res, interval), 'f', -1, 64))
	first := true
	buf := make([]byte, 0, 64)
	err = s.st.QueryEach(r.Context(), name, keys, fromT, toT, res, func(k string, pts []SeriesPoint) error {
		kb, _ := json.Marshal(k)
		if !first {
			w.Write([]byte{','})
		}
		first = false
		w.Write(kb)
		w.Write([]byte(":["))
		for i, p := range pts {
			buf = buf[:0]
			if i > 0 {
				buf = append(buf, ',')
			}
			buf = append(buf, '[')
			buf = strconv.AppendInt(buf, p.TS, 10)
			buf = append(buf, ',')
			buf = strconv.AppendFloat(buf, p.Avg, 'f', -1, 64)
			if res != ResRaw {
				buf = append(buf, ',')
				buf = strconv.AppendFloat(buf, p.Max, 'f', -1, 64)
			}
			buf = append(buf, ']')
			if _, err := w.Write(buf); err != nil {
				return err
			}
		}
		_, err := w.Write([]byte{']'})
		return err
	})
	if err != nil {
		// Headers are already sent; the truncated body tells the client.
		log.Printf("series %s: %v", name, err)
		return
	}
	w.Write([]byte("}}"))
}

// stepFor is the expected spacing of points at a resolution.
func stepFor(res Resolution, machineInterval float64) float64 {
	switch res {
	case Res1m:
		return 60
	case Res10m:
		return 600
	default:
		return machineInterval
	}
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	ok, err := s.st.DeleteMachine(r.Context(), r.PathValue("name"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		httpError(w, http.StatusNotFound, "unknown machine")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

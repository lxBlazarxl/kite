package hub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

type testHub struct {
	t   *testing.T
	st  *Store
	srv *Server
	ts  *httptest.Server
	now time.Time
}

func newTestHub(t *testing.T, mod func(*Config)) *testHub {
	t.Helper()
	st, _ := openTest(t)
	cfg := Config{AgentToken: "tok", RawRetention: 24 * time.Hour, RollupRetention: 168 * time.Hour}
	if mod != nil {
		mod(&cfg)
	}
	h := &testHub{t: t, st: st, now: t0}
	h.srv = NewServer(st, cfg, func() time.Time { return h.now })
	h.ts = httptest.NewServer(h.srv.Handler())
	t.Cleanup(h.ts.Close)
	return h
}

func (h *testHub) do(method, path, token string, body []byte, cookies ...*http.Cookie) *http.Response {
	h.t.Helper()
	req, _ := http.NewRequest(method, h.ts.URL+path, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func (h *testHub) ingest(s *protocol.Sample) *http.Response {
	b, _ := json.Marshal(s)
	return h.do("POST", "/api/v1/ingest", "tok", b)
}

func decode(t *testing.T, r *http.Response, v any) {
	t.Helper()
	if r.StatusCode != 200 {
		b, _ := io.ReadAll(r.Body)
		t.Fatalf("status %d: %s", r.StatusCode, b)
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func TestIngestAuth(t *testing.T) {
	h := newTestHub(t, nil)
	b, _ := json.Marshal(sample("pc", "linux", 1))
	if r := h.do("POST", "/api/v1/ingest", "", b); r.StatusCode != 401 {
		t.Fatalf("no token: %d", r.StatusCode)
	}
	if r := h.do("POST", "/api/v1/ingest", "wrong", b); r.StatusCode != 401 {
		t.Fatalf("wrong token: %d", r.StatusCode)
	}
	if r := h.do("POST", "/api/v1/ingest", "tok", b); r.StatusCode != 204 {
		t.Fatalf("good token: %d", r.StatusCode)
	}
}

func TestIngestValidation(t *testing.T) {
	h := newTestHub(t, nil)
	if r := h.do("POST", "/api/v1/ingest", "tok", []byte("{")); r.StatusCode != 400 {
		t.Fatalf("bad json: %d", r.StatusCode)
	}
	if r := h.ingest(sample("a b", "linux", 1)); r.StatusCode != 400 {
		t.Fatalf("bad name: %d", r.StatusCode)
	}
	big := []byte(`{"machine":"pc","hostname":"` + strings.Repeat("x", 2<<20) + `"}`)
	if r := h.do("POST", "/api/v1/ingest", "tok", big); r.StatusCode != 413 {
		t.Fatalf("huge body: %d", r.StatusCode)
	}
}

type machineJSON struct {
	Name        string          `json:"name"`
	OS          string          `json:"os"`
	Online      bool            `json:"online"`
	LastSeen    int64           `json:"last_seen"`
	IntervalSec float64         `json:"interval_sec"`
	Latest      json.RawMessage `json:"latest"`
}

func TestMachinesOnline(t *testing.T) {
	h := newTestHub(t, nil)
	h.ingest(sample("pc", "linux", 1))
	h.now = t0.Add(10 * time.Second)
	var ms []machineJSON
	decode(t, h.do("GET", "/api/v1/machines", "", nil), &ms)
	if len(ms) != 1 || !ms[0].Online || ms[0].LastSeen != t0.Unix() || ms[0].OS != "linux" ||
		ms[0].IntervalSec != 5 || !strings.Contains(string(ms[0].Latest), `"total_pct":1`) {
		t.Fatalf("got %+v", ms)
	}
	h.now = t0.Add(16 * time.Second)
	decode(t, h.do("GET", "/api/v1/machines", "", nil), &ms)
	if ms[0].Online {
		t.Fatal("must be offline after 3 missed intervals")
	}
}

func TestMachinesEmptyIsArray(t *testing.T) {
	h := newTestHub(t, nil)
	r := h.do("GET", "/api/v1/machines", "", nil)
	b, _ := io.ReadAll(r.Body)
	if strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("got %q", b)
	}
}

type seriesJSON struct {
	Resolution string                 `json:"resolution"`
	Step       float64                `json:"step"`
	Series     map[string][][]float64 `json:"series"`
}

func TestSeriesRaw(t *testing.T) {
	h := newTestHub(t, nil)
	h.ingest(sample("pc", "linux", 10))
	h.now = t0.Add(5 * time.Second)
	h.ingest(sample("pc", "linux", 20))
	var got seriesJSON
	path := fmt.Sprintf("/api/v1/machines/pc/series?key=cpu.total&from=%d&to=%d", t0.Unix()-1, t0.Unix()+10)
	decode(t, h.do("GET", path, "", nil), &got)
	want := seriesJSON{"raw", 5, map[string][][]float64{"cpu.total": {{float64(t0.Unix()), 10}, {float64(t0.Unix() + 5), 20}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestSeries1m(t *testing.T) {
	h := newTestHub(t, nil)
	base := t0.Unix() / 60 * 60
	h.now = time.Unix(base, 0)
	h.ingest(sample("pc", "linux", 10))
	h.now = time.Unix(base+5, 0)
	h.ingest(sample("pc", "linux", 30))
	if err := h.st.Rollup(bg, time.Unix(base+61, 0)); err != nil {
		t.Fatal(err)
	}
	var got seriesJSON
	decode(t, h.do("GET", fmt.Sprintf("/api/v1/machines/pc/series?key=cpu.total&from=%d&to=%d&res=1m", base, base+60), "", nil), &got)
	if got.Resolution != "1m" || got.Step != 60 || !reflect.DeepEqual(got.Series["cpu.total"], [][]float64{{float64(base), 20, 30}}) {
		t.Fatalf("got %+v", got)
	}
}

func TestSeriesErrors(t *testing.T) {
	h := newTestHub(t, nil)
	h.ingest(sample("pc", "linux", 1))
	cases := map[string]int{
		"/api/v1/machines/pc/series?from=1&to=2":                      400,
		"/api/v1/machines/pc/series?key=cpu.total&from=5&to=2":        400,
		"/api/v1/machines/pc/series?key=cpu.total&from=x&to=2":        400,
		"/api/v1/machines/pc/series?key=cpu.total&from=1&to=2&res=5m": 400,
		"/api/v1/machines/nope/series?key=cpu.total&from=1&to=2":      404,
		"/api/v1/machines/nope/keys":                                  404,
	}
	for path, want := range cases {
		if r := h.do("GET", path, "", nil); r.StatusCode != want {
			t.Errorf("%s: got %d want %d", path, r.StatusCode, want)
		}
	}
}

func TestKeysAPI(t *testing.T) {
	h := newTestHub(t, nil)
	h.ingest(sample("pc", "linux", 1))
	var keys []string
	decode(t, h.do("GET", "/api/v1/machines/pc/keys", "", nil), &keys)
	if len(keys) == 0 || keys[0] != "cpu.total" {
		t.Fatalf("got %v", keys)
	}
}

func TestDeleteMachineAPI(t *testing.T) {
	h := newTestHub(t, nil)
	h.ingest(sample("pc", "linux", 1))
	if r := h.do("DELETE", "/api/v1/machines/pc", "", nil); r.StatusCode != 204 {
		t.Fatalf("delete: %d", r.StatusCode)
	}
	if r := h.do("GET", "/api/v1/machines/pc/keys", "", nil); r.StatusCode != 404 {
		t.Fatalf("keys after delete: %d", r.StatusCode)
	}
	if r := h.do("DELETE", "/api/v1/machines/pc", "", nil); r.StatusCode != 404 {
		t.Fatalf("second delete: %d", r.StatusCode)
	}
}

func TestRootPlaceholder(t *testing.T) {
	h := newTestHub(t, nil)
	r := h.do("GET", "/", "", nil)
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || !strings.Contains(string(b), "kite hub is running") {
		t.Fatalf("%d %s", r.StatusCode, b)
	}
}

func TestIngestZeroIntervalDefaults(t *testing.T) {
	h := newTestHub(t, nil)
	s := sample("pc", "linux", 1)
	s.IntervalSec = 0
	h.ingest(s)
	var ms []machineJSON
	decode(t, h.do("GET", "/api/v1/machines", "", nil), &ms)
	if ms[0].IntervalSec != 5 || !ms[0].Online {
		t.Fatalf("got %+v", ms[0])
	}
}

func TestSlowBodyIsCutOff(t *testing.T) {
	old := bodyReadTimeout
	bodyReadTimeout = 150 * time.Millisecond
	defer func() { bodyReadTimeout = old }()
	h := newTestHub(t, nil)
	conn, err := net.Dial("tcp", h.ts.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "POST /api/v1/ingest HTTP/1.1\r\nHost: x\r\nAuthorization: Bearer tok\r\nContent-Length: 1000\r\n\r\n{\"machine\":")
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil || !strings.HasPrefix(string(buf[:n]), "HTTP/1.1 400") {
		t.Fatalf("slow body not cut off: %q %v", buf[:n], err)
	}
}

func TestHTTPServerTimeouts(t *testing.T) {
	s := NewHTTPServer(":0", http.NotFoundHandler())
	if s.IdleTimeout == 0 || s.ReadHeaderTimeout == 0 {
		t.Fatalf("timeouts missing: %+v", s)
	}
	if s.ReadTimeout != 0 || s.WriteTimeout != 0 {
		t.Fatal("server-wide read/write timeouts would kill SSE streams")
	}
}

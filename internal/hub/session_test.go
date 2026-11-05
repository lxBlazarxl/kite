package hub

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func withPassword(c *Config) { c.WebPassword = "pw" }

func login(t *testing.T, h *testHub, pw string) *http.Response {
	t.Helper()
	return h.do("POST", "/api/v1/login", "", []byte(`{"password":"`+pw+`"}`))
}

func cookieFrom(t *testing.T, r *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range r.Cookies() {
		if c.Name == "kite_session" {
			return c
		}
	}
	t.Fatal("no kite_session cookie")
	return nil
}

func TestLoginFlow(t *testing.T) {
	h := newTestHub(t, withPassword)
	if r := h.do("GET", "/api/v1/machines", "", nil); r.StatusCode != 401 {
		t.Fatalf("no cookie: %d", r.StatusCode)
	}
	if r := login(t, h, "nope"); r.StatusCode != 401 {
		t.Fatalf("wrong password: %d", r.StatusCode)
	}
	r := login(t, h, "pw")
	if r.StatusCode != 204 {
		t.Fatalf("login: %d", r.StatusCode)
	}
	c := cookieFrom(t, r)
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Fatalf("cookie flags %+v", c)
	}
	if r := h.do("GET", "/api/v1/machines", "", nil, c); r.StatusCode != 200 {
		t.Fatalf("with cookie: %d", r.StatusCode)
	}
	if r := h.ingest(sample("pc", "linux", 1)); r.StatusCode != 204 {
		t.Fatalf("ingest must not need a cookie: %d", r.StatusCode)
	}
	for _, p := range []string{"/api/v1/machines/pc/keys", "/api/v1/stream"} {
		if r := h.do("GET", p, "", nil); r.StatusCode != 401 {
			t.Errorf("%s without cookie: %d", p, r.StatusCode)
		}
	}
	if r := h.do("DELETE", "/api/v1/machines/pc", "", nil); r.StatusCode != 401 {
		t.Errorf("delete without cookie: %d", r.StatusCode)
	}
}

func TestCookieTamperAndExpiry(t *testing.T) {
	h := newTestHub(t, withPassword)
	c := cookieFrom(t, login(t, h, "pw"))
	bad := *c
	last := bad.Value[len(bad.Value)-1]
	flip := byte('0')
	if last == '0' {
		flip = '1'
	}
	bad.Value = bad.Value[:len(bad.Value)-1] + string(flip)
	if r := h.do("GET", "/api/v1/machines", "", nil, &bad); r.StatusCode != 401 {
		t.Fatalf("tampered: %d", r.StatusCode)
	}
	forged := *c
	forged.Value = "9999999999" + c.Value[strings.Index(c.Value, "."):]
	if r := h.do("GET", "/api/v1/machines", "", nil, &forged); r.StatusCode != 401 {
		t.Fatalf("extended expiry: %d", r.StatusCode)
	}
	h.now = t0.Add(31 * 24 * time.Hour)
	if r := h.do("GET", "/api/v1/machines", "", nil, c); r.StatusCode != 401 {
		t.Fatalf("expired: %d", r.StatusCode)
	}
}

func TestCookieSurvivesRestart(t *testing.T) {
	h := newTestHub(t, withPassword)
	c := cookieFrom(t, login(t, h, "pw"))
	h2 := NewServer(h.st, Config{AgentToken: "tok", WebPassword: "pw"}, func() time.Time { return t0 })
	req, _ := http.NewRequest("GET", "/api/v1/machines", nil)
	req.AddCookie(c)
	if !h2.validSession(req) {
		t.Fatal("cookie rejected after restart")
	}
	h3 := NewServer(h.st, Config{AgentToken: "tok", WebPassword: "changed"}, func() time.Time { return t0 })
	if h3.validSession(req) {
		t.Fatal("changing the password must invalidate sessions")
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	h := newTestHub(t, withPassword)
	r := h.do("POST", "/api/v1/logout", "", nil)
	if r.StatusCode != 204 {
		t.Fatalf("logout: %d", r.StatusCode)
	}
	if c := cookieFrom(t, r); c.MaxAge >= 0 {
		t.Fatalf("cookie not cleared: %+v", c)
	}
}

func TestNoPasswordOpen(t *testing.T) {
	h := newTestHub(t, nil)
	if r := h.do("GET", "/api/v1/machines", "", nil); r.StatusCode != 200 {
		t.Fatalf("got %d", r.StatusCode)
	}
	if r := login(t, h, "anything"); r.StatusCode != 204 {
		t.Fatalf("login without password configured: %d", r.StatusCode)
	}
}

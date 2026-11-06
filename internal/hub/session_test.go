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

func sessionInfo(t *testing.T, h *testHub, cookies ...*http.Cookie) (bool, bool) {
	t.Helper()
	var v struct {
		PasswordRequired bool `json:"password_required"`
		LoggedIn         bool `json:"logged_in"`
	}
	decode(t, h.do("GET", "/api/v1/session", "", nil, cookies...), &v)
	return v.PasswordRequired, v.LoggedIn
}

func TestSessionInfo(t *testing.T) {
	open := newTestHub(t, nil)
	if req, in := sessionInfo(t, open); req || !in {
		t.Fatalf("no password: %v %v", req, in)
	}
	h := newTestHub(t, withPassword)
	if req, in := sessionInfo(t, h); !req || in {
		t.Fatalf("password, no cookie: %v %v", req, in)
	}
	c := cookieFrom(t, login(t, h, "pw"))
	if req, in := sessionInfo(t, h, c); !req || !in {
		t.Fatalf("logged in: %v %v", req, in)
	}
}

func TestSessionIncludesVersion(t *testing.T) {
	h := newTestHub(t, nil)
	var v struct {
		Version string `json:"version"`
	}
	decode(t, h.do("GET", "/api/v1/session", "", nil), &v)
	if v.Version != "dev" {
		t.Fatalf("got %q", v.Version)
	}
}

func TestInstallInfo(t *testing.T) {
	h := newTestHub(t, nil)
	var v struct {
		Version   string `json:"version"`
		Token     string `json:"token"`
		Downloads bool   `json:"downloads"`
	}
	decode(t, h.do("GET", "/api/v1/install-info", "", nil), &v)
	if v.Token != "tok" || v.Version != "dev" {
		t.Fatalf("got %+v", v)
	}
	p := newTestHub(t, withPassword)
	if r := p.do("GET", "/api/v1/install-info", "", nil); r.StatusCode != 401 {
		t.Fatalf("install-info without login: %d", r.StatusCode)
	}
	if r := p.do("GET", "/install.sh", "", nil); r.StatusCode != 200 {
		t.Fatalf("install.sh must not need a login: %d", r.StatusCode)
	}
}

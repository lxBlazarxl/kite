package hub

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookie = "kite_session"
	sessionTTL    = 30 * 24 * time.Hour
)

// sessionKey derives the cookie signing key from the password and the
// hub's stored secret, so cookies survive restarts and a password change
// invalidates them.
func sessionKey(st *Store, password string) []byte {
	secret, err := st.Secret(context.Background())
	if err != nil {
		log.Printf("session secret: %v (sessions will not survive restart)", err)
		secret = make([]byte, 32)
		rand.Read(secret)
	}
	sum := sha256.Sum256(append([]byte(password), secret...))
	return sum[:]
}

func (s *Server) sign(expiry string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(expiry))
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	expiry, sig, ok := strings.Cut(c.Value, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.sign(expiry))) {
		return false
	}
	exp, err := strconv.ParseInt(expiry, 10, 64)
	return err == nil && s.now().Unix() < exp
}

// requireSession guards read and delete routes when a web password is set.
func (s *Server) requireSession(h http.HandlerFunc) http.HandlerFunc {
	if s.cfg.WebPassword == "" {
		return h
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.validSession(r) {
			httpError(w, http.StatusUnauthorized, "login required")
			return
		}
		h(w, r)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WebPassword == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.NewResponseController(w).SetReadDeadline(time.Now().Add(bodyReadTimeout))
	var body struct {
		Password string `json:"password"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.cfg.WebPassword)) != 1 {
		time.Sleep(500 * time.Millisecond) // slow down guessing
		httpError(w, http.StatusUnauthorized, "wrong password")
		return
	}
	expiry := strconv.FormatInt(s.now().Add(sessionTTL).Unix(), 10)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    expiry + "." + s.sign(expiry),
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.TLSCert != "",
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLogout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

// handleSession tells the page whether a password is set and whether this
// browser is logged in. It never requires a session itself.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	required := s.cfg.WebPassword != ""
	writeJSON(w, map[string]bool{"password_required": required, "logged_in": !required || s.validSession(r)})
}

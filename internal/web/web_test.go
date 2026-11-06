package web

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func get(t *testing.T, h http.Handler, path string) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Result()
}

func TestServesIndex(t *testing.T) {
	r := get(t, Handler(), "/")
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/html") ||
		r.Header.Get("Cache-Control") != "no-cache" || !strings.Contains(string(b), `<div id="app">`) {
		t.Fatalf("%d %v %s", r.StatusCode, r.Header, b)
	}
}

func TestAssetCache(t *testing.T) {
	h := newHandler(fstest.MapFS{
		"index.html":    {Data: []byte("<html></html>")},
		"assets/a-1.js": {Data: []byte("console.log(1)")},
	})
	r := get(t, h, "/assets/a-1.js")
	if r.StatusCode != 200 || !strings.Contains(r.Header.Get("Content-Type"), "javascript") ||
		r.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("%d %v", r.StatusCode, r.Header)
	}
	if r := get(t, h, "/nope.js"); r.StatusCode != 404 {
		t.Fatalf("missing file: %d", r.StatusCode)
	}
}

// A fresh checkout (the release workflow) only has committed files, so this
// fails if a rebuilt page was committed without its new asset files.
func TestIndexAssetsEmbedded(t *testing.T) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		t.Fatal(err)
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	refs := regexp.MustCompile(`assets/[^"']+`).FindAllString(string(index), -1)
	if len(refs) == 0 {
		t.Fatal("index.html references no assets")
	}
	for _, r := range refs {
		if _, err := fs.Stat(sub, r); err != nil {
			t.Errorf("index.html references %s, which is not embedded", r)
		}
	}
}

package agentdl

import (
	"archive/tar"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func withFiles(t *testing.T, fsys fstest.MapFS) {
	t.Helper()
	old := files
	files = fsys
	t.Cleanup(func() { files = old })
}

func baseFiles() fstest.MapFS {
	return fstest.MapFS{
		"bin/README":       {Data: []byte("x")},
		"install.sh":       {Data: []byte("#!/bin/sh\n")},
		"install.ps1":      {Data: []byte("# ps\n")},
		"Dockerfile.agent": {Data: []byte("FROM scratch\nARG TARGETARCH\n")},
	}
}

func withAgents() fstest.MapFS {
	f := baseFiles()
	f["bin/kite-agent-linux-amd64"] = &fstest.MapFile{Data: []byte("amd64-bin")}
	f["bin/kite-agent-linux-arm64"] = &fstest.MapFile{Data: []byte("arm64-bin")}
	f["bin/kite-agent-windows-amd64.exe"] = &fstest.MapFile{Data: []byte("win-bin")}
	return f
}

func get(path string) *http.Response {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Result()
}

func TestDownloadsWithoutAgents(t *testing.T) {
	withFiles(t, baseFiles())
	if Available() {
		t.Fatal("no binaries must mean not available")
	}
	r := get("/download/kite-agent-linux-amd64")
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 404 || !strings.Contains(string(b), "built without agent downloads") {
		t.Fatalf("%d %q", r.StatusCode, b)
	}
	if r := get("/docker/agent.tar"); r.StatusCode != 404 {
		t.Fatalf("tar without agents: %d", r.StatusCode)
	}
}

func TestDownloadsWithAgents(t *testing.T) {
	withFiles(t, withAgents())
	if !Available() {
		t.Fatal("all binaries present must mean available")
	}
	r := get("/download/kite-agent-windows-amd64.exe")
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || string(b) != "win-bin" || r.Header.Get("Content-Type") != "application/octet-stream" ||
		!strings.Contains(r.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("%d %v %q", r.StatusCode, r.Header, b)
	}
	for _, p := range []string{"/download/README", "/download/..%2finstall.sh", "/download/kite-agent-linux-mips"} {
		if r := get(p); r.StatusCode != 404 {
			t.Errorf("%s: %d", p, r.StatusCode)
		}
	}
}

func TestDockerContext(t *testing.T) {
	withFiles(t, withAgents())
	var buf bytes.Buffer
	if err := DockerContext(&buf); err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(&buf)
	got := map[string]int64{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got[h.Name] = h.Mode
		if h.Name == "Dockerfile" {
			b, _ := io.ReadAll(tr)
			if !strings.Contains(string(b), "ARG TARGETARCH") {
				t.Errorf("Dockerfile %q", b)
			}
		}
	}
	if len(got) != 3 || got["kite-agent-linux-amd64"] != 0o755 || got["kite-agent-linux-arm64"] != 0o755 {
		t.Fatalf("tar entries %v", got)
	}
}

func TestScriptsServed(t *testing.T) {
	withFiles(t, baseFiles())
	if r := get("/install.sh"); r.StatusCode != 200 || r.Header.Get("Content-Type") != "text/x-shellscript; charset=utf-8" {
		t.Fatalf("install.sh %d %v", r.StatusCode, r.Header)
	}
	if r := get("/install.ps1"); r.StatusCode != 200 || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("install.ps1 %d %v", r.StatusCode, r.Header)
	}
}

// Package agentdl lets the hub hand out what a new machine needs: agent
// binaries, install scripts and a Docker build context for the agent. The
// release build puts the agent binaries in bin/ before building the hub.
package agentdl

import (
	"archive/tar"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
)

//go:embed bin install.sh install.ps1 Dockerfile.agent
var embedded embed.FS

// files is embedded in production and replaced in tests.
var files fs.FS = embedded

var agentNames = []string{"kite-agent-linux-amd64", "kite-agent-linux-arm64", "kite-agent-windows-amd64.exe"}

const notBuilt = "this hub was built without agent downloads; use a release build\n"

// Available reports whether every agent binary is embedded.
func Available() bool {
	for _, n := range agentNames {
		if _, err := fs.Stat(files, path.Join("bin", n)); err != nil {
			return false
		}
	}
	return true
}

// Handler serves /download/<agent>, /install.sh, /install.ps1 and
// /docker/agent.tar. None of them contain secrets, so none need a login.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /download/{name}", serveAgent)
	mux.HandleFunc("GET /install.sh", serveFile("install.sh", "text/x-shellscript; charset=utf-8"))
	mux.HandleFunc("GET /install.ps1", serveFile("install.ps1", "text/plain; charset=utf-8"))
	mux.HandleFunc("GET /docker/agent.tar", func(w http.ResponseWriter, r *http.Request) {
		if !Available() {
			http.Error(w, notBuilt, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/x-tar")
		DockerContext(w)
	})
	return mux
}

func serveAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	known := false
	for _, n := range agentNames {
		known = known || n == name
	}
	if !known {
		http.NotFound(w, r)
		return
	}
	b, err := fs.ReadFile(files, path.Join("bin", name))
	if err != nil {
		http.Error(w, notBuilt, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Write(b)
}

func serveFile(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(files, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(b)
	}
}

// DockerContext writes a Docker build context: the agent Dockerfile and
// both Linux agent binaries (BuildKit's TARGETARCH picks one).
func DockerContext(w io.Writer) error {
	tw := tar.NewWriter(w)
	add := func(name string, data []byte, mode int64) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}
	df, err := fs.ReadFile(files, "Dockerfile.agent")
	if err != nil {
		return err
	}
	if err := add("Dockerfile", df, 0o644); err != nil {
		return err
	}
	for _, n := range agentNames[:2] {
		b, err := fs.ReadFile(files, path.Join("bin", n))
		if err != nil {
			return err
		}
		if err := add(n, b, 0o755); err != nil {
			return err
		}
	}
	return tw.Close()
}

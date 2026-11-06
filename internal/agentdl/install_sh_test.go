//go:build linux

package agentdl

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type shEnv struct {
	prefix, log, systemctl, systemdDir string
	mu                                 sync.Mutex
	served                             []byte
	srv                                *httptest.Server
}

func (e *shEnv) serve(b []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.served = b
}

func newShEnv(t *testing.T) *shEnv {
	t.Helper()
	e := &shEnv{prefix: t.TempDir(), served: []byte("agent-v1")}
	stubs := t.TempDir()
	e.log = filepath.Join(stubs, "systemctl.log")
	e.systemctl = filepath.Join(stubs, "systemctl")
	e.systemdDir = t.TempDir() // stands in for /run/systemd/system on a systemd-booted machine
	os.WriteFile(e.systemctl, []byte("#!/bin/sh\necho \"$*\" >> "+e.log+"\n"), 0o755)
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download/kite-agent-linux-amd64" {
			http.NotFound(w, r)
			return
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		w.Write(e.served)
	}))
	t.Cleanup(e.srv.Close)
	return e
}

func (e *shEnv) run(t *testing.T, env map[string]string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{"install.sh"}, args...)...)
	cmd.Env = append(os.Environ(), "KITE_PREFIX="+e.prefix, "KITE_SKIP_ROOT_CHECK=1", "KITE_SYSTEMCTL="+e.systemctl, "KITE_ARCH=x86_64", "KITE_SYSTEMD_DIR="+e.systemdDir)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (e *shEnv) file(t *testing.T, p string) (string, os.FileMode) {
	t.Helper()
	full := filepath.Join(e.prefix, p)
	b, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	st, _ := os.Stat(full)
	return string(b), st.Mode().Perm()
}

func TestInstallShSyntax(t *testing.T) {
	if out, err := exec.Command("sh", "-n", "install.sh").CombinedOutput(); err != nil {
		t.Fatalf("sh -n: %v %s", err, out)
	}
	if _, err := exec.LookPath("shellcheck"); err != nil {
		t.Skip("shellcheck not installed")
	}
	if out, err := exec.Command("shellcheck", "-s", "sh", "install.sh").CombinedOutput(); err != nil {
		t.Fatalf("shellcheck: %s", out)
	}
}

func TestInstallShInstalls(t *testing.T) {
	e := newShEnv(t)
	out, err := e.run(t, nil, "--hub", e.srv.URL+"/", "--token", "tok", "--name", "pc-01")
	if err != nil {
		t.Fatalf("%v %s", err, out)
	}
	bin, mode := e.file(t, "usr/local/bin/kite-agent")
	if bin != "agent-v1" || mode != 0o755 {
		t.Fatalf("binary %q mode %v", bin, mode)
	}
	envf, mode := e.file(t, "etc/kite/agent.env")
	if mode != 0o600 || !strings.Contains(envf, "HUB_URL="+e.srv.URL+"\n") || !strings.Contains(envf, "AGENT_TOKEN=tok\n") || !strings.Contains(envf, "AGENT_NAME=pc-01\n") {
		t.Fatalf("env %q mode %v", envf, mode)
	}
	unit, _ := e.file(t, "etc/systemd/system/kite-agent.service")
	for _, want := range []string{"EnvironmentFile=/etc/kite/agent.env", "ExecStart=/usr/local/bin/kite-agent", "Restart=always", "Nice=10", "IOSchedulingClass=idle", "WantedBy=multi-user.target"} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit missing %q", want)
		}
	}
	log, _ := os.ReadFile(e.log)
	if string(log) != "daemon-reload\nenable kite-agent\nrestart kite-agent\n" {
		t.Fatalf("systemctl calls %q", log)
	}
	if !strings.Contains(out, "Kite agent installed") {
		t.Fatalf("output %q", out)
	}
}

func TestInstallShUpgrade(t *testing.T) {
	e := newShEnv(t)
	e.run(t, nil, "--hub", e.srv.URL, "--token", "tok")
	e.serve([]byte("agent-v2"))
	if out, err := e.run(t, nil, "--hub", e.srv.URL, "--token", "tok"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if bin, _ := e.file(t, "usr/local/bin/kite-agent"); bin != "agent-v2" {
		t.Fatalf("binary not upgraded: %q", bin)
	}
	envf, _ := e.file(t, "etc/kite/agent.env")
	if strings.Contains(envf, "AGENT_NAME") {
		t.Fatalf("no --name must mean no AGENT_NAME: %q", envf)
	}
	log, _ := os.ReadFile(e.log)
	if strings.Count(string(log), "restart kite-agent") != 2 {
		t.Fatalf("systemctl calls %q", log)
	}
}

func TestInstallShUninstall(t *testing.T) {
	e := newShEnv(t)
	e.run(t, nil, "--hub", e.srv.URL, "--token", "tok")
	if out, err := e.run(t, nil, "--uninstall"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	for _, p := range []string{"usr/local/bin/kite-agent", "etc/kite", "etc/systemd/system/kite-agent.service"} {
		if _, err := os.Stat(filepath.Join(e.prefix, p)); !os.IsNotExist(err) {
			t.Errorf("%s still there", p)
		}
	}
	log, _ := os.ReadFile(e.log)
	if !strings.Contains(string(log), "disable --now kite-agent") {
		t.Fatalf("systemctl calls %q", log)
	}
}

func TestInstallShRefuses(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"no token", nil, []string{"--hub", "http://hub.local:8420"}, "--token"},
		{"unknown cpu", map[string]string{"KITE_ARCH": "mips"}, []string{"--hub", "http://hub.local:8420", "--token", "t"}, "unsupported CPU"},
		{"no systemd", map[string]string{"KITE_SYSTEMCTL": "/nonexistent/systemctl"}, []string{"--hub", "http://hub.local:8420", "--token", "t"}, "systemd"},
		{"not root", map[string]string{"KITE_SKIP_ROOT_CHECK": "0"}, []string{"--hub", "http://hub.local:8420", "--token", "t"}, "root"},
		{"download fails", nil, []string{"--hub", "http://127.0.0.1:1", "--token", "t"}, "download failed"},
		{"systemd not running", map[string]string{"KITE_SYSTEMD_DIR": "/nonexistent/systemd"}, []string{"--hub", "http://hub.local:8420", "--token", "t"}, "systemd is not running"},
		{"name with a space", nil, []string{"--hub", "http://hub.local:8420", "--token", "t", "--name", "Living Room"}, "invalid name"},
		{"name starting with a dot", nil, []string{"--hub", "http://hub.local:8420", "--token", "t", "--name", ".x"}, "invalid name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "not root" && os.Geteuid() == 0 {
				t.Skip("running as root")
			}
			e := newShEnv(t)
			out, err := e.run(t, c.env, c.args...)
			if err == nil || !strings.Contains(out, c.want) {
				t.Fatalf("want failure mentioning %q, got %v %q", c.want, err, out)
			}
			entries, _ := os.ReadDir(e.prefix)
			if len(entries) != 0 {
				t.Fatalf("something was created: %v", entries)
			}
		})
	}
}

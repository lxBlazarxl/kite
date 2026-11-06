package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func sddl(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}

func TestSecureDirCreatesProtected(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Kite")
	if err := secureDir(dir); err != nil {
		t.Fatal(err)
	}
	s := sddl(t, dir)
	if !strings.Contains(s, "D:P") || strings.Contains(s, ";;;BU)") || strings.Contains(s, ";;;AU)") || strings.Contains(s, ";;;WD)") {
		t.Fatalf("dir not locked down: %s", s)
	}
	// Files created inside inherit the protection (agent.env holds the token).
	f := filepath.Join(dir, "agent.env")
	os.WriteFile(f, []byte("x"), 0o600)
	if fs := sddl(t, f); strings.Contains(fs, ";;;BU)") || strings.Contains(fs, ";;;AU)") {
		t.Fatalf("file readable by users: %s", fs)
	}
	if err := secureDir(dir); err != nil {
		t.Fatalf("existing admin-owned dir must be accepted: %v", err)
	}
}

func TestSecureDirRejectsJunction(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	os.Mkdir(target, 0o755)
	link := filepath.Join(base, "Kite")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v %s", err, out)
	}
	if err := secureDir(link); err == nil || !strings.Contains(err.Error(), "reparse") {
		t.Fatalf("junction must be refused, got %v", err)
	}
}

func TestCopyExe(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.exe")
	os.WriteFile(src, []byte("binary"), 0o755)
	dst := filepath.Join(dir, "Program Files", "Kite", "kite-agent.exe")
	if err := copyExe(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "binary" {
		t.Fatalf("copied %q", b)
	}
	if err := copyExe(dst, dst); err != nil {
		t.Fatalf("copying onto itself must be a no-op: %v", err)
	}
}

func TestInstalledExePath(t *testing.T) {
	p := installedExePath()
	if !strings.HasSuffix(strings.ToLower(p), `\kite\kite-agent.exe`) || !strings.Contains(strings.ToLower(p), "program files") {
		t.Fatalf("got %q", p)
	}
}

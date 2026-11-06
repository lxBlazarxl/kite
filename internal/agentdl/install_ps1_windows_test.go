package agentdl

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writePs1(t *testing.T) string {
	t.Helper()
	b, err := fs.ReadFile(embedded, "install.ps1")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "install.ps1")
	os.WriteFile(p, b, 0o644)
	return p
}

func TestInstallPs1Parses(t *testing.T) {
	p := writePs1(t)
	script := "$e=$null; [void][System.Management.Automation.Language.Parser]::ParseFile('" + p + "', [ref]$null, [ref]$e); if ($e.Count) { $e | ForEach-Object { $_.Message }; exit 1 }"
	if out, err := exec.Command("powershell", "-NoProfile", "-Command", script).CombinedOutput(); err != nil {
		t.Fatalf("parse errors: %s", out)
	}
}

func TestInstallPs1RefusesWithoutToken(t *testing.T) {
	p := writePs1(t)
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", p)
	cmd.Env = append(os.Environ(), "KITE_HUB=http://hub.local:8420", "KITE_TOKEN=")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "KITE_TOKEN") {
		t.Fatalf("want a refusal mentioning KITE_TOKEN, got %v %s", err, out)
	}
}

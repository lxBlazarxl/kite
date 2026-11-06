package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAgentVersionFlag(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "kite-agent")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	out, err := exec.Command(bin, "-version").Output()
	if err != nil || string(out) != "kite-agent dev\n" {
		t.Fatalf("got %q %v", out, err)
	}
}

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "agent.log")
	f, err := NewRotatingFile(p, 10)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("12345678"))
	f.Write([]byte("abcdef"))
	f.Close()
	old, _ := os.ReadFile(p + ".1")
	cur, _ := os.ReadFile(p)
	if string(old) != "12345678" || string(cur) != "abcdef" {
		t.Fatalf("old %q cur %q", old, cur)
	}
}

func TestRotatingFileAppends(t *testing.T) {
	p := filepath.Join(t.TempDir(), "agent.log")
	os.WriteFile(p, []byte("x"), 0o644)
	f, _ := NewRotatingFile(p, 100)
	f.Write([]byte("y"))
	f.Close()
	if b, _ := os.ReadFile(p); string(b) != "xy" {
		t.Fatalf("got %q", b)
	}
}

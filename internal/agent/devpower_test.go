package agent

import (
	"encoding/binary"
	"testing"
)

func TestPowerStateFromData(t *testing.T) {
	b := make([]byte, 56)
	binary.LittleEndian.PutUint32(b[0:], 56)
	binary.LittleEndian.PutUint32(b[4:], 1)
	if st, ok := powerStateFromData(b); !ok || st != 1 {
		t.Fatalf("D0: %d %v", st, ok)
	}
	binary.LittleEndian.PutUint32(b[4:], 4)
	if st, ok := powerStateFromData(b); !ok || st != 4 {
		t.Fatalf("D3: %d %v", st, ok)
	}
	if _, ok := powerStateFromData(b[:4]); ok {
		t.Fatal("short buffer must fail")
	}
}

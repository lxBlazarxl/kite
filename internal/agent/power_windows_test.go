package agent

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestReadPowerStatusReal(t *testing.T) {
	ps, err := readPowerStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !ps.HasBattery {
		if !ps.OnAC || ps.Percent != nil {
			t.Fatalf("no battery must mean on AC without a percentage: %+v", ps)
		}
		t.Skip("no battery on this machine")
	}
	if ps.Percent == nil {
		t.Fatalf("battery without a percentage: %+v", ps)
	}
	t.Logf("on AC %v, %.0f%%", ps.OnAC, *ps.Percent)
}

func TestLowerPriorityReal(t *testing.T) {
	if err := LowerPriority(); err != nil {
		t.Fatal(err)
	}
	pc, err := windows.GetPriorityClass(windows.CurrentProcess())
	if err != nil || pc != windows.BELOW_NORMAL_PRIORITY_CLASS {
		t.Fatalf("priority class %#x err %v", pc, err)
	}
}

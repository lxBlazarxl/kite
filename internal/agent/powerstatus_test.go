package agent

import "testing"

func TestDecodePowerStatus(t *testing.T) {
	cases := []struct {
		ac, flag, pct byte
		hasBat, onAC  bool
		percent       float64 // -1 = nil
	}{
		{1, 8, 100, true, true, 100},
		{0, 1, 57, true, false, 57},
		{0, 128, 255, false, true, -1},
		{255, 255, 255, false, true, -1},
	}
	for _, c := range cases {
		got := decodePowerStatus(c.ac, c.flag, c.pct)
		if got.HasBattery != c.hasBat || got.OnAC != c.onAC {
			t.Errorf("%v: got %+v", c, got)
		}
		if (c.percent < 0) != (got.Percent == nil) || (got.Percent != nil && *got.Percent != c.percent) {
			t.Errorf("%v: percent %v", c, got.Percent)
		}
	}
}

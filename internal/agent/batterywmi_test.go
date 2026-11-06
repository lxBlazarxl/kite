package agent

import (
	"math"
	"testing"
)

func TestBatteryFromWMI(t *testing.T) {
	cases := []struct {
		in    wmiBatteryStatus
		state string
		watts float64
	}{
		{wmiBatteryStatus{Discharging: true, DischargeRate: 8200}, "discharging", -8.2},
		{wmiBatteryStatus{PowerOnline: true, Charging: true, ChargeRate: 45000}, "charging", 45},
		{wmiBatteryStatus{PowerOnline: true}, "full", 0},
		{wmiBatteryStatus{}, "idle", 0},
	}
	for _, c := range cases {
		st, w := batteryFromWMI(c.in)
		if st != c.state || math.Abs(w-c.watts) > 1e-9 {
			t.Errorf("%+v: got %q %v", c.in, st, w)
		}
	}
}

func TestKelvinTenthsToC(t *testing.T) {
	if got := kelvinTenthsToC(3242); math.Abs(got-51.05) > 1e-9 {
		t.Fatalf("got %v", got)
	}
}

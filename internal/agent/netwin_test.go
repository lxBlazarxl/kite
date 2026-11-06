package agent

import "testing"

func TestIfRowUsable(t *testing.T) {
	cases := []struct {
		name   string
		flags  uint8
		oper   uint32
		ifType uint32
		want   bool
	}{
		{"Ethernet", 0b00000101, 1, 6, true},
		{"Tailscale", 0, 1, 53, true},
		{"Wi-Fi (down)", 0b00010101, 2, 71, true},
		{"Ethernet-WFP Native MAC Layer LightWeight Filter-0000", 0b00000010, 1, 6, false},
		{"Ethernet (Kernel Debugger)", 0, 6, 6, false},
		{"Loopback Pseudo-Interface 1", 0, 1, 24, false},
		{"Teredo Tunneling Pseudo-Interface", 0, 6, 131, false},
	}
	for _, c := range cases {
		if got := ifRowUsable(c.flags, c.oper, c.ifType); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

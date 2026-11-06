package agent

import (
	"reflect"
	"testing"
)

func TestOrderAdapters(t *testing.T) {
	in := []Adapter{
		{Name: "RTX", VendorID: 0x10DE, LUID: "l1"},
		{Name: "Microsoft Basic Render Driver", VendorID: 0x1414, LUID: "l9"},
		{Name: "Intel UHD", VendorID: 0x8086, LUID: "l3"},
		{Name: "Radeon", VendorID: 0x1002, LUID: "l2"},
		{Name: "RTX", VendorID: 0x10DE, LUID: "l1"},
		{Name: "Soft", VendorID: 0x1234, LUID: "l8", Software: true},
	}
	var got []string
	for _, a := range orderAdapters(in) {
		got = append(got, a.LUID)
	}
	if want := []string{"l2", "l1", "l3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestVendorName(t *testing.T) {
	for id, want := range map[uint32]string{0x1002: "amd", 0x10DE: "nvidia", 0x8086: "intel", 0x5143: "other"} {
		if got := vendorName(id); got != want {
			t.Errorf("%#x -> %q", id, got)
		}
	}
}

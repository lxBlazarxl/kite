package agent

import (
	"reflect"
	"testing"
)

func TestLUIDFromInstance(t *testing.T) {
	cases := map[string]string{
		"pid_1234_luid_0x00000000_0x0000ABCD_phys_0_eng_0_engtype_3D": "0x00000000_0x0000abcd",
		"luid_0x00000000_0x0000ABCD_phys_0":                           "0x00000000_0x0000abcd",
	}
	for in, want := range cases {
		if got, ok := luidFromInstance(in); !ok || got != want {
			t.Errorf("%q -> %q %v", in, got, ok)
		}
	}
	if _, ok := luidFromInstance("_Total"); ok {
		t.Error("no luid must be false")
	}
}

func TestSumByLUID(t *testing.T) {
	vals := []CounterValue{
		{"pid_1_luid_0x0_0xA_phys_0_eng_0_engtype_3D", 60},
		{"pid_2_luid_0x0_0xA_phys_0_eng_1_engtype_3D", 70},
		{"pid_3_luid_0x0_0xB_phys_0_eng_0_engtype_3D", 5},
		{"_Total", 99},
	}
	if got := sumByLUID(vals, 100); !reflect.DeepEqual(got, map[string]float64{"0x0_0xa": 100, "0x0_0xb": 5}) {
		t.Errorf("capped %v", got)
	}
	if got := sumByLUID(vals, 0); got["0x0_0xa"] != 130 {
		t.Errorf("uncapped %v", got)
	}
}

func TestFreqFromPerf(t *testing.T) {
	if got := freqFromPerf(120, 3200); got != 3840 {
		t.Fatalf("got %v", got)
	}
}

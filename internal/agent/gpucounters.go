package agent

import (
	"regexp"
	"strings"
)

// CounterValue is one instance of a Windows performance counter.
type CounterValue struct {
	Instance string
	Value    float64
}

var luidRe = regexp.MustCompile(`(?i)luid_(0x[0-9a-f]+_0x[0-9a-f]+)`)

// luidFromInstance extracts the adapter LUID from a GPU counter instance
// name such as "pid_1234_luid_0x00000000_0x0000ABCD_phys_0_eng_0_engtype_3D",
// lowercased so it matches Adapter.LUID.
func luidFromInstance(instance string) (string, bool) {
	m := luidRe.FindStringSubmatch(instance)
	if m == nil {
		return "", false
	}
	return strings.ToLower(m[1]), true
}

// sumByLUID adds counter values per adapter. With capAt > 0 each sum is
// capped (3D utilisation of several processes can add up past 100 %).
func sumByLUID(vals []CounterValue, capAt float64) map[string]float64 {
	out := map[string]float64{}
	for _, v := range vals {
		if luid, ok := luidFromInstance(v.Instance); ok {
			out[luid] += v.Value
		}
	}
	if capAt > 0 {
		for k, v := range out {
			out[k] = min(v, capAt)
		}
	}
	return out
}

// freqFromPerf turns "% Processor Performance" into MHz.
func freqFromPerf(perfPct, baseMHz float64) float64 {
	return perfPct / 100 * baseMHz
}

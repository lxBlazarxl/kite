package agent

import (
	"testing"
	"time"
)

func TestRatesReset(t *testing.T) {
	r, t0 := NewRates(), time.Unix(1000, 0)
	if _, ok := r.Rate("x", 100, t0); ok {
		t.Fatal("first sample must not have a rate")
	}
	if v, ok := r.Rate("x", 300, t0.Add(2*time.Second)); !ok || v != 100 {
		t.Fatalf("got %v %v", v, ok)
	}
	if _, ok := r.Rate("x", 50, t0.Add(4*time.Second)); ok {
		t.Fatal("decrease must reset, not report")
	}
	if v, ok := r.Rate("x", 150, t0.Add(5*time.Second)); !ok || v != 100 {
		t.Fatalf("after reset got %v %v", v, ok)
	}
}

func TestRatesZeroElapsed(t *testing.T) {
	r, t0 := NewRates(), time.Unix(1000, 0)
	r.Rate("x", 100, t0)
	if _, ok := r.Rate("x", 200, t0); ok {
		t.Fatal("zero elapsed time must not report a rate")
	}
}

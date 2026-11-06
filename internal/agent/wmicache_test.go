package agent

import (
	"errors"
	"testing"
	"time"
)

func TestWMICacheRefreshesEvery30s(t *testing.T) {
	calls, fail := 0, false
	c := newCached(30*time.Second, func() (int, error) {
		calls++
		if fail {
			return 0, errors.New("wmi down")
		}
		return calls, nil
	})
	t0 := time.Unix(1000, 0)
	if v, ok := c.get(t0); !ok || v != 1 || calls != 1 {
		t.Fatalf("first: %v %v calls %d", v, ok, calls)
	}
	if v, _ := c.get(t0.Add(10 * time.Second)); v != 1 || calls != 1 {
		t.Fatalf("cached: %v calls %d", v, calls)
	}
	fail = true
	if v, ok := c.get(t0.Add(31 * time.Second)); !ok || v != 1 || calls != 2 {
		t.Fatalf("failed refresh keeps last value: %v %v calls %d", v, ok, calls)
	}
	if c.get(t0.Add(40 * time.Second)); calls != 2 {
		t.Fatalf("failure retried too soon: calls %d", calls)
	}
	fail = false
	if v, _ := c.get(t0.Add(62 * time.Second)); v != 3 {
		t.Fatalf("recovered value %v", v)
	}
}

func TestWMICacheNeverSucceeded(t *testing.T) {
	c := newCached(30*time.Second, func() (int, error) { return 0, errors.New("no") })
	if _, ok := c.get(time.Unix(0, 0)); ok {
		t.Fatal("no value yet must report ok=false")
	}
}

func TestAsyncCacheNeverBlocks(t *testing.T) {
	release := make(chan struct{})
	c := newAsyncCached(30*time.Second, func() (int, error) {
		<-release
		return 7, nil
	})
	start := time.Now()
	if _, ok := c.get(time.Unix(0, 0)); ok {
		t.Fatal("no value before the first fetch finishes")
	}
	if time.Since(start) > 10*time.Millisecond {
		t.Fatal("get blocked on the fetch")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if v, ok := c.get(time.Unix(1, 0)); ok && v == 7 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("value never arrived")
}

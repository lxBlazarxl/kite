package hub

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/retransmit/kite/internal/protocol"
)

var (
	bg = context.Background()
	t0 = time.Unix(1_700_000_000, 0)
)

func sample(name, os string, cpu float64) *protocol.Sample {
	return &protocol.Sample{
		Version: 1, Machine: name, OS: os, IntervalSec: 5, Time: t0,
		CPU:    &protocol.CPU{TotalPct: cpu},
		Memory: &protocol.Memory{TotalBytes: 100, UsedBytes: 50},
	}
}

func openTest(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kite.db")
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st, path
}

func ingest(t *testing.T, st *Store, s *protocol.Sample, at time.Time) {
	t.Helper()
	if err := st.Ingest(bg, s, at); err != nil {
		t.Fatal(err)
	}
}

func query(t *testing.T, st *Store, res Resolution, from, to time.Time) []SeriesPoint {
	t.Helper()
	got, err := st.Query(bg, "pc", []string{"cpu.total"}, from, to, res)
	if err != nil {
		t.Fatal(err)
	}
	return got["cpu.total"]
}

func TestIngestAndQueryRaw(t *testing.T) {
	st, _ := openTest(t)
	ingest(t, st, sample("pc", "linux", 10), t0)
	ingest(t, st, sample("pc", "linux", 20), t0.Add(5*time.Second))
	got := query(t, st, ResRaw, t0.Add(-time.Second), t0.Add(10*time.Second))
	want := []SeriesPoint{{t0.Unix(), 10, 10}, {t0.Unix() + 5, 20, 20}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestQueryMissingKeyIsEmpty(t *testing.T) {
	st, _ := openTest(t)
	ingest(t, st, sample("pc", "linux", 10), t0)
	got, err := st.Query(bg, "pc", []string{"gpu.0.util"}, t0, t0, ResRaw)
	if err != nil {
		t.Fatal(err)
	}
	if pts, ok := got["gpu.0.util"]; !ok || len(pts) != 0 {
		t.Fatalf("want empty slice for unknown key, got %v (present %v)", pts, ok)
	}
}

func TestIngestUsesReceivedAt(t *testing.T) {
	st, _ := openTest(t)
	s := sample("pc", "linux", 10)
	s.Time = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	ingest(t, st, s, t0)
	got := query(t, st, ResRaw, t0.Add(-time.Hour), t0.Add(time.Hour))
	if len(got) != 1 || got[0].TS != t0.Unix() {
		t.Fatalf("got %v", got)
	}
}

func TestSameSecondReplaces(t *testing.T) {
	st, _ := openTest(t)
	ingest(t, st, sample("pc", "linux", 10), t0)
	ingest(t, st, sample("pc", "linux", 30), t0.Add(300*time.Millisecond))
	got := query(t, st, ResRaw, t0, t0)
	if len(got) != 1 || got[0].Avg != 30 {
		t.Fatalf("got %v", got)
	}
}

func TestMachinesLatest(t *testing.T) {
	st, _ := openTest(t)
	ingest(t, st, sample("pc", "linux", 10), t0)
	ingest(t, st, sample("pc", "linux", 20), t0.Add(5*time.Second))
	ms, err := st.Machines(bg)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 {
		t.Fatalf("got %d machines", len(ms))
	}
	m := ms[0]
	if m.Name != "pc" || m.OS != "linux" || m.IntervalSec != 5 || !m.LastSeen.Equal(t0.Add(5*time.Second)) ||
		!strings.Contains(string(m.Latest), `"total_pct":20`) {
		t.Fatalf("got %+v latest %s", m, m.Latest)
	}
}

func TestDualBootSameName(t *testing.T) {
	st, _ := openTest(t)
	ingest(t, st, sample("pc", "linux", 10), t0)
	ingest(t, st, sample("pc", "windows", 10), t0.Add(time.Hour))
	ms, _ := st.Machines(bg)
	if len(ms) != 1 || ms[0].OS != "windows" {
		t.Fatalf("got %+v", ms)
	}
}

func TestKeysSorted(t *testing.T) {
	st, _ := openTest(t)
	ingest(t, st, sample("pc", "linux", 10), t0)
	keys, err := st.Keys(bg, "pc")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cpu.total", "mem.total", "mem.used", "swap.total", "swap.used"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("got %v", keys)
	}
	if _, err := st.Keys(bg, "nope"); !errors.Is(err, ErrUnknownMachine) {
		t.Fatalf("got %v", err)
	}
}

func TestDeleteMachine(t *testing.T) {
	st, _ := openTest(t)
	ingest(t, st, sample("pc", "linux", 10), t0)
	if ok, err := st.DeleteMachine(bg, "pc"); !ok || err != nil {
		t.Fatalf("delete: %v %v", ok, err)
	}
	if _, err := st.Query(bg, "pc", []string{"cpu.total"}, t0, t0, ResRaw); !errors.Is(err, ErrUnknownMachine) {
		t.Fatalf("query after delete: %v", err)
	}
	var n int
	if err := st.db.QueryRow(`SELECT count(*) FROM raw`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("raw rows left: %d %v", n, err)
	}
	if ok, _ := st.DeleteMachine(bg, "pc"); ok {
		t.Fatal("second delete must report false")
	}
	ingest(t, st, sample("pc", "linux", 7), t0.Add(time.Minute))
	if got := query(t, st, ResRaw, t0, t0.Add(time.Hour)); len(got) != 1 || got[0].Avg != 7 {
		t.Fatalf("re-ingest after delete: %v", got)
	}
}

func TestReopenPersists(t *testing.T) {
	st, path := openTest(t)
	ingest(t, st, sample("pc", "linux", 10), t0)
	sec1, err := st.Secret(bg)
	if err != nil || len(sec1) != 32 {
		t.Fatalf("secret %v %v", sec1, err)
	}
	st.Close()
	st2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	if got := query(t, st2, ResRaw, t0, t0); len(got) != 1 {
		t.Fatalf("after reopen: %v", got)
	}
	sec2, _ := st2.Secret(bg)
	if string(sec1) != string(sec2) {
		t.Fatal("secret changed across reopen")
	}
}

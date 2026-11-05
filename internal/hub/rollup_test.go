package hub

import (
	"reflect"
	"testing"
	"time"
)

func at(sec int64) time.Time { return time.Unix(sec, 0) }

func seedRollup(t *testing.T, st *Store) {
	t.Helper()
	for i := int64(0); i < 12; i++ {
		ingest(t, st, sample("pc", "linux", float64(i+1)), at(60+5*i))
	}
	ingest(t, st, sample("pc", "linux", 100), at(120))
	ingest(t, st, sample("pc", "linux", 200), at(125))
	ingest(t, st, sample("pc", "linux", 999), at(180))
}

func TestRollup(t *testing.T) {
	st, _ := openTest(t)
	seedRollup(t, st)
	if err := st.Rollup(bg, at(181)); err != nil {
		t.Fatal(err)
	}
	want := []SeriesPoint{{60, 6.5, 12}, {120, 150, 200}}
	if got := query(t, st, Res1m, at(0), at(1000)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if err := st.Rollup(bg, at(181)); err != nil {
		t.Fatal(err)
	}
	if got := query(t, st, Res1m, at(0), at(1000)); !reflect.DeepEqual(got, want) {
		t.Fatalf("second run changed rows: %v", got)
	}
}

func TestRollupResumesAfterReopen(t *testing.T) {
	st, path := openTest(t)
	seedRollup(t, st)
	if err := st.Rollup(bg, at(181)); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	if err := st2.Rollup(bg, at(241)); err != nil {
		t.Fatal(err)
	}
	want := []SeriesPoint{{60, 6.5, 12}, {120, 150, 200}, {180, 999, 999}}
	got, _ := st2.Query(bg, "pc", []string{"cpu.total"}, at(0), at(1000), Res1m)
	if !reflect.DeepEqual(got["cpu.total"], want) {
		t.Fatalf("got %v", got["cpu.total"])
	}
}

func TestRollupCatchesUpLongGap(t *testing.T) {
	st, _ := openTest(t)
	ingest(t, st, sample("pc", "linux", 1), at(60))
	ingest(t, st, sample("pc", "linux", 2), at(60+3*86400))
	if err := st.Rollup(bg, at(60+3*86400+120)); err != nil {
		t.Fatal(err)
	}
	got := query(t, st, Res1m, at(0), at(4*86400))
	if len(got) != 2 || got[1].TS != 60+3*86400 {
		t.Fatalf("did not catch up across 3 days: %v", got)
	}
}

func TestPrune(t *testing.T) {
	st, _ := openTest(t)
	now := t0.Add(8 * 24 * time.Hour)
	for _, ts := range []time.Time{t0, now.Add(-25 * time.Hour), now.Add(-time.Hour)} {
		ingest(t, st, sample("pc", "linux", 1), ts)
	}
	if err := st.Rollup(bg, now); err != nil {
		t.Fatal(err)
	}
	if err := st.Prune(bg, now, 24*time.Hour, 168*time.Hour); err != nil {
		t.Fatal(err)
	}
	if raw := query(t, st, ResRaw, t0, now); len(raw) != 1 || raw[0].TS != now.Add(-time.Hour).Unix() {
		t.Fatalf("raw after prune: %v", raw)
	}
	roll := query(t, st, Res1m, t0, now)
	if len(roll) != 2 {
		t.Fatalf("rollup after prune: %v (want the 25h and 1h old minutes kept, 8d old gone)", roll)
	}
}

func TestChooseResolution(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		from, to time.Time
		want     Resolution
	}{
		{t0.Add(-time.Hour), t0, ResRaw},
		{t0.Add(-3 * time.Hour), t0, ResRaw},
		{t0.Add(-6 * time.Hour), t0, Res1m},
		{t0.Add(-30 * time.Hour), t0.Add(-29 * time.Hour), Res1m},
	}
	for _, c := range cases {
		if got := ChooseResolution(c.from, c.to, t0, day); got != c.want {
			t.Errorf("%v..%v: got %s want %s", c.from, c.to, got, c.want)
		}
	}
}

func TestChooseResolutionTiers(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		from, to time.Time
		want     Resolution
	}{
		{t0.Add(-time.Hour), t0, ResRaw},
		{t0.Add(-3 * time.Hour), t0, ResRaw},
		{t0.Add(-6 * time.Hour), t0, Res1m},
		{t0.Add(-36 * time.Hour), t0, Res1m},
		{t0.Add(-37 * time.Hour), t0, Res10m},
		{t0.Add(-7 * day), t0, Res10m},
		{t0.Add(-30 * time.Hour), t0.Add(-29 * time.Hour), Res1m},
	}
	for _, c := range cases {
		if got := ChooseResolution(c.from, c.to, t0, day); got != c.want {
			t.Errorf("%v: got %s want %s", c.to.Sub(c.from), got, c.want)
		}
	}
}

func TestQuery10m(t *testing.T) {
	st, _ := openTest(t)
	for ts := int64(600); ts < 1200; ts += 60 {
		ingest(t, st, sample("pc", "linux", 1), at(ts))
	}
	ingest(t, st, sample("pc", "linux", 5), at(1200))
	if err := st.Rollup(bg, at(1300)); err != nil {
		t.Fatal(err)
	}
	want := []SeriesPoint{{600, 1, 1}, {1200, 5, 5}}
	if got := query(t, st, Res10m, at(0), at(2000)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

package agent

import (
	"testing"
	"time"
)

func TestEnumAdaptersReal(t *testing.T) {
	raw, err := enumAdapters()
	if err != nil {
		t.Fatal(err)
	}
	as := orderAdapters(raw)
	t.Logf("raw %+v\nordered %+v", raw, as)
	if len(as) != 2 || as[0].VendorID != 0x1002 || as[1].VendorID != 0x10DE {
		t.Fatalf("want AMD then NVIDIA, got %+v", as)
	}
	q, err := newPDHQuery(`\GPU Adapter Memory(*)\Dedicated Usage`)
	if err != nil {
		t.Fatal(err)
	}
	defer q.close()
	time.Sleep(200 * time.Millisecond)
	vals, err := q.collect()
	if err != nil {
		t.Fatal(err)
	}
	pdhLUIDs := sumByLUID(vals[0], 0)
	for _, a := range as {
		if _, ok := pdhLUIDs[a.LUID]; !ok {
			t.Errorf("adapter %s LUID %s not among PDH LUIDs %v", a.Name, a.LUID, pdhLUIDs)
		}
	}
}

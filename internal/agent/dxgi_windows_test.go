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
	if len(as) == 0 {
		t.Fatal("no usable display adapter")
	}
	for i := 1; i < len(as); i++ {
		if vendorRank(as[i-1].VendorID) > vendorRank(as[i].VendorID) {
			t.Fatalf("adapters not ordered AMD, NVIDIA, others: %+v", as)
		}
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

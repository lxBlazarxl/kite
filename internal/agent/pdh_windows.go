package agent

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pdh                              = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQueryW                = pdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounterW        = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData          = pdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterArrayW = pdh.NewProc("PdhGetFormattedCounterArrayW")
	procPdhCloseQuery                = pdh.NewProc("PdhCloseQuery")
)

const (
	pdhFmtDouble    = 0x00000200
	pdhFmtNoCap100  = 0x00008000
	pdhMoreData     = 0x800007D2
	pdhCstatusValid = 0x0
	pdhCstatusNew   = 0x1
)

// pdhItemDouble mirrors PDH_FMT_COUNTERVALUE_ITEM_DOUBLE.
type pdhItemDouble struct {
	Name   *uint16
	Status uint32
	_      uint32
	Value  float64
}

// pdhQuery reads a fixed set of (possibly wildcard) counters. PDH is the
// same API Task Manager uses; a collect costs microseconds.
type pdhQuery struct {
	query    uintptr
	counters []uintptr
	buf      []byte
}

func pdhErr(call string, r uintptr) error {
	return fmt.Errorf("%s: PDH status 0x%08x", call, uint32(r))
}

func newPDHQuery(paths ...string) (*pdhQuery, error) {
	q := &pdhQuery{}
	if r, _, _ := procPdhOpenQueryW.Call(0, 0, uintptr(unsafe.Pointer(&q.query))); r != 0 {
		return nil, pdhErr("PdhOpenQuery", r)
	}
	for _, p := range paths {
		ptr, err := windows.UTF16PtrFromString(p)
		if err != nil {
			q.close()
			return nil, err
		}
		var c uintptr
		if r, _, _ := procPdhAddEnglishCounterW.Call(q.query, uintptr(unsafe.Pointer(ptr)), 0, uintptr(unsafe.Pointer(&c))); r != 0 {
			q.close()
			return nil, fmt.Errorf("%s: %w", p, pdhErr("PdhAddEnglishCounter", r))
		}
		q.counters = append(q.counters, c)
	}
	// Rate counters need a first sample to diff against.
	procPdhCollectQueryData.Call(q.query)
	return q, nil
}

// collect returns one slice of instance values per counter path.
func (q *pdhQuery) collect() ([][]CounterValue, error) {
	if r, _, _ := procPdhCollectQueryData.Call(q.query); r != 0 {
		return nil, pdhErr("PdhCollectQueryData", r)
	}
	out := make([][]CounterValue, len(q.counters))
	for i, c := range q.counters {
		vals, err := q.read(c)
		if err != nil {
			return nil, err
		}
		out[i] = vals
	}
	return out, nil
}

func (q *pdhQuery) read(counter uintptr) ([]CounterValue, error) {
	for attempt := 0; attempt < 3; attempt++ {
		size := uint32(len(q.buf))
		var count uint32
		var bufPtr uintptr
		if size > 0 {
			bufPtr = uintptr(unsafe.Pointer(&q.buf[0]))
		}
		r, _, _ := procPdhGetFormattedCounterArrayW.Call(counter, pdhFmtDouble|pdhFmtNoCap100,
			uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), bufPtr)
		if r == pdhMoreData {
			q.buf = make([]byte, size+1024) // instances come and go; leave headroom
			continue
		}
		if r != 0 {
			return nil, pdhErr("PdhGetFormattedCounterArray", r)
		}
		vals := make([]CounterValue, 0, count)
		if count == 0 {
			return vals, nil
		}
		items := unsafe.Slice((*pdhItemDouble)(unsafe.Pointer(&q.buf[0])), count)
		for _, it := range items {
			if it.Status != pdhCstatusValid && it.Status != pdhCstatusNew {
				continue
			}
			vals = append(vals, CounterValue{Instance: windows.UTF16PtrToString(it.Name), Value: it.Value})
		}
		return vals, nil
	}
	return nil, fmt.Errorf("PdhGetFormattedCounterArray: buffer kept growing")
}

func (q *pdhQuery) close() {
	if q.query != 0 {
		procPdhCloseQuery.Call(q.query)
		q.query = 0
	}
}

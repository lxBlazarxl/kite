package agent

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procCreateDXGIFactory1 = windows.NewLazySystemDLL("dxgi.dll").NewProc("CreateDXGIFactory1")

var iidIDXGIFactory1 = windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}

const (
	dxgiErrorNotFound   = 0x887A0002
	dxgiAdapterSoftware = 2
	vtblRelease         = 2
	vtblEnumAdapters1   = 12 // IDXGIFactory1
	vtblAdapterGetDesc1 = 10 // IDXGIAdapter1
)

// dxgiAdapterDesc1 mirrors DXGI_ADAPTER_DESC1.
type dxgiAdapterDesc1 struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	LUIDLow               uint32
	LUIDHigh              int32
	Flags                 uint32
}

// comObject is the start of every COM object: a pointer to its vtable.
type comObject struct{ vtbl *[32]uintptr }

// comCall invokes method index i of the COM object obj.
func comCall(obj *comObject, i int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(obj.vtbl[i], append([]uintptr{uintptr(unsafe.Pointer(obj))}, args...)...)
	return r
}

// enumAdapters lists display adapters once (at startup) with their LUIDs,
// which key the GPU performance counters.
func enumAdapters() ([]Adapter, error) {
	var factory *comObject
	if r, _, _ := procCreateDXGIFactory1.Call(uintptr(unsafe.Pointer(&iidIDXGIFactory1)), uintptr(unsafe.Pointer(&factory))); r != 0 {
		return nil, fmt.Errorf("CreateDXGIFactory1: 0x%08x", uint32(r))
	}
	defer comCall(factory, vtblRelease)

	var out []Adapter
	for i := uintptr(0); ; i++ {
		var adapter *comObject
		r := comCall(factory, vtblEnumAdapters1, i, uintptr(unsafe.Pointer(&adapter)))
		if uint32(r) == dxgiErrorNotFound {
			break
		}
		if r != 0 {
			return out, fmt.Errorf("EnumAdapters1(%d): 0x%08x", i, uint32(r))
		}
		var d dxgiAdapterDesc1
		r = comCall(adapter, vtblAdapterGetDesc1, uintptr(unsafe.Pointer(&d)))
		comCall(adapter, vtblRelease)
		if r != 0 {
			continue
		}
		out = append(out, Adapter{
			Name:     windows.UTF16ToString(d.Description[:]),
			VendorID: d.VendorID,
			DeviceID: d.DeviceID,
			LUID:     fmt.Sprintf("0x%08x_0x%08x", uint32(d.LUIDHigh), d.LUIDLow),
			Software: d.Flags&dxgiAdapterSoftware != 0,
		})
	}
	return out, nil
}

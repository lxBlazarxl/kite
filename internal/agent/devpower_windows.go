package agent

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	cfgmgr32                   = windows.NewLazySystemDLL("cfgmgr32.dll")
	procCMGetDeviceIDListSizeW = cfgmgr32.NewProc("CM_Get_Device_ID_List_SizeW")
	procCMGetDeviceIDListW     = cfgmgr32.NewProc("CM_Get_Device_ID_ListW")
	procCMLocateDevNodeW       = cfgmgr32.NewProc("CM_Locate_DevNodeW")
	procCMGetDevNodePropertyW  = cfgmgr32.NewProc("CM_Get_DevNode_PropertyW")
	devpkeyDeviceFmtid         = windows.GUID{Data1: 0xa45c254e, Data2: 0xdf1c, Data3: 0x4efd, Data4: [8]byte{0x80, 0x20, 0x67, 0xd1, 0x46, 0xa8, 0x50, 0xe0}}
	guidDevClassDisplay        = windows.GUID{Data1: 0x4d36e968, Data2: 0xe325, Data3: 0x11ce, Data4: [8]byte{0xbf, 0xc1, 0x08, 0x00, 0x2b, 0xe1, 0x03, 0x18}}
)

const (
	cmGetIDListFilterEnumerator = 0x00000001
	cmGetIDListFilterPresent    = 0x00000100
	pidDeviceClassGUID          = 10
	pidDevicePowerData          = 32
)

type devPropKey struct {
	Fmtid windows.GUID
	Pid   uint32
}

func devNodeProperty(devinst uint32, pid uint32) ([]byte, error) {
	key := devPropKey{devpkeyDeviceFmtid, pid}
	var typ uint32
	buf := make([]byte, 256)
	size := uint32(len(buf))
	if r, _, _ := procCMGetDevNodePropertyW.Call(uintptr(devinst), uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&typ)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0); r != 0 {
		return nil, fmt.Errorf("CM_Get_DevNode_Property(%d): CR 0x%x", pid, r)
	}
	return buf[:size], nil
}

// nvidiaAwake reports whether any present NVIDIA display device is in D0.
// It reads the PnP manager's cached power data and never opens the device,
// so asking does not wake a sleeping GPU.
func nvidiaAwake() (bool, error) {
	filter, _ := windows.UTF16PtrFromString("PCI")
	flags := uintptr(cmGetIDListFilterEnumerator | cmGetIDListFilterPresent)
	var size uint32
	if r, _, _ := procCMGetDeviceIDListSizeW.Call(uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(filter)), flags); r != 0 {
		return false, fmt.Errorf("CM_Get_Device_ID_List_Size: CR 0x%x", r)
	}
	list := make([]uint16, size)
	if r, _, _ := procCMGetDeviceIDListW.Call(uintptr(unsafe.Pointer(filter)), uintptr(unsafe.Pointer(&list[0])), uintptr(size), flags); r != 0 {
		return false, fmt.Errorf("CM_Get_Device_ID_List: CR 0x%x", r)
	}
	found := false
	// The list is double-NUL terminated; walk it manually.
	start := 0
	for i, c := range list {
		if c != 0 {
			continue
		}
		if i == start {
			break
		}
		id := windows.UTF16ToString(list[start:i])
		start = i + 1
		if !strings.Contains(strings.ToUpper(id), "VEN_10DE") {
			continue
		}
		idPtr, _ := windows.UTF16PtrFromString(id)
		var devinst uint32
		if r, _, _ := procCMLocateDevNodeW.Call(uintptr(unsafe.Pointer(&devinst)), uintptr(unsafe.Pointer(idPtr)), 0); r != 0 {
			continue
		}
		cls, err := devNodeProperty(devinst, pidDeviceClassGUID)
		if err != nil || len(cls) < 16 || *(*windows.GUID)(unsafe.Pointer(&cls[0])) != guidDevClassDisplay {
			continue // the GPU's HDMI audio function and the like
		}
		found = true
		pd, err := devNodeProperty(devinst, pidDevicePowerData)
		if err != nil {
			return false, err
		}
		if st, ok := powerStateFromData(pd); ok && st == powerDeviceD0 {
			return true, nil
		}
	}
	if !found {
		return false, fmt.Errorf("no NVIDIA display device found")
	}
	return false, nil
}

package agent

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// nvmlLib calls nvml.dll directly: microseconds per call and no
// nvidia-smi process, which matters while gaming.
type nvmlLib struct {
	count, handle, name, util, mem, temp, power *windows.LazyProc
}

type nvmlUtilization struct{ GPU, Memory uint32 }
type nvmlMemory struct{ Total, Free, Used uint64 }

const nvmlTemperatureGPU = 0

func loadNVML() (*nvmlLib, error) {
	dll := windows.NewLazySystemDLL("nvml.dll")
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("nvml.dll: %w", err)
	}
	if r, _, _ := dll.NewProc("nvmlInit_v2").Call(); r != 0 {
		return nil, fmt.Errorf("nvmlInit_v2: %d", r)
	}
	return &nvmlLib{
		count:  dll.NewProc("nvmlDeviceGetCount_v2"),
		handle: dll.NewProc("nvmlDeviceGetHandleByIndex_v2"),
		name:   dll.NewProc("nvmlDeviceGetName"),
		util:   dll.NewProc("nvmlDeviceGetUtilizationRates"),
		mem:    dll.NewProc("nvmlDeviceGetMemoryInfo"),
		temp:   dll.NewProc("nvmlDeviceGetTemperature"),
		power:  dll.NewProc("nvmlDeviceGetPowerUsage"),
	}, nil
}

func (n *nvmlLib) read() ([]NVMLReading, error) {
	var count uint32
	if r, _, _ := n.count.Call(uintptr(unsafe.Pointer(&count))); r != 0 {
		return nil, fmt.Errorf("nvmlDeviceGetCount: %d", r)
	}
	out := make([]NVMLReading, 0, count)
	for i := uint32(0); i < count; i++ {
		var dev uintptr
		if r, _, _ := n.handle.Call(uintptr(i), uintptr(unsafe.Pointer(&dev))); r != 0 {
			continue
		}
		var rd NVMLReading
		name := make([]byte, 96)
		if r, _, _ := n.name.Call(dev, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name))); r == 0 {
			rd.Name = windows.ByteSliceToString(name)
		}
		var u nvmlUtilization
		if r, _, _ := n.util.Call(dev, uintptr(unsafe.Pointer(&u))); r == 0 {
			v := float64(u.GPU)
			rd.UtilPct = &v
		}
		var m nvmlMemory
		if r, _, _ := n.mem.Call(dev, uintptr(unsafe.Pointer(&m))); r == 0 {
			rd.VRAMUsed, rd.VRAMTotal = &m.Used, &m.Total
		}
		var t uint32
		if r, _, _ := n.temp.Call(dev, nvmlTemperatureGPU, uintptr(unsafe.Pointer(&t))); r == 0 {
			v := float64(t)
			rd.TempC = &v
		}
		var mw uint32
		if r, _, _ := n.power.Call(dev, uintptr(unsafe.Pointer(&mw))); r == 0 {
			v := float64(mw) / 1000
			rd.PowerW = &v
		}
		out = append(out, rd)
	}
	return out, nil
}

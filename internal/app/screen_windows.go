//go:build windows

package app

import (
	"syscall"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procMonitorFromPoint = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW  = user32.NewProc("GetMonitorInfoW")
)

const monitorDefaultToNearest = 2

type winRect struct {
	Left, Top, Right, Bottom int32
}

type winMonitorInfo struct {
	CbSize    uint32
	RcMonitor winRect
	RcWork    winRect
	DwFlags   uint32
}

// monitorBoundsAt returns the virtual-desktop bounds of whichever
// monitor contains (x, y). Wails' ScreenGetAll doesn't report each
// screen's position within the virtual desktop, so on a multi-monitor
// setup there's no way to tell where a non-primary monitor's edges
// actually are without asking Windows directly.
func monitorBoundsAt(x, y int) (left, top, right, bottom int, ok bool) {
	// POINT is two int32s; the Windows x64/arm64 calling convention
	// passes an 8-byte struct like this packed into a single register.
	point := uintptr(uint32(x)) | uintptr(uint32(y))<<32

	hMonitor, _, _ := procMonitorFromPoint.Call(point, monitorDefaultToNearest)
	if hMonitor == 0 {
		return 0, 0, 0, 0, false
	}

	mi := winMonitorInfo{CbSize: uint32(unsafe.Sizeof(winMonitorInfo{}))}
	ret, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
	if ret == 0 {
		return 0, 0, 0, 0, false
	}

	return int(mi.RcMonitor.Left), int(mi.RcMonitor.Top), int(mi.RcMonitor.Right), int(mi.RcMonitor.Bottom), true
}

// workAreaOriginAt returns the top-left corner (in virtual-desktop
// coordinates) of the work area of whichever monitor contains (x, y).
// Wails' WindowSetPosition on Windows treats its x/y as an offset from
// this origin rather than as an absolute desktop coordinate (unlike
// WindowGetPosition, which does return an absolute one) - so setting
// an absolute position means subtracting this first.
func workAreaOriginAt(x, y int) (originX, originY int, ok bool) {
	point := uintptr(uint32(x)) | uintptr(uint32(y))<<32

	hMonitor, _, _ := procMonitorFromPoint.Call(point, monitorDefaultToNearest)
	if hMonitor == 0 {
		return 0, 0, false
	}

	mi := winMonitorInfo{CbSize: uint32(unsafe.Sizeof(winMonitorInfo{}))}
	ret, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
	if ret == 0 {
		return 0, 0, false
	}

	return int(mi.RcWork.Left), int(mi.RcWork.Top), true
}

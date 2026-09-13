//go:build windows

package app

import (
	"os"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procMonitorFromPoint         = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	procFindWindowExW            = user32.NewProc("FindWindowExW")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
)

const (
	monitorDefaultToNearest = 2

	swpNoSize     = 0x0001
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	// Wails' default class name for its main window; run.go doesn't
	// override it.
	wailsWindowClass = "wailsWindow"
)

// mainWindow caches the app window's handle once found. Only a
// successful lookup is cached, so an early miss is retried next call.
var mainWindow atomic.Uintptr

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

// findMainWindow returns this process's Wails window. Wails doesn't
// expose the handle, so it's looked up by class name - filtered to this
// process, since a second Wails app running alongside registers the
// same class.
func findMainWindow() uintptr {
	if hwnd := mainWindow.Load(); hwnd != 0 {
		return hwnd
	}

	className, err := syscall.UTF16PtrFromString(wailsWindowClass)
	if err != nil {
		return 0
	}
	pid := uint32(os.Getpid())

	var hwnd uintptr
	for {
		hwnd, _, _ = procFindWindowExW.Call(0, hwnd, uintptr(unsafe.Pointer(className)), 0)
		if hwnd == 0 {
			return 0
		}
		var windowPid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&windowPid)))
		if windowPid == pid {
			mainWindow.Store(hwnd)
			return hwnd
		}
	}
}

// moveWindowNative puts the window's top-left corner at an absolute
// virtual-desktop coordinate, bypassing Wails' WindowSetPosition.
//
// Wails offsets whatever it's given by the work area of the monitor the
// window is on *before* the move. Compensating for that from outside
// means guessing which monitor Windows will pick, and a guess based on
// where the window is going rather than where it is was wrong for
// exactly one frame per crossing - long enough to throw the window a
// whole screen back, where it stayed until dragged a second time over
// the edge. Setting the position directly leaves nothing to guess.
func moveWindowNative(x, y int) bool {
	hwnd := findMainWindow()
	if hwnd == 0 {
		return false
	}
	ret, _, _ := procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
	return ret != 0
}

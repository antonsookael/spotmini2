//go:build darwin

package app

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

typedef struct {
	double left, top, right, bottom;
	int ok;
} MonitorBounds;

// currentMonitorBounds returns the full screen rect (not the visible
// work area) of the screen the window is on, in the coordinate space
// Wails' GetPosition/SetPosition use on macOS: top-down, relative to
// that screen's *visible* frame origin. Like Windows' monitorBoundsAt
// it uses the full rect, so the window can snap flush to the true edge
// and tuck behind the menu bar/Dock.
//
// Derived from screen.frame (F) and visibleFrame (V), inverting Wails'
// transform out of Cocoa's bottom-left-origin space:
//   left   = F.origin.x - V.origin.x
//   right  = left + F.width
//   bottom = (V.origin.y - F.origin.y) + V.height
//   top    = bottom - F.height
MonitorBounds currentMonitorBounds() {
	MonitorBounds mb = {0, 0, 0, 0, 0};

	NSWindow *win = [NSApp mainWindow];
	if (win == nil) {
		for (NSWindow *w in [NSApp windows]) {
			if ([w isVisible]) {
				win = w;
				break;
			}
		}
	}
	if (win == nil) {
		return mb;
	}

	NSScreen *screen = [win screen];
	if (screen == nil) {
		return mb;
	}

	NSRect f = [screen frame];
	NSRect v = [screen visibleFrame];

	mb.left = f.origin.x - v.origin.x;
	mb.right = mb.left + f.size.width;
	mb.bottom = (v.origin.y - f.origin.y) + v.size.height;
	mb.top = mb.bottom - f.size.height;
	mb.ok = 1;
	return mb;
}
*/
import "C"

// monitorBoundsAt returns the full-screen bounds of the screen the
// app's window currently sits on, in the coordinate space
// WindowGetPosition/WindowSetPosition use on macOS. (x, y) is unused:
// unlike Windows there's no "which monitor is this absolute point on"
// question to answer, since macOS positions are already relative to
// whichever screen it considers current.
func monitorBoundsAt(x, y int) (left, top, right, bottom int, ok bool) {
	mb := C.currentMonitorBounds()
	if mb.ok == 0 {
		return 0, 0, 0, 0, false
	}
	return int(mb.left), int(mb.top), int(mb.right), int(mb.bottom), true
}

// workAreaOriginAt has no compensation to do on macOS - WindowSetPosition
// already takes coordinates relative to the current screen's visible
// frame origin (see monitorBoundsAt), so passthrough is correct.
func workAreaOriginAt(x, y int) (originX, originY int, ok bool) {
	return 0, 0, false
}

// moveWindowNative is a Windows workaround; WindowSetPosition is already
// correct on macOS.
func moveWindowNative(x, y int) bool {
	return false
}

// Only Linux prepares, sizes and shows its window natively; see
// screen_linux.go.
const nativeStartHidden = false

func prepareNativeWindow(width, height int) {}

func resizeWindowNative(width, height int) bool {
	return false
}

func windowSizeNative() (width, height int, ok bool) {
	return 0, 0, false
}

// The window outlives OnShutdown here, so its position can still be
// asked for then; see positionAtShutdown in windowpos.go.
const windowGoneAtShutdown = false

func lastWindowPositionNative() (x, y int, ok bool) {
	return 0, 0, false
}

func focusWindowNative() bool {
	return false
}

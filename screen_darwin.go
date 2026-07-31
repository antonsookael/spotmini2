//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

typedef struct {
	double left, top, right, bottom;
	int ok;
} MonitorBounds;

// currentMonitorBounds returns the full screen (not just the visible
// work area) of whichever screen the app's window is on, translated
// into the coordinate space Wails' own GetPosition/SetPosition use on
// macOS: top-down and relative to that screen's *visible* frame origin
// (see WailsContext.m's SetPosition/GetPosition, which measure against
// -screen.visibleFrame, not -screen.frame). Matching Windows'
// monitorBoundsAt (which uses the full RcMonitor rect, not the
// taskbar-excluded work area), this lets the window snap flush against
// the true screen edge, tucking behind the menu bar/Dock rather than
// stopping short of it.
//
// The four bounds are derived from screen.frame (F) and
// screen.visibleFrame (V), both in Cocoa's native bottom-left-origin,
// y-up space, by inverting Wails' own coordinate transform:
//   left   = F.origin.x - V.origin.x
//   right  = left + F.width
//   bottom = (V.origin.y - F.origin.y) + V.height
//   top    = bottom - F.height
// Each works out independent of window size, so callers don't need to
// know the window's dimensions to use them (right-left and bottom-top
// both always equal the screen's full width/height).
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

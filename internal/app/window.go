package app

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// snapThreshold is how close (in px) the window has to be to a screen
// edge, once the drag is released, before it snaps flush against it.
const snapThreshold = 24

// SetWindowWidth resizes the window horizontally, then pulls it back
// on-screen if the new width ran it past a monitor edge.
//
// The resize itself always anchors the top-left corner and grows to the
// right, so auto-fit widening a window that's sitting against the right
// edge would otherwise push its tail off-screen. Re-anchoring to the
// edge keeps a window that was flush flush, and one that wasn't fully
// visible.
//
// Lives in Go rather than the frontend because only monitorBoundsAt
// knows where the current monitor actually ends - on a multi-monitor
// desktop the browser has no idea.
func (a *App) SetWindowWidth(width int) {
	oldWidth, height := runtime.WindowGetSize(a.ctx)
	if oldWidth == width {
		return
	}
	x, y := runtime.WindowGetPosition(a.ctx)

	// Sampled before the resize, so the point is guaranteed to still be
	// inside the window's current monitor.
	centerX, centerY := x+oldWidth/2, y+height/2

	runtime.WindowSetSize(a.ctx, width, height)

	left, _, right, _, ok := monitorBoundsAt(centerX, centerY)
	if !ok {
		return
	}

	newX := x
	if newX+width > right {
		newX = right - width
	}
	// After the clamp above, in case the window is wider than the
	// monitor - staying flush left is better than hanging off both ends.
	if newX < left {
		newX = left
	}
	if newX != x {
		a.setAbsoluteWindowPosition(newX, y)
	}
}

// setAbsoluteWindowPosition moves the window to an absolute desktop
// coordinate. Windows needs compensation first (see workAreaOriginAt);
// elsewhere it's a passthrough.
func (a *App) setAbsoluteWindowPosition(x, y int) {
	curX, curY := runtime.WindowGetPosition(a.ctx)
	width, height := runtime.WindowGetSize(a.ctx)

	originX, originY, ok := workAreaOriginAt(curX+width/2, curY+height/2)
	if !ok {
		runtime.WindowSetPosition(a.ctx, x, y)
		return
	}
	runtime.WindowSetPosition(a.ctx, x-originX, y-originY)
}

// BeginDrag resolves the coordinate-compensation origin (see
// workAreaOriginAt) once per drag so DragWindowTo can reuse it. Doing
// it per-mousemove instead cost two syscalls plus a size query every
// frame, which made dragging visibly laggy.
func (a *App) BeginDrag() {
	x, y := runtime.WindowGetPosition(a.ctx)
	width, height := runtime.WindowGetSize(a.ctx)
	originX, originY, ok := workAreaOriginAt(x+width/2, y+height/2)

	a.dragMu.Lock()
	a.dragActive = true
	a.dragOriginResolved = ok
	a.dragOriginX, a.dragOriginY = originX, originY
	a.dragMu.Unlock()
}

// EndDrag marks the drag finished, so a stray DragWindowTo arriving
// after mouseup resolves a fresh origin instead of a stale cached one.
func (a *App) EndDrag() {
	a.dragMu.Lock()
	a.dragActive = false
	a.dragMu.Unlock()
}

// DragWindowTo moves the window to an absolute desktop coordinate.
// The frontend calls this per mousemove rather than the Wails runtime
// directly, since only Go can translate into the coordinate space each
// platform actually expects.
func (a *App) DragWindowTo(x, y int) {
	a.dragMu.Lock()
	active := a.dragActive
	resolved := a.dragOriginResolved
	originX, originY := a.dragOriginX, a.dragOriginY
	a.dragMu.Unlock()

	if !active {
		a.setAbsoluteWindowPosition(x, y)
		return
	}
	if !resolved {
		runtime.WindowSetPosition(a.ctx, x, y)
		return
	}
	runtime.WindowSetPosition(a.ctx, x-originX, y-originY)
}

// currentScreen returns the screen the window currently sits on (or the
// first one reported, if none is flagged current), and false if no
// screen info is available at all.
func (a *App) currentScreen() (runtime.Screen, bool) {
	screens, err := runtime.ScreenGetAll(a.ctx)
	if err != nil || len(screens) == 0 {
		return runtime.Screen{}, false
	}
	for _, s := range screens {
		if s.IsCurrent {
			return s, true
		}
	}
	return screens[0], true
}

// currentScreenHeight returns the logical height of the screen the window
// currently sits on, or 0 if it can't be determined.
func (a *App) currentScreenHeight() int {
	screen, ok := a.currentScreen()
	if !ok {
		return 0
	}
	return screen.Size.Height
}

// SnapWindowToEdges snaps the window flush against a nearby screen
// edge. Called by the frontend on mouseup, so it only ever fires on a
// real release, never mid-drag.
func (a *App) SnapWindowToEdges() {
	x, y := runtime.WindowGetPosition(a.ctx)
	a.snapToEdges(x, y)
}

// snapToEdges moves the window flush against any screen edge it's
// within snapThreshold pixels of.
func (a *App) snapToEdges(x, y int) {
	width, height := runtime.WindowGetSize(a.ctx)

	// Prefer the real bounds of the monitor the window is on. Falling
	// back to Wails' screen size assumes a desktop origin, so it's only
	// correct on a single monitor.
	left, top, right, bottom, ok := monitorBoundsAt(x+width/2, y+height/2)
	if !ok {
		screen, found := a.currentScreen()
		if !found {
			return
		}
		left, top = 0, 0
		right, bottom = screen.Size.Width, screen.Size.Height
	}

	snappedX, snappedY := x, y

	if x-left <= snapThreshold {
		snappedX = left
	} else if right-(x+width) <= snapThreshold {
		snappedX = right - width
	}

	if y-top <= snapThreshold {
		snappedY = top
	} else if bottom-(y+height) <= snapThreshold {
		snappedY = bottom - height
	}

	if snappedX != x || snappedY != y {
		a.setAbsoluteWindowPosition(snappedX, snappedY)
	}
}

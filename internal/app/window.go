package app

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	// snapThreshold is how close (in px) the window has to be to a screen
	// edge, once the drag is released, before it snaps flush against it.
	snapThreshold = 24

	// minVisibleWidth is how much of the window has to stay on-screen
	// horizontally. Some overhang is deliberate - tucking the window
	// half off the side is a reasonable thing to want - but enough has
	// to remain to grab it again.
	minVisibleWidth = 80
)

// clampToScreen keeps (x, y) somewhere the window can still be grabbed:
// at least minVisibleWidth on-screen horizontally, never above the top
// edge, and never so far down that the bar itself is off the bottom.
//
// It clamps against the monitor the window is currently over rather than
// the whole desktop, which is what lets it still be dragged between
// monitors: monitorBoundsAt resolves to the nearest monitor, so the
// limits hand off to the neighbouring screen as the window's centre
// crosses onto it, and the allowed ranges overlap either side of a
// shared edge instead of fencing the window in.
//
// The window is frameless, so the top edge matters more than it looks:
// the drag handle *is* the bar, and a bar pushed above the screen top
// can't be grabbed back.
func clampToScreen(x, y, width, height int) (int, int) {
	left, top, right, bottom, ok := monitorBoundsAt(x+width/2, y+height/2)
	if !ok {
		return x, y
	}

	if x > right-minVisibleWidth {
		x = right - minVisibleWidth
	}
	if x < left-(width-minVisibleWidth) {
		x = left - (width - minVisibleWidth)
	}

	if y > bottom-collapsedHeight {
		y = bottom - collapsedHeight
	}
	if y < top {
		y = top
	}

	return x, y
}

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
	// Cached so clampToScreen doesn't need a size query every frame.
	// Auto-fit could resize mid-drag if the song changes, leaving this
	// slightly stale for a frame or two - only enough to shift the
	// clamp by the width delta, which isn't worth a per-frame query.
	a.dragWidth, a.dragHeight = width, height
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
	width, height := a.dragWidth, a.dragHeight
	a.dragMu.Unlock()

	if !active {
		a.setAbsoluteWindowPosition(x, y)
		return
	}

	// Clamped per frame rather than only on release, so the window
	// stops at the boundary as you drag instead of snapping back
	// afterwards. The target is recomputed from the pointer each frame,
	// so dragging back inwards picks the window up again straight away.
	x, y = clampToScreen(x, y, width, height)

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

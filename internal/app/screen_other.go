//go:build !windows && !darwin && !linux

package app

// No native implementation outside Windows/macOS/Linux - callers
// assume the screen sits at the desktop origin, only true for one
// monitor.
func monitorBoundsAt(x, y int) (left, top, right, bottom int, ok bool) {
	return 0, 0, 0, 0, false
}

// Only Windows needs this; elsewhere WindowSetPosition already takes
// an absolute desktop coordinate.
func workAreaOriginAt(x, y int) (originX, originY int, ok bool) {
	return 0, 0, false
}

// Only Windows moves the window natively; see screen_windows.go.
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

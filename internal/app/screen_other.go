//go:build !windows && !darwin

package app

// No native implementation outside Windows/macOS - callers assume the
// screen sits at the desktop origin, only true for one monitor.
func monitorBoundsAt(x, y int) (left, top, right, bottom int, ok bool) {
	return 0, 0, 0, 0, false
}

// Only Windows needs this; elsewhere WindowSetPosition already takes
// an absolute desktop coordinate.
func workAreaOriginAt(x, y int) (originX, originY int, ok bool) {
	return 0, 0, false
}

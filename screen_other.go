//go:build !windows && !darwin

package main

// monitorBoundsAt has no native implementation outside Windows and
// macOS yet - callers fall back to assuming the current screen sits at
// the desktop origin, which is only correct on a single-monitor setup.
func monitorBoundsAt(x, y int) (left, top, right, bottom int, ok bool) {
	return 0, 0, 0, 0, false
}

// workAreaOriginAt has no native implementation outside Windows -
// WindowSetPosition on other platforms is assumed to take a true
// absolute desktop coordinate already, so no compensation is needed.
func workAreaOriginAt(x, y int) (originX, originY int, ok bool) {
	return 0, 0, false
}

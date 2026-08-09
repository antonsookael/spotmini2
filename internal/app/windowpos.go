package app

import (
	"encoding/json"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spotmini-gui/internal/logging"
	"spotmini-gui/internal/paths"
)

const windowPosFile = "window.json"

type windowPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// saveWindowPosition records the window position for the next launch.
// Called on shutdown, not per move - dragging is frame by frame, which
// would mean hundreds of writes per drag.
func (a *App) saveWindowPosition() {
	x, y := runtime.WindowGetPosition(a.ctx)

	data, err := json.MarshalIndent(windowPosition{X: x, Y: y}, "", "  ")
	if err != nil {
		return
	}
	path, err := paths.ConfigFile(windowPosFile)
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0644)
}

// restoreWindowPosition returns the window to its last spot, then
// re-snaps if it's near an edge - a saved position is only valid for
// the layout it was saved under, so a changed display can leave a
// previously flush window slightly off.
func (a *App) restoreWindowPosition() {
	path, err := paths.ConfigFile(windowPosFile)
	if err != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var pos windowPosition
	if err := json.Unmarshal(data, &pos); err != nil {
		return
	}

	// A saved position can point somewhere that no longer exists - a
	// monitor that's since been unplugged, or a rearranged desktop -
	// which would restore the window off-screen with no way to reach it.
	width, height := runtime.WindowGetSize(a.ctx)
	x, y := clampToScreen(pos.X, pos.Y, width, height)

	a.setAbsoluteWindowPosition(x, y)
	a.snapToEdges(x, y)
}

// ensureOnScreen re-runs the clamp once the window actually exists.
//
// restoreWindowPosition runs from OnStartup, which Wails starts before
// the window is ordered front - and on macOS the screen lookup resolves
// from the key or first visible window, so at that point there isn't one
// and clampToScreen has nothing to clamp against. It returns the saved
// coordinates untouched, which means the guard against "restored onto a
// monitor that's since been unplugged" silently did nothing on the
// platform where it matters most: the bar is frameless with no tray or
// dock entry, so a window restored off-screen can't be retrieved at all
// without deleting window.json by hand.
func (a *App) ensureOnScreen() {
	x, y := runtime.WindowGetPosition(a.ctx)
	width, height := runtime.WindowGetSize(a.ctx)

	clampedX, clampedY := clampToScreen(x, y, width, height)
	if clampedX == x && clampedY == y {
		return
	}

	logging.Printf("Window was off-screen at %d,%d - moved to %d,%d", x, y, clampedX, clampedY)
	a.setAbsoluteWindowPosition(clampedX, clampedY)
}

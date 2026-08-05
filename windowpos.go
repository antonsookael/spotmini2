package main

import (
	"encoding/json"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spotmini-gui/paths"
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

	a.setAbsoluteWindowPosition(pos.X, pos.Y)
	a.snapToEdges(pos.X, pos.Y)
}

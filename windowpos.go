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

// saveWindowPosition records where the window currently sits, so the
// next launch can put it back there. Called on shutdown rather than on
// every move - the frontend drives dragging frame by frame (see
// DragWindowTo), and writing a file on each of those would mean
// hundreds of writes per drag.
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

// restoreWindowPosition moves the window back to its last saved spot,
// then snaps it to the nearest screen edge if it's close to one -
// without that, a window saved flush against an edge can end up a few
// pixels off after restore, since a saved position is only valid for
// the screen layout it was saved under (an unplugged monitor or a
// changed resolution moves where that edge actually is).
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

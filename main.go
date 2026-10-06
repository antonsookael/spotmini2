package main

import (
	"embed"

	"spotmini-gui/internal/app"
	"spotmini-gui/internal/logging"
)

// The embed directive resolves relative to this file, so it has to live
// here at the module root rather than alongside the rest of the app
// package - hence passing the assets in rather than reading them there.
//
//go:embed all:frontend/dist
var assets embed.FS

// Linux has no executable icon resource for the desktop to read, so the
// window is handed its icon at runtime instead. A 256px copy of
// build/appicon.png rather than the original: at 1024px the icon is too
// big for the X11 property the taskbar reads it from, and GTK silently
// leaves it unset.
//
//go:embed build/linux/icon.png
var icon []byte

func main() {
	if err := app.Run(assets, icon); err != nil {
		// Via logging rather than println: a built app has no console
		// attached, so the one message explaining why the window never
		// appeared would otherwise go nowhere.
		logging.Printf("Fatal: %v", err)
	}
}

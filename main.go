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

func main() {
	if err := app.Run(assets); err != nil {
		// Via logging rather than println: a built app has no console
		// attached, so the one message explaining why the window never
		// appeared would otherwise go nowhere.
		logging.Printf("Fatal: %v", err)
	}
}

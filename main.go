package main

import (
	"embed"

	"spotmini-gui/internal/app"
)

// The embed directive resolves relative to this file, so it has to live
// here at the module root rather than alongside the rest of the app
// package - hence passing the assets in rather than reading them there.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := app.Run(assets); err != nil {
		println("Error:", err.Error())
	}
}

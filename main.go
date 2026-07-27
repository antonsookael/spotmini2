package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:       "spotmini-gui",
		Width:       320,
		Height:      50,
		MinWidth:    320,
		MinHeight:   50,
		MaxWidth:    320,
		MaxHeight:   50,
		Frameless:   true,
		AlwaysOnTop: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 18, G: 18, B: 18, A: 255},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			// No rounded corners / Aero shadow - sharp edges suit a
			// thin strip sitting flush in a screen corner.
			DisableFramelessWindowDecorations: true,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

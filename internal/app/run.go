package app

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// Run builds the App and starts the Wails event loop, blocking until
// the window closes.
//
// The Wails setup lives here rather than in main so startup/shutdown
// can stay unexported: Wails binds every exported method on App to the
// frontend, and lifecycle hooks have no business being callable from
// JavaScript.
func Run(assets embed.FS) error {
	a := New()

	return wails.Run(&options.App{
		Title:       "spotmini",
		Width:       windowWidth,
		Height:      collapsedHeight,
		MinWidth:    minWindowWidth,
		MinHeight:   collapsedHeight,
		Frameless:   true,
		AlwaysOnTop: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 18, G: 18, B: 18, A: 255},
		OnStartup:        a.startup,
		OnShutdown:       a.shutdown,
		Bind: []interface{}{
			a,
		},
		Windows: &windows.Options{
			DisableFramelessWindowDecorations: true,
		},
	})
}

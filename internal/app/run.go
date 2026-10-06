package app

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// windowTitle is also how screen_linux.go picks the main window out
// from anything else GTK has open, so the two can't drift apart.
const windowTitle = "spotmini"

// Run builds the App and starts the Wails event loop, blocking until
// the window closes.
//
// The Wails setup lives here rather than in main so startup/shutdown
// can stay unexported: Wails binds every exported method on App to the
// frontend, and lifecycle hooks have no business being callable from
// JavaScript.
func Run(assets embed.FS, icon []byte) error {
	a := New()

	return wails.Run(&options.App{
		Title:       windowTitle,
		Width:       windowWidth,
		Height:      collapsedHeight,
		MinWidth:    minWindowWidth,
		MinHeight:   collapsedHeight,
		Frameless:   true,
		AlwaysOnTop: true,
		// Frameless windows keep a resize border, so clicking near an
		// edge starts an OS resize drag. That drag captures the mouse,
		// so the webview never sees the mouseup and the JS drag handler
		// stays armed - leaving the window stuck following the cursor.
		// Nothing here wants manual resizing anyway: the width is
		// auto-fit's and the height is the panel's.
		DisableResize: true,
		StartHidden:   nativeStartHidden,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 18, G: 18, B: 18, A: 255},
		OnStartup:        a.startup,
		OnDomReady:       a.domReady,
		OnShutdown:       a.shutdown,
		Bind: []interface{}{
			a,
		},
		Windows: &windows.Options{
			DisableFramelessWindowDecorations: true,
		},
		Linux: &linux.Options{
			// What the taskbar and window switcher show. Windows and
			// macOS take theirs from the packaged build instead.
			Icon: icon,
			// Wails' own default when no Linux options are given at all,
			// its workaround for the webview coming up blank on some GPU
			// drivers (wailsapp/wails#2977). Spelled out because
			// supplying any options switches that default off.
			WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
		},
	})
}

package app

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ToggleSettingsPanel opens/closes the customize panel, resizing (and, if
// there isn't room to grow downward, repositioning) the window to fit it.
func (a *App) ToggleSettingsPanel() {
	a.togglePanel("settings")
}

// TogglePlaylistsPanel opens/closes the playlist-picker panel, using the
// same expand/collapse mechanism as ToggleSettingsPanel.
func (a *App) TogglePlaylistsPanel() {
	a.togglePanel("playlists")
}

// ToggleHotkeysPanel opens/closes the hotkey list. Deliberately absent
// from hotkeys.Actions: it's reached from a button in the settings
// panel, so a global hotkey for the list of global hotkeys would be one
// more binding to remember for no benefit.
func (a *App) ToggleHotkeysPanel() {
	a.togglePanel("hotkeys")
}

// ToggleAlwaysOnTop lets the frontend flip the setting - it owns the
// value (localStorage) and the checkbox, so flipping it here would
// desync both.
func (a *App) ToggleAlwaysOnTop() {
	runtime.EventsEmit(a.ctx, "toggle-always-on-top")
}

// togglePanel expands panel, or collapses if it's already the open one.
// Switching straight between panels leaves the window alone; the
// frontend re-measures and calls SetPanelHeight for the new one.
//
// Expanding uses defaultExpandedHeight only as a starting point - the
// frontend corrects it once it has measured the panel that's now
// visible.
func (a *App) togglePanel(panel string) {
	a.panelMu.Lock()
	defer a.panelMu.Unlock()

	wasExpanded := a.expandedPanel != ""

	if a.expandedPanel == panel {
		a.expandedPanel = ""
	} else {
		a.expandedPanel = panel
	}

	// Reuse the current width rather than hardcoding windowWidth, which
	// would stomp whatever auto-fit set (see updateAutoWidth in main.js).
	width, currentHeight := runtime.WindowGetSize(a.ctx)

	if a.expandedPanel != "" {
		if !wasExpanded {
			x, y := runtime.WindowGetPosition(a.ctx)
			screenHeight := a.currentScreenHeight()

			if screenHeight > 0 && y+defaultExpandedHeight > screenHeight {
				a.openedUpward = true
				a.setAbsoluteWindowPosition(x, y-(defaultExpandedHeight-collapsedHeight))
			} else {
				a.openedUpward = false
			}
			runtime.WindowSetSize(a.ctx, width, defaultExpandedHeight)
		}
		if panel == "playlists" {
			// The hotkey can fire while another app is focused, and
			// AlwaysOnTop only affects z-order - without this the
			// window shows but never takes keyboard focus, so the
			// search input's .focus() does nothing.
			runtime.WindowShow(a.ctx)
		}
	} else {
		// Measured rather than derived from a constant: the height is
		// whatever the last panel asked for, so assuming the default
		// would put the window back in the wrong place.
		delta := currentHeight - collapsedHeight

		runtime.WindowSetSize(a.ctx, width, collapsedHeight)
		if a.openedUpward {
			x, y := runtime.WindowGetPosition(a.ctx)
			a.setAbsoluteWindowPosition(x, y+delta)
			a.openedUpward = false
		}
	}
	runtime.EventsEmit(a.ctx, "panel-changed", a.expandedPanel)
}

// SetPanelHeight sizes the window to fit a panel needing contentHeight
// pixels, so panels size themselves instead of every added row needing
// a hardcoded height bumped to match.
//
// Clamped to the screen: a panel with an unbounded list (playlists)
// would otherwise ask for a window taller than the display.
func (a *App) SetPanelHeight(contentHeight int) {
	a.panelMu.Lock()
	defer a.panelMu.Unlock()

	// Nothing to size against once collapsed, and a late call must not
	// re-expand the window behind the user's back.
	if a.expandedPanel == "" {
		return
	}

	target := collapsedHeight + contentHeight
	if screenHeight := a.currentScreenHeight(); screenHeight > 0 && target > screenHeight {
		target = screenHeight
	}

	width, current := runtime.WindowGetSize(a.ctx)
	if current == target {
		return
	}

	// Opening upward pins the bottom edge, so a height change has to
	// move the top by the same amount or the window creeps down the
	// screen every time a panel resizes.
	if a.openedUpward {
		x, y := runtime.WindowGetPosition(a.ctx)
		a.setAbsoluteWindowPosition(x, y-(target-current))
	}
	runtime.WindowSetSize(a.ctx, width, target)
}

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

// ToggleAlwaysOnTop lets the frontend flip the setting - it owns the
// value (localStorage) and the checkbox, so flipping it here would
// desync both.
func (a *App) ToggleAlwaysOnTop() {
	runtime.EventsEmit(a.ctx, "toggle-always-on-top")
}

// togglePanel expands panel, or collapses if it's already the open
// one. Switching straight between panels leaves the window size and
// position alone, since both use expandedHeight.
func (a *App) togglePanel(panel string) {
	a.panelMu.Lock()
	defer a.panelMu.Unlock()

	wasExpanded := a.expandedPanel != ""

	if a.expandedPanel == panel {
		a.expandedPanel = ""
	} else {
		a.expandedPanel = panel
	}

	delta := expandedHeight - collapsedHeight
	// Reuse the current width rather than hardcoding windowWidth, which
	// would stomp whatever auto-fit set (see updateAutoWidth in main.js).
	width, _ := runtime.WindowGetSize(a.ctx)

	if a.expandedPanel != "" {
		if !wasExpanded {
			x, y := runtime.WindowGetPosition(a.ctx)
			screenHeight := a.currentScreenHeight()

			if screenHeight > 0 && y+expandedHeight > screenHeight {
				a.openedUpward = true
				a.setAbsoluteWindowPosition(x, y-delta)
			} else {
				a.openedUpward = false
			}
			runtime.WindowSetSize(a.ctx, width, expandedHeight)
		}
		if panel == "playlists" {
			// The hotkey can fire while another app is focused, and
			// AlwaysOnTop only affects z-order - without this the
			// window shows but never takes keyboard focus, so the
			// search input's .focus() does nothing.
			runtime.WindowShow(a.ctx)
		}
	} else {
		runtime.WindowSetSize(a.ctx, width, collapsedHeight)
		if a.openedUpward {
			x, y := runtime.WindowGetPosition(a.ctx)
			a.setAbsoluteWindowPosition(x, y+delta)
			a.openedUpward = false
		}
	}
	runtime.EventsEmit(a.ctx, "panel-changed", a.expandedPanel)
}

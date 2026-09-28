package app

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ToggleSettingsPanel opens/closes the customize panel, resizing (and, if
// there isn't room to grow downward, repositioning) the window to fit it.
//
// It's what the settings hotkey and gear do, so while favorites mode is
// on it opens the mode's own panel instead: the mode's controls are what
// there is to adjust then, and the regular settings are a back arrow
// away from there.
func (a *App) ToggleSettingsPanel() {
	a.panelMu.Lock()
	open := a.expandedPanel
	a.panelMu.Unlock()

	a.togglePanel(settingsToggleTarget(open, a.stats.Mode().Active))
}

// settingsToggleTarget is the panel ToggleSettingsPanel toggles, given
// the one open now: toggling the open one is what closes it.
func settingsToggleTarget(open string, favoritesOn bool) string {
	switch open {
	case "", "playlists":
		if favoritesOn {
			return "favorites"
		}
		return "settings"
	}
	// Settings or a page reached from it. The hotkey opens and closes the
	// whole thing - stepping back to the first page instead left it
	// taking two presses to put away.
	return open
}

// ShowSettingsPanel opens the regular settings, for the back arrows that
// lead there. Unlike ToggleSettingsPanel it never redirects to favorites
// and never closes the window.
func (a *App) ShowSettingsPanel() {
	a.panelMu.Lock()
	open := a.expandedPanel
	a.panelMu.Unlock()

	if open != "settings" {
		a.togglePanel("settings")
	}
}

// TogglePlaylistsPanel opens/closes the playlist-picker panel, using the
// same expand/collapse mechanism as ToggleSettingsPanel.
//
// Does nothing in favorites mode, which has no playlists: picking one
// would only end the mode.
func (a *App) TogglePlaylistsPanel() {
	if a.stats.Mode().Active {
		return
	}
	a.togglePanel("playlists")
}

// ToggleHotkeysPanel opens/closes the hotkey list. Deliberately absent
// from hotkeys.Actions: it's reached from a button in the settings
// panel, so a global hotkey for the list of global hotkeys would be one
// more binding to remember for no benefit.
func (a *App) ToggleHotkeysPanel() {
	a.togglePanel("hotkeys")
}

// ToggleFavoritesPanel opens/closes favorites mode's panel.
func (a *App) ToggleFavoritesPanel() {
	a.togglePanel("favorites")
}

// ToggleStatsPanel opens/closes the listening stats, reached from
// settings.
func (a *App) ToggleStatsPanel() {
	a.togglePanel("stats")
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
			_, bottom, ok := a.currentScreenBounds()

			if ok && y+defaultExpandedHeight > bottom {
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
	// Clamped against the monitor's height, not its bottom edge - target
	// is a window size, and those are only the same number on a display
	// whose top edge sits at 0.
	if top, bottom, ok := a.currentScreenBounds(); ok && target > bottom-top {
		target = bottom - top
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

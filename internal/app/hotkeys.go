package app

import (
	"fmt"

	"golang.design/x/hotkey"

	"spotmini-gui/internal/hotkeys"
)

// applyHotkey registers binding for action, replacing any previous one.
// The old hotkey is only torn down once the new one registers, so a
// rejected binding doesn't leave the action dead.
func (a *App) applyHotkey(action string, binding hotkeys.HotkeyBinding) error {
	hk, err := hotkeys.BindingToHotkey(binding)
	if err != nil {
		return err
	}
	if err := hk.Register(); err != nil {
		return err
	}

	a.hotkeyMu.Lock()
	old := a.activeHotkeys[action]
	a.activeHotkeys[action] = hk
	a.hotkeyMu.Unlock()

	if old != nil {
		old.Unregister()
	}

	fmt.Printf("Listening for the %s hotkey...\n", action)
	go a.watchHotkey(action, hk)
	return nil
}

// watchHotkey runs until hk is unregistered (Unregister closes the channel
// Keydown reads from, so this loop exits cleanly on its own).
func (a *App) watchHotkey(action string, hk *hotkey.Hotkey) {
	fn := a.hotkeyAction(action)
	for range hk.Keydown() {
		fn()
	}
}

func (a *App) hotkeyAction(action string) func() {
	switch action {
	case "settings":
		return a.ToggleSettingsPanel
	case "playlists":
		return a.TogglePlaylistsPanel
	case "playPause":
		return a.PlayPause
	case "next":
		return a.NextTrack
	case "previous":
		return a.PreviousTrack
	case "shuffle":
		return a.ToggleShuffle
	case "loop":
		return a.ToggleLoop
	case "volumeUp":
		return a.VolumeUp
	case "volumeDown":
		return a.VolumeDown
	case "alwaysOnTop":
		return a.ToggleAlwaysOnTop
	}
	return func() {}
}

// GetHotkeyConfig returns the currently active hotkey bindings.
func (a *App) GetHotkeyConfig() hotkeys.HotkeyConfig {
	a.hotkeyMu.Lock()
	defer a.hotkeyMu.Unlock()
	return a.hotkeyConfig
}

// SetHotkeyBinding rebinds action, persists it, and applies it now.
// The previous binding stays active if the new one won't register.
func (a *App) SetHotkeyBinding(action string, mods []string, key string) error {
	a.hotkeyMu.Lock()
	if _, ok := a.hotkeyConfig.Binding(action); !ok {
		a.hotkeyMu.Unlock()
		return fmt.Errorf("unknown hotkey action %q", action)
	}
	a.hotkeyMu.Unlock()

	binding := hotkeys.HotkeyBinding{Mods: mods, Key: key}
	if err := a.applyHotkey(action, binding); err != nil {
		return err
	}

	a.hotkeyMu.Lock()
	a.hotkeyConfig.SetBinding(action, binding)
	cfg := a.hotkeyConfig
	a.hotkeyMu.Unlock()

	return hotkeys.SaveConfig(cfg)
}

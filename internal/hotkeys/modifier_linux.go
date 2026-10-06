//go:build linux

package hotkeys

import "golang.design/x/hotkey"

// X11 has no named Alt or Super modifier, only the numbered Mod1-Mod5
// slots - on every stock keymap Alt is Mod1 and Super (the Windows key)
// is Mod4.
func modifierFromString(s string) (hotkey.Modifier, bool) {
	switch s {
	case "ctrl":
		return hotkey.ModCtrl, true
	case "alt":
		return hotkey.Mod1, true
	case "shift":
		return hotkey.ModShift, true
	case "cmd":
		return hotkey.Mod4, true
	}
	return 0, false
}

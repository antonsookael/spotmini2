//go:build darwin

package hotkeys

import "golang.design/x/hotkey"

func modifierFromString(s string) (hotkey.Modifier, bool) {
	switch s {
	case "ctrl":
		return hotkey.ModCtrl, true
	case "alt":
		return hotkey.ModOption, true
	case "shift":
		return hotkey.ModShift, true
	case "cmd":
		return hotkey.ModCmd, true
	}
	return 0, false
}

//go:build !darwin

package main

import "golang.design/x/hotkey"

func modifierFromString(s string) (hotkey.Modifier, bool) {
	switch s {
	case "ctrl":
		return hotkey.ModCtrl, true
	case "alt":
		return hotkey.ModAlt, true
	case "shift":
		return hotkey.ModShift, true
	case "cmd":
		return hotkey.ModWin, true
	}
	return 0, false
}

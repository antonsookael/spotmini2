//go:build !darwin

package main

import "golang.design/x/hotkey"

func settingsHotkeyMods() []hotkey.Modifier {
	return []hotkey.Modifier{hotkey.ModCtrl, hotkey.ModAlt}
}

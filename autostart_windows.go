//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// autostartValueName is the value name under the Run key - Windows
// launches whatever it points to, quoted, at login.
const autostartValueName = "spotmini-gui"

const autostartRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func isAutostartEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	_, _, err = key.GetStringValue(autostartValueName)
	return err == nil
}

func setAutostart(enabled bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if !enabled {
		if err := key.DeleteValue(autostartValueName); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return key.SetStringValue(autostartValueName, `"`+exe+`"`)
}

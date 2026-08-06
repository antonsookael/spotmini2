//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const autostartLabel = "com.spotmini-gui.autostart"

func autostartPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", autostartLabel+".plist"), nil
}

func isAutostartEnabled() bool {
	path, err := autostartPlistPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// setAutostart writes (or removes) a LaunchAgent plist with
// RunAtLoad - launchd starts it directly at login, no AppleScript
// "Login Items" automation needed.
func setAutostart(enabled bool) error {
	path, err := autostartPlistPath()
	if err != nil {
		return err
	}

	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`, autostartLabel, exe)

	return os.WriteFile(path, []byte(plist), 0644)
}

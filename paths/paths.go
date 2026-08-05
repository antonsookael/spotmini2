// Package paths resolves a stable per-user location for the token,
// hotkey config and log. A relative filename would resolve against the
// working directory - fine under `wails dev`, but unpredictable for a
// double-clicked bundle, which silently lost the token every launch.
package paths

import (
	"os"
	"path/filepath"
)

const appDirName = "spotmini-gui"

// ConfigFile returns the path to name inside the per-user config dir
// (~/Library/Application Support/spotmini-gui, %AppData% on Windows),
// creating it if needed.
func ConfigFile(name string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, appDirName)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(appDir, name), nil
}

// Package paths resolves a stable, per-user location for the app's
// persisted files (auth token, hotkey config, log). A bare relative
// filename resolves against the process's current working directory,
// which is the project root under `wails dev` but is unpredictable and
// unrelated to the app bundle's own location for a double-clicked
// packaged build - silently losing the saved token/config on every
// launch.
package paths

import (
	"os"
	"path/filepath"
)

const appDirName = "spotmini-gui"

// ConfigFile returns the full path to name inside the app's per-user
// config directory (e.g. ~/Library/Application Support/spotmini-gui on
// macOS, %AppData%/spotmini-gui on Windows), creating that directory
// first if it doesn't already exist.
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

//go:build linux

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const autostartDesktopFile = "spotmini-gui.desktop"

// autostartDesktopPath is the XDG autostart entry - every desktop
// environment that follows the spec launches what's in this folder at
// login. os.UserConfigDir honours $XDG_CONFIG_HOME, as the spec asks.
func autostartDesktopPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", autostartDesktopFile), nil
}

func isAutostartEnabled() bool {
	path, err := autostartDesktopPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// desktopExecQuote quotes a path for a .desktop Exec line.
//
// Two layers of escaping apply, and the spec is explicit that both do.
// The argument is wrapped in double quotes, with ", `, $ and \ each
// escaped by a backslash. Then, because Exec is a string value like any
// other in the file, every backslash that produced is doubled again -
// skipping that leaves sequences like \" which a strict reader (the
// systemd generator most desktops now autostart through) rejects
// outright, dropping the entry. Last, % is doubled so it isn't taken for
// a field code.
func desktopExecQuote(path string) string {
	quoted := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`).Replace(path)
	quoted = strings.ReplaceAll(quoted, `\`, `\\`)
	return `"` + strings.ReplaceAll(quoted, "%", "%%") + `"`
}

func setAutostart(enabled bool) error {
	path, err := autostartDesktopPath()
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

	entry := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=spotmini
Exec=%s
Terminal=false
X-GNOME-Autostart-enabled=true
`, desktopExecQuote(exe))

	return os.WriteFile(path, []byte(entry), 0644)
}

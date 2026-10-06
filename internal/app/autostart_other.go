//go:build !windows && !darwin && !linux

package app

// Autostart has no implementation outside Windows, macOS and Linux
// yet - the same platforms the release workflow actually builds for.
func isAutostartEnabled() bool {
	return false
}

func setAutostart(enabled bool) error {
	return nil
}

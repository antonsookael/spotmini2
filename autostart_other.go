//go:build !windows && !darwin

package main

// Autostart has no implementation outside Windows and macOS yet - the
// same two platforms the release workflow actually builds for.
func isAutostartEnabled() bool {
	return false
}

func setAutostart(enabled bool) error {
	return nil
}

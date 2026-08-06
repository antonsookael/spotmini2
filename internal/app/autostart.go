package app

// The per-platform implementations behind these live in
// autostart_windows.go / autostart_darwin.go / autostart_other.go.

// IsAutostartEnabled reports whether spotmini is currently set to
// launch automatically at login. Queries the OS directly (registry key
// on Windows, LaunchAgent file on macOS) rather than a cached setting,
// since it can be changed outside the app (Task Manager's Startup tab,
// deleting the LaunchAgent by hand, etc.).
func (a *App) IsAutostartEnabled() bool {
	return isAutostartEnabled()
}

// SetAutostart enables or disables launching spotmini automatically at
// login.
func (a *App) SetAutostart(enabled bool) error {
	return setAutostart(enabled)
}

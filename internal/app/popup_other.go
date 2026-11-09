//go:build !linux

package app

// Windows and macOS keep the window as the now-playing strip; see
// popup.go.
const popupMode = false

func (a *App) startDesktopThemeWatch()       {}
func (a *App) togglePopupPanel(panel string) {}

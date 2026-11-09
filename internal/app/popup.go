package app

// Popup mode is how the Linux build shows its panels: the window isn't
// a now-playing strip there (the desktop draws that from the control
// socket, see ipc_linux.go), so it stays hidden and only appears while
// a panel is open, placed under the top bar like the desktop's own
// popups. popupMode is a constant per platform; Windows and macOS keep
// the strip. The platform halves are in popup_linux.go / popup_other.go.

// Where the popup goes when the desktop hasn't said: just under a
// top bar, right-aligned, in the desktop's own logical pixels.
const (
	popupDefaultTop   = 56
	popupDefaultRight = 18
	popupDefaultWidth = 360
)

// DesktopTheme is what the desktop tells the popup about its looks - the
// rice writes it to ~/.local/state/rice/spotmini.json on every preset
// change. Available is false when there is no such file, and the
// frontend then keeps its own colours.
type DesktopTheme struct {
	Available bool   `json:"available"`
	Preset    string `json:"preset"`
	Base      string `json:"base"`
	Surface   string `json:"surface"`
	Accent    string `json:"accent"`
	Accent3   string `json:"accent3"`
	Text      string `json:"text"`
	Subtext   string `json:"subtext"`
	Urgent    string `json:"urgent"`
	// Solid is a preset without see-through apps.
	Solid bool `json:"solid"`
	// Popup is where to open: offsets from the monitor's top and right
	// edges and the width, in the desktop's logical pixels.
	Popup struct {
		Top   int `json:"top"`
		Right int `json:"right"`
		Width int `json:"width"`
	} `json:"popup"`
}

// GetDesktopTheme is the frontend's way to the theme; changes after
// that arrive as "desktop-theme-changed" events.
func (a *App) GetDesktopTheme() DesktopTheme {
	return a.desktopThemeNow()
}

func (a *App) desktopThemeNow() DesktopTheme {
	a.desktopThemeMu.Lock()
	defer a.desktopThemeMu.Unlock()
	return a.desktopTheme
}

func (a *App) setDesktopTheme(t DesktopTheme) {
	a.desktopThemeMu.Lock()
	a.desktopTheme = t
	a.desktopThemeMu.Unlock()
}

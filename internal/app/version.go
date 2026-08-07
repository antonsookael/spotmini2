package app

// Version is stamped in at build time by the release workflow, via
//
//	-ldflags "-X spotmini-gui/internal/app.Version=v1.2.3"
//
// so it can't drift from the tag the way a hardcoded constant would.
// Local builds keep the default, which is what makes a dev build
// obvious in the settings panel.
var Version = "dev"

// GetVersion returns the running build's version, for the settings
// panel to display.
func (a *App) GetVersion() string {
	return Version
}

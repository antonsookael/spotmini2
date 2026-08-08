package backend

import "spotmini-gui/internal/logging"

// openBrowser launches the system's default browser to the given URL.
// The per-platform implementations behind it live in
// browser_windows.go / browser_other.go.
//
// Failing to open is worth logging rather than printing: it happens
// during login, before there's a window to show anything in, and a
// built app has no console for the fallback "open this manually" line
// to appear on - so without the log there is nothing at all to go on.
func openBrowser(url string) {
	if err := openURL(url); err != nil {
		logging.Printf("Couldn't open the browser automatically (%v) - open this URL manually: %s", err, url)
	}
}

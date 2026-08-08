//go:build windows

package backend

import "golang.org/x/sys/windows"

// openURL hands the URL straight to the shell's default handler.
//
// Deliberately not "cmd /c start", which is what this used to be. cmd
// reads & as a command separator, and Go only wraps an argument in
// quotes when it contains a space - so a correctly encoded auth URL
// (no spaces, an & before every parameter) reached cmd unquoted and was
// cut off at the first one. Spotify received a request carrying nothing
// but client_id and answered "response_type must be code".
//
// It went unnoticed for a while because the URL used to be built by
// string interpolation and carried raw spaces from the scope list: that
// malformed URL got quoted, which hid the & problem entirely. Encoding
// the URL properly is what exposed it.
//
// ShellExecute takes the URL as one value with no command line for
// anything to re-parse, so there is no shell left to misread it. It's
// also what Wails' own BrowserOpenURL ends up calling, via pkg/browser.
func openURL(url string) error {
	return windows.ShellExecute(0, nil, windows.StringToUTF16Ptr(url), nil, nil, windows.SW_SHOWNORMAL)
}

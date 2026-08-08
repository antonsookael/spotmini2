package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spotmini-gui/internal/logging"
)

// latestReleaseURL is the same endpoint the download page reads, so the
// app and the site can never disagree about what the newest build is.
const latestReleaseURL = "https://api.github.com/repos/antonsookael/spotmini2/releases/latest"

// releasePageURL is where the notice sends people. Deliberately the
// release page rather than the asset itself: these builds are only
// self-signed, so the download needs the Gatekeeper instructions that
// live alongside it, not a file landing silently in Downloads.
const releasePageURL = "https://github.com/antonsookael/spotmini2/releases/latest"

// updateCheckTimeout bounds the whole check. It runs at startup and
// nothing waits on it, but a hung connection would otherwise park a
// goroutine for as long as the app is open.
const updateCheckTimeout = 10 * time.Second

type latestRelease struct {
	TagName string `json:"tag_name"`
}

// UpdateInfo is what the frontend gets told when a newer build exists.
type UpdateInfo struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// checkForUpdate emits "update-available" if GitHub's latest release is
// newer than the running build. Silent otherwise - there's nothing worth
// saying about being up to date, and nothing the user can do about a
// check that couldn't reach GitHub.
func (a *App) checkForUpdate() {
	// A local build has no tag to compare against and is usually ahead
	// of the last release anyway, so the check would only ever nag.
	if Version == "dev" {
		return
	}

	latest, err := fetchLatestVersion()
	if err != nil {
		logging.Printf("Update check failed: %v", err)
		return
	}

	newer, err := isNewer(latest, Version)
	if err != nil {
		logging.Printf("Update check: %v", err)
		return
	}
	if !newer {
		return
	}

	info := UpdateInfo{Version: latest, URL: releasePageURL}

	// Stored before it's emitted, so a frontend that asks the moment it
	// loads can't catch this half-done.
	a.updateMu.Lock()
	a.updateInfo = &info
	a.updateMu.Unlock()

	fmt.Printf("[update] %s is available (running %s)\n", latest, Version)
	runtime.EventsEmit(a.ctx, "update-available", info)
}

// GetUpdateInfo returns the newer release found at startup, or nil if
// there is none, the check hasn't finished, or it failed.
//
// The frontend asks for this as well as listening for
// "update-available", because the event alone isn't reliable: the check
// is kicked off from startup, before the webview has loaded, and Wails
// delivers events by evaluating script against the page as it is right
// then rather than queuing them. One that fires early reaches no
// listeners and is simply gone. Between the two paths, whichever
// arrives second finds the value already set and changes nothing.
func (a *App) GetUpdateInfo() *UpdateInfo {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	return a.updateInfo
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: updateCheckTimeout}
	resp, err := client.Get(latestReleaseURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Worth distinguishing in the log: 403 here is almost always
		// GitHub's unauthenticated rate limit rather than a real fault.
		return "", fmt.Errorf("github returned status %d", resp.StatusCode)
	}

	var release latestRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	if release.TagName == "" {
		return "", fmt.Errorf("release has no tag name")
	}
	return release.TagName, nil
}

// isNewer compares two "v1.2.3" tags numerically, part by part.
//
// Not a string comparison, which is the trap this whole function exists
// to avoid: "v1.10" sorts *before* "v1.9" lexically, so a check written
// the obvious way would have gone quiet for good the moment the tags
// crossed ten - which they already have.
//
// A missing part counts as zero, so v1.10 and v1.10.0 compare equal.
func isNewer(candidate, current string) (bool, error) {
	c, err := parseVersion(candidate)
	if err != nil {
		return false, fmt.Errorf("parsing latest release %q: %w", candidate, err)
	}
	running, err := parseVersion(current)
	if err != nil {
		return false, fmt.Errorf("parsing running version %q: %w", current, err)
	}

	for i := 0; i < len(c) || i < len(running); i++ {
		a, b := at(c, i), at(running, i)
		if a != b {
			return a > b, nil
		}
	}
	return false, nil
}

func at(parts []int, i int) int {
	if i < len(parts) {
		return parts[i]
	}
	return 0
}

func parseVersion(tag string) ([]int, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if trimmed == "" {
		return nil, fmt.Errorf("empty version")
	}

	fields := strings.Split(trimmed, ".")
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", field)
		}
		parts = append(parts, n)
	}
	return parts, nil
}

// OpenReleasePage opens the latest release in the browser, for the
// update notice in the settings panel to call.
func (a *App) OpenReleasePage() {
	runtime.BrowserOpenURL(a.ctx, releasePageURL)
}

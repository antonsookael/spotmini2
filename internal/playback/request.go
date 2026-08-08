package playback

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"spotmini-gui/internal/logging"
)

// apiPrefix is trimmed off logged URLs - it's on every one of them, and
// dropping it keeps the interesting part (the endpoint and its query,
// which is what says what the command actually did) at the front.
const apiPrefix = "https://api.spotify.com/v1"

// ErrNoActiveDevice is returned when Spotify rejects a player command
// because there's no active device - typically because one sat idle
// long enough for Spotify to drop it.
var ErrNoActiveDevice = errors.New("no active device")

// ErrPremiumRequired is returned when Spotify rejects a player command
// because the account isn't Premium - the playback control endpoints
// (play/pause/skip/volume/shuffle/repeat/transfer) all reject Free-tier
// accounts outright, regardless of device state.
var ErrPremiumRequired = errors.New("spotify premium required")

// doPlayerRequest issues a request against the Spotify player API and
// translates a "no active device" response into ErrNoActiveDevice, so
// callers can tell that failure apart from any other kind and react to
// it (reviving a device and retrying) instead of just logging it.
func doPlayerRequest(method, url, accessToken string, body io.Reader) error {
	// The endpoint is what identifies the command - a bare status and
	// body gives no clue whether it was a skip, a volume change or a
	// device transfer that produced it.
	action := method + " " + strings.TrimPrefix(url, apiPrefix)

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		// Never reached Spotify at all - worth recording, since from the
		// user's side it looks identical to a command being ignored.
		logging.Printf("[spotify] %s -> %v", action, err)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	line := fmt.Sprintf("[spotify] %s -> %d", action, resp.StatusCode)
	if len(respBody) > 0 {
		line += " " + string(respBody)
	}

	// Successes stay on stdout only. They're the routine case and would
	// bulk up a log file that's meant to be read after something went
	// wrong - but they're still worth seeing live under `wails dev`.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Println(line)
		return nil
	}

	// %s rather than passing line as the format: a response body can
	// contain a stray % that would otherwise be read as a verb.
	logging.Printf("%s", line)

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		var parsed struct {
			Error struct {
				Reason string `json:"reason"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &parsed) == nil {
			switch parsed.Error.Reason {
			case "NO_ACTIVE_DEVICE":
				return ErrNoActiveDevice
			case "PREMIUM_REQUIRED":
				return ErrPremiumRequired
			}
		}
	}

	// Carries the endpoint so callers that only surface the error - like
	// the revival retry loop - still say which command it was.
	return fmt.Errorf("%s: spotify returned status %d: %s", action, resp.StatusCode, string(respBody))
}

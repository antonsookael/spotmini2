package playback

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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

// took formats how long a call has been running, rounded to the
// millisecond - sub-millisecond precision is noise next to a network
// round trip, and the full float makes the log lines hard to scan.
func took(start time.Time) time.Duration {
	return time.Since(start).Round(time.Millisecond)
}

// reasonError turns a rejection carrying one of Spotify's player
// "reason" codes into the matching sentinel, or nil when it isn't one
// the app can do anything about.
//
// Shared by reads and writes on purpose. It used to be inline in
// doPlayerRequest, which meant only writes could ever produce these -
// a read refused for the same cause came back as a generic error, so
// the callers that check for ErrPremiumRequired could never see one
// and reported "request failed, check the log" for an account that
// simply isn't Premium.
func reasonError(status int, body []byte) error {
	if status != http.StatusNotFound && status != http.StatusForbidden {
		return nil
	}

	var parsed struct {
		Error struct {
			Reason string `json:"reason"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return nil
	}

	switch parsed.Error.Reason {
	case "NO_ACTIVE_DEVICE":
		return ErrNoActiveDevice
	case "PREMIUM_REQUIRED":
		return ErrPremiumRequired
	}
	return nil
}

// doPlayerGet issues a read against the player API and returns the raw
// response body, or a nil body for the 204 Spotify answers with when
// nothing is active at all.
//
// The status check is the whole point of routing every read through
// here. Spotify's error responses are valid JSON, just not the shape
// the caller is unmarshaling into, so one of those lands as a
// zero-valued struct with no error at all - a 429 read as "not
// playing", or as "volume 0". Every read endpoint has to tell those
// apart, and none of them can do it by looking at the body alone.
func doPlayerGet(url, accessToken string) ([]byte, error) {
	action := "GET " + strings.TrimPrefix(url, apiPrefix)
	start := time.Now()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logging.Printf("[spotify] %s -> %v in %s", action, err, took(start))
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// %s rather than as the format string: a response body can
		// contain a stray % that would otherwise be read as a verb.
		logging.Printf("%s", fmt.Sprintf("[spotify] %s -> %d %s in %s", action, resp.StatusCode, string(body), took(start)))

		// Same classification writes get: a read refused because the
		// account isn't Premium has to reach the caller as
		// ErrPremiumRequired, or it can only be reported as an
		// unexplained failure.
		if reason := reasonError(resp.StatusCode, body); reason != nil {
			return nil, reason
		}
		return nil, fmt.Errorf("%s: spotify returned status %d: %s", action, resp.StatusCode, string(body))
	}

	fmt.Printf("[spotify] %s -> %d in %s\n", action, resp.StatusCode, took(start))

	// Distinguished from a body the caller can parse, rather than
	// handed back as an empty slice - "nothing is active" means
	// something different to each caller, so each one decides.
	if len(body) == 0 {
		return nil, nil
	}
	return body, nil
}

// doPlayerRequest issues a request against the Spotify player API and
// translates a "no active device" response into ErrNoActiveDevice, so
// callers can tell that failure apart from any other kind and react to
// it (reviving a device and retrying) instead of just logging it.
func doPlayerRequest(method, url, accessToken string, body io.Reader) error {
	// The endpoint is what identifies the command - a bare status and
	// body gives no clue whether it was a skip, a volume change or a
	// device transfer that produced it.
	action := method + " " + strings.TrimPrefix(url, apiPrefix)
	start := time.Now()

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
		logging.Printf("[spotify] %s -> %v in %s", action, err, took(start))
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	line := fmt.Sprintf("[spotify] %s -> %d", action, resp.StatusCode)
	if len(respBody) > 0 {
		line += " " + string(respBody)
	}
	// Appended last so the duration reads the same way on every line,
	// whether or not Spotify sent a body back.
	line += " in " + took(start).String()

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

	if reason := reasonError(resp.StatusCode, respBody); reason != nil {
		return reason
	}

	// Carries the endpoint so callers that only surface the error - like
	// the revival retry loop - still say which command it was.
	return fmt.Errorf("%s: spotify returned status %d: %s", action, resp.StatusCode, string(respBody))
}

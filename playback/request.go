package playback

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

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
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Println("Status:", resp.StatusCode)
	fmt.Println("Response body:", string(respBody))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

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

	return fmt.Errorf("spotify returned status %d: %s", resp.StatusCode, string(respBody))
}

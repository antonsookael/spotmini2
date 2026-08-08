package playback

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type Device struct {
	ID           string `json:"id"`
	IsActive     bool   `json:"is_active"`
	IsRestricted bool   `json:"is_restricted"`
	Name         string `json:"name"`
	Type         string `json:"type"`
}

type devicesResponse struct {
	Devices []Device `json:"devices"`
}

// GetDevices lists every device Spotify currently knows about for this
// account, including ones that have dropped off as "active" after being
// idle. Unlike /me/player, this endpoint always returns data (never an
// empty 204 body) as long as at least one device has connected recently.
func GetDevices(accessToken string) ([]Device, error) {
	// Routed through doPlayerGet for its status check: an error
	// response unmarshals into an empty Devices list, which reads as
	// "Spotify isn't open anywhere" - a wrong and confusing thing to
	// tell the user when the real problem was a 429 or an expired token.
	body, err := doPlayerGet("https://api.spotify.com/v1/me/player/devices", accessToken)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}

	var parsed devicesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing devices response: %w (raw: %s)", err, string(body))
	}

	return parsed.Devices, nil
}

// TransferPlayback moves playback to deviceID, which is how a device that
// Spotify dropped for being idle gets reactivated - there's no separate
// "reactivate" call, transferring to it is what brings it back.
func TransferPlayback(accessToken, deviceID string, play bool) error {
	payload, err := json.Marshal(map[string]interface{}{
		"device_ids": []string{deviceID},
		"play":       play,
	})
	if err != nil {
		return err
	}

	return doPlayerRequest("PUT", "https://api.spotify.com/v1/me/player", accessToken, bytes.NewReader(payload))
}

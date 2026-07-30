package playback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/player/devices", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
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

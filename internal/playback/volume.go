package playback

import (
	"encoding/json"
	"fmt"
)

type volumeResponse struct {
	Device struct {
		VolumePercent int `json:"volume_percent"`
	} `json:"device"`
}

// GetVolume reads the active device's volume (0-100), or
// ErrNoActiveDevice if there's no active session to read one from.
//
// That case is an error rather than a plain 0 because the caller
// adjusts *relative* to what comes back: reporting an unknown volume as
// 0 turned "raise the volume" into "set the volume to one step", which
// slammed a device sitting at 70% down to 10% as soon as it was
// revived.
func GetVolume(accessToken string) (int, error) {
	body, err := doPlayerGet("https://api.spotify.com/v1/me/player", accessToken)
	if err != nil {
		return 0, err
	}
	if body == nil {
		return 0, ErrNoActiveDevice
	}

	var state volumeResponse
	if err := json.Unmarshal(body, &state); err != nil {
		return 0, err
	}

	return state.Device.VolumePercent, nil
}

// SetVolume sets the active device's volume to percent, clamped to
// 0-100 - Spotify's API 400s on an out-of-range value. Returns the
// clamped value actually sent, so a caller that passed in an
// unclamped target (e.g. current+delta) can report what really got
// applied.
func SetVolume(accessToken string, percent int) (int, error) {
	if percent < 0 {
		percent = 0
	} else if percent > 100 {
		percent = 100
	}
	url := fmt.Sprintf("https://api.spotify.com/v1/me/player/volume?volume_percent=%d", percent)
	return percent, doPlayerRequest("PUT", url, accessToken, nil)
}

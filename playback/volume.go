package playback

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type volumeResponse struct {
	Device struct {
		VolumePercent int `json:"volume_percent"`
	} `json:"device"`
}

// GetVolume reads the active device's current volume (0-100).
func GetVolume(accessToken string) (int, error) {
	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/player", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// Same edge case as IsPlaying/IsShuffled - empty body means no
	// active session, so there's no device volume to report.
	if len(body) == 0 {
		return 0, nil
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

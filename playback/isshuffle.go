package playback

import (
	"encoding/json"
	"io"
	"net/http"
)

type ShuffleStateResponse struct {
	ShuffleState bool `json:"shuffle_state"`
}

func IsShuffled(accessToken string) (bool, error) {
	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/player", nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	// Same edge case as isPlaying - empty body means no active session.
	if len(body) == 0 {
		return false, nil
	}

	var state ShuffleStateResponse
	if err := json.Unmarshal(body, &state); err != nil {
		return false, err
	}

	return state.ShuffleState, nil
}

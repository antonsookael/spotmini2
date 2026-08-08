package playback

import (
	"encoding/json"
)

type ShuffleStateResponse struct {
	ShuffleState bool `json:"shuffle_state"`
}

func IsShuffled(accessToken string) (bool, error) {
	body, err := doPlayerGet("https://api.spotify.com/v1/me/player", accessToken)
	if err != nil {
		return false, err
	}

	// Same edge case as IsPlaying - no body means no active session.
	if body == nil {
		return false, nil
	}

	var state ShuffleStateResponse
	if err := json.Unmarshal(body, &state); err != nil {
		return false, err
	}

	return state.ShuffleState, nil
}

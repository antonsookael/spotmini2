package playback

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type PlaybackState struct {
	IsPlaying    bool   `json:"is_playing"`
	ShuffleState bool   `json:"shuffle_state"`
	RepeatState  string `json:"repeat_state"`
	ProgressMs   int    `json:"progress_ms"`
	Item         Track  `json:"item"`
}

type Track struct {
	Name       string   `json:"name"`
	DurationMs int      `json:"duration_ms"`
	Artists    []Artist `json:"artists"`
}

type Artist struct {
	Name string `json:"name"`
}

func IsPlaying(accessToken string) (bool, error) {
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

	// If nothing is currently active, Spotify returns an empty body
	// with a 204 status - there's no JSON to parse in that case.
	if len(body) == 0 {
		fmt.Println("No active playback session")
		return false, nil
	}

	var state PlaybackState
	if err := json.Unmarshal(body, &state); err != nil {
		return false, err
	}

	return state.IsPlaying, nil
}

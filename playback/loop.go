package playback

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type repeatStateResponse struct {
	RepeatState string `json:"repeat_state"`
}

// GetRepeatState reads Spotify's current repeat mode: "off", "context"
// (repeat the whole playlist/album), or "track" (repeat just this song).
func GetRepeatState(accessToken string) (string, error) {
	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/player", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Same edge case as IsPlaying/IsShuffled - empty body means no
	// active session, which is as good as "off".
	if len(body) == 0 {
		return "off", nil
	}

	var state repeatStateResponse
	if err := json.Unmarshal(body, &state); err != nil {
		return "", err
	}

	return state.RepeatState, nil
}

// SetRepeatState sets Spotify's repeat mode. Valid values are "track",
// "context", and "off".
func SetRepeatState(accessToken, state string) error {
	url := fmt.Sprintf("https://api.spotify.com/v1/me/player/repeat?state=%s", state)
	return doPlayerRequest("PUT", url, accessToken, nil)
}

// nextRepeatState cycles through Spotify's repeat modes in the same
// order the official app's repeat button does: off -> context -> track
// -> off.
func nextRepeatState(current string) string {
	switch current {
	case "off":
		return "context"
	case "context":
		return "track"
	default:
		return "off"
	}
}

// ToggleLoop advances the repeat mode one step forward from current and
// returns the mode it switched to.
func ToggleLoop(accessToken, current string) (string, error) {
	next := nextRepeatState(current)
	if err := SetRepeatState(accessToken, next); err != nil {
		return current, err
	}
	return next, nil
}

package playback

import (
	"encoding/json"
)

type PlaybackState struct {
	IsPlaying    bool   `json:"is_playing"`
	ShuffleState bool   `json:"shuffle_state"`
	RepeatState  string `json:"repeat_state"`
	ProgressMs   int    `json:"progress_ms"`
	Item         Track  `json:"item"`
}

// Track is Spotify's shape for whatever is currently playing - a music
// track (Artists populated, Show nil) or a podcast episode (Show
// populated, Artists nil/empty). Name and DurationMs are present
// either way.
type Track struct {
	Name       string   `json:"name"`
	DurationMs int      `json:"duration_ms"`
	Artists    []Artist `json:"artists"`
	Show       *Show    `json:"show"`
	// URI is Spotify's own "spotify:track:..."/"spotify:episode:..." URI
	// scheme - opening it hands off to the desktop app directly (if
	// installed and registered as the URI handler), unlike the
	// https://open.spotify.com/... web link, which always opens in a
	// browser instead.
	URI string `json:"uri"`
}

type Artist struct {
	Name string `json:"name"`
}

// Show is the podcast a currently-playing episode belongs to - present
// in place of Artists when Track represents an episode rather than a
// track.
type Show struct {
	Name string `json:"name"`
}

func IsPlaying(accessToken string) (bool, error) {
	body, err := doPlayerGet("https://api.spotify.com/v1/me/player", accessToken)
	if err != nil {
		return false, err
	}

	// If nothing is currently active, Spotify returns an empty body
	// with a 204 status - there's no JSON to parse in that case, and
	// nothing is playing.
	if body == nil {
		return false, nil
	}

	var state PlaybackState
	if err := json.Unmarshal(body, &state); err != nil {
		return false, err
	}

	return state.IsPlaying, nil
}

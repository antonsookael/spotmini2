package playback

import (
	"encoding/json"
	"io"
	"net/http"
)

// NowPlaying fetches the current playback state, including the track
// name, artist(s), and progress into the song. Same endpoint as
// IsPlaying/IsShuffled - reuses the shared PlaybackState struct so
// there's only one type declaration for this JSON shape across the
// whole package.
//
// additional_types=track,episode is required here specifically (the
// other callers of this endpoint don't touch `item` so they don't need
// it) - without it, Spotify silently returns item: null for a podcast
// episode even though currently_playing_type correctly says "episode",
// a backwards-compatibility quirk for clients that only ever expected
// tracks.
func NowPlaying(accessToken string) (PlaybackState, error) {
	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/player?additional_types=track,episode", nil)
	if err != nil {
		return PlaybackState{}, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return PlaybackState{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return PlaybackState{}, err
	}

	if len(body) == 0 {
		return PlaybackState{}, nil
	}

	var state PlaybackState
	if err := json.Unmarshal(body, &state); err != nil {
		return PlaybackState{}, err
	}

	return state, nil
}

package playback

import (
	"encoding/json"
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
	body, err := doPlayerGet("https://api.spotify.com/v1/me/player?additional_types=track,episode", accessToken)
	if err != nil {
		return PlaybackState{}, err
	}

	// Same edge case as IsPlaying - no body means no active session,
	// which the caller reads as nothing playing.
	if body == nil {
		return PlaybackState{}, nil
	}

	var state PlaybackState
	if err := json.Unmarshal(body, &state); err != nil {
		return PlaybackState{}, err
	}

	return state, nil
}

package playback

import (
	"encoding/json"
	"fmt"
	"time"
)

// RecentPlay is one entry of Spotify's listening history. Spotify only
// lists a track here once it's been played for at least 30 seconds.
type RecentPlay struct {
	Track    TrackResult
	PlayedAt time.Time
}

// GetRecentlyPlayed returns up to 50 plays made after the given time,
// newest first. A zero after asks for the latest 50 - which is also all
// Spotify will ever give, however far back after reaches.
func GetRecentlyPlayed(accessToken string, after time.Time) ([]RecentPlay, error) {
	u := "https://api.spotify.com/v1/me/player/recently-played?limit=50"
	if !after.IsZero() {
		u += fmt.Sprintf("&after=%d", after.UnixMilli())
	}
	body, err := doGet(u, accessToken)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Items []struct {
			Track    apiTrack  `json:"track"`
			PlayedAt time.Time `json:"played_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing recently played response: %w (raw: %s)", err, string(body))
	}

	plays := make([]RecentPlay, len(parsed.Items))
	for i, item := range parsed.Items {
		plays[i] = RecentPlay{Track: item.Track.result(), PlayedAt: item.PlayedAt}
	}
	return plays, nil
}

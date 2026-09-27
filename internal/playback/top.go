package playback

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// Spotify's names for the windows its top lists are computed over.
const (
	RangeShort  = "short_term"  // roughly the last 4 weeks
	RangeMedium = "medium_term" // roughly the last 6 months
	RangeLong   = "long_term"   // several years
)

// apiTrack is Spotify's full track object, trimmed to what anything here
// reads.
type apiTrack struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	URI        string   `json:"uri"`
	DurationMs int      `json:"duration_ms"`
	Artists    []Artist `json:"artists"`
}

func (t apiTrack) result() TrackResult {
	artist := ""
	if len(t.Artists) > 0 {
		artist = t.Artists[0].Name
	}
	return TrackResult{ID: t.ID, Name: t.Name, URI: t.URI, Artist: artist, DurationMs: t.DurationMs}
}

type TopArtist struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// GetTopTracks returns one page of the user's most-played tracks over
// timeRange, best first. Spotify caps a page at 50.
func GetTopTracks(accessToken, timeRange string, limit, offset int) ([]TrackResult, error) {
	u := fmt.Sprintf("https://api.spotify.com/v1/me/top/tracks?time_range=%s&limit=%d&offset=%d",
		url.QueryEscape(timeRange), limit, offset)
	body, err := doGet(u, accessToken)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Items []apiTrack `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing top tracks response: %w (raw: %s)", err, string(body))
	}

	results := make([]TrackResult, len(parsed.Items))
	for i, item := range parsed.Items {
		results[i] = item.result()
	}
	return results, nil
}

// GetTopArtists returns the user's most-played artists over timeRange,
// best first.
func GetTopArtists(accessToken, timeRange string, limit int) ([]TopArtist, error) {
	u := fmt.Sprintf("https://api.spotify.com/v1/me/top/artists?time_range=%s&limit=%d",
		url.QueryEscape(timeRange), limit)
	body, err := doGet(u, accessToken)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Items []TopArtist `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing top artists response: %w (raw: %s)", err, string(body))
	}
	return parsed.Items, nil
}

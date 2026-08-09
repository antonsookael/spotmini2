package playback

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// TrackResult is a single hit from SearchTracks - just enough to show
// in a results list and start playing or save it.
type TrackResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URI    string `json:"uri"`
	Artist string `json:"artist"`
}

type searchResponse struct {
	Tracks struct {
		Items []struct {
			ID      string   `json:"id"`
			Name    string   `json:"name"`
			URI     string   `json:"uri"`
			Artists []Artist `json:"artists"`
		} `json:"items"`
	} `json:"tracks"`
}

// SearchTracks searches Spotify's catalog for tracks matching query,
// up to limit results.
func SearchTracks(accessToken, query string, limit int) ([]TrackResult, error) {
	u := fmt.Sprintf("https://api.spotify.com/v1/search?type=track&limit=%d&q=%s", limit, url.QueryEscape(query))

	body, err := doGet(u, accessToken)
	if err != nil {
		return nil, err
	}

	var parsed searchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing search response: %w (raw: %s)", err, string(body))
	}

	results := make([]TrackResult, len(parsed.Tracks.Items))
	for i, item := range parsed.Tracks.Items {
		artist := ""
		if len(item.Artists) > 0 {
			artist = item.Artists[0].Name
		}
		results[i] = TrackResult{ID: item.ID, Name: item.Name, URI: item.URI, Artist: artist}
	}
	return results, nil
}

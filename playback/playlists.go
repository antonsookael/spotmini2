package playback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Playlist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URI  string `json:"uri"`
}

type playlistsResponse struct {
	Items []Playlist `json:"items"`
}

// GetPlaylists lists the current user's playlists, up to Spotify's
// per-page max of 50. Deliberately not paginating further than that -
// it covers the overwhelming majority of libraries and keeps this to a
// single request.
func GetPlaylists(accessToken string) ([]Playlist, error) {
	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/playlists?limit=50", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed playlistsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing playlists response: %w (raw: %s)", err, string(body))
	}

	return parsed.Items, nil
}

// PlayPlaylist starts playback of the playlist at uri from the
// beginning.
func PlayPlaylist(accessToken, uri string) error {
	payload, err := json.Marshal(map[string]interface{}{
		"context_uri": uri,
	})
	if err != nil {
		return err
	}
	return doPlayerRequest("PUT", "https://api.spotify.com/v1/me/player/play", accessToken, bytes.NewReader(payload))
}

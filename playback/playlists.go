package playback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

	// Checked explicitly rather than just trying to unmarshal whatever
	// came back - an error response (e.g. 403 for a missing OAuth
	// scope) is still valid JSON, just not shaped like
	// playlistsResponse, so it would otherwise silently unmarshal into
	// an empty Items and look identical to "no playlists" instead of
	// surfacing the real failure.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("spotify returned status %d: %s", resp.StatusCode, string(body))
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

type savedTrack struct {
	Track struct {
		URI string `json:"uri"`
	} `json:"track"`
}

type savedTracksResponse struct {
	Items []savedTrack `json:"items"`
}

// GetLikedSongURIs fetches the URIs of the user's most recently saved
// tracks, up to Spotify's per-page max of 50. Liked Songs has no
// context_uri the play endpoint accepts the way playlists/albums do,
// so playing it means passing explicit track URIs instead of a
// context - this is what supplies those.
func GetLikedSongURIs(accessToken string) ([]string, error) {
	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/tracks?limit=50", nil)
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

	// See the identical check in GetPlaylists - without this, an error
	// response (e.g. missing OAuth scope) unmarshals into an empty
	// Items and looks exactly like "no saved tracks" instead of
	// surfacing the real failure.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("spotify returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed savedTracksResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing saved tracks response: %w (raw: %s)", err, string(body))
	}

	uris := make([]string, len(parsed.Items))
	for i, item := range parsed.Items {
		uris[i] = item.Track.URI
	}
	return uris, nil
}

// PlayURIs starts playback of the given track URIs, in order.
func PlayURIs(accessToken string, uris []string) error {
	payload, err := json.Marshal(map[string]interface{}{
		"uris": uris,
	})
	if err != nil {
		return err
	}
	return doPlayerRequest("PUT", "https://api.spotify.com/v1/me/player/play", accessToken, bytes.NewReader(payload))
}

// SaveTrack adds the track with the given Spotify ID to the current
// user's Liked Songs. Not a player command (no device involved), so
// unlike PlayURIs this doesn't go through doPlayerRequest/withDeviceRevival.
func SaveTrack(accessToken, id string) error {
	req, err := http.NewRequest("PUT", "https://api.spotify.com/v1/me/tracks?ids="+url.QueryEscape(id), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("spotify returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

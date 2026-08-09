package playback

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	// Through the shared helper, which is what turns a rejection into a
	// named cause. Hand-rolled, this returned a bare "status 401" that
	// nothing could recognise, so a token that had died early left the
	// picker broken for the session while playback recovered.
	body, err := doGet("https://api.spotify.com/v1/me/playlists?limit=50", accessToken)
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
	// Through the shared helper for the same reason as GetPlaylists: an
	// error response unmarshals into an empty Items and looks exactly
	// like "no saved tracks", and a bare status error tells the caller
	// nothing it can act on.
	body, err := doGet("https://api.spotify.com/v1/me/tracks?limit=50", accessToken)
	if err != nil {
		return nil, err
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
	// doPlayerRequest despite not being a player command: what's wanted
	// is its rejection handling, so a Like refused for a dead token
	// reaches the caller as something the bar can name.
	return doPlayerRequest("PUT", "https://api.spotify.com/v1/me/tracks?ids="+url.QueryEscape(id), accessToken, nil)
}

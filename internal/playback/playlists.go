package playback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"spotmini-gui/internal/logging"
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

// PlayInContext starts trackURI playing from inside contextURI (its
// album, say), so the track starts at once and the context carries on
// after it. The way a single picked track is played now that a bare
// URI list is ignored (see PlayURIs) - the queue would have put it
// behind whatever was queued already.
func PlayInContext(accessToken, contextURI, trackURI string) error {
	payload, err := json.Marshal(map[string]interface{}{
		"context_uri": contextURI,
		"offset":      map[string]string{"uri": trackURI},
	})
	if err != nil {
		return err
	}
	return doPlayerRequest("PUT", "https://api.spotify.com/v1/me/player/play", accessToken, bytes.NewReader(payload))
}

// TrackAlbumURI looks up the album a track belongs to, for PlayInContext.
func TrackAlbumURI(accessToken, trackURI string) (string, error) {
	id := strings.TrimPrefix(trackURI, "spotify:track:")
	body, err := doGet("https://api.spotify.com/v1/tracks/"+url.PathEscape(id), accessToken)
	if err != nil {
		return "", err
	}

	var parsed struct {
		Album struct {
			URI string `json:"uri"`
		} `json:"album"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parsing track response: %w (raw: %s)", err, string(body))
	}
	if parsed.Album.URI == "" {
		return "", fmt.Errorf("track %s has no album in its response (raw: %s)", id, string(body))
	}
	return parsed.Album.URI, nil
}

// GetUserID returns the signed-in account's Spotify user ID, which the
// Liked Songs context is addressed by.
func GetUserID(accessToken string) (string, error) {
	body, err := doGet("https://api.spotify.com/v1/me", accessToken)
	if err != nil {
		return "", err
	}

	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parsing profile response: %w (raw: %s)", err, string(body))
	}
	if parsed.ID == "" {
		return "", fmt.Errorf("profile response has no user id (raw: %s)", string(body))
	}
	return parsed.ID, nil
}

// PlayLikedSongs plays the user's whole Liked Songs library, newest
// first, through the collection context Spotify keeps for it. This used
// to fetch the 50 newest saved tracks and play them as a URI list, until
// Spotify stopped acting on URI lists (see PlayURIs) - the context both
// still works and covers the entire library.
func PlayLikedSongs(accessToken, userID string) error {
	return PlayPlaylist(accessToken, "spotify:user:"+userID+":collection")
}

// queueHead is how many tracks PlayURIs queues before starting playback.
// The rest follow in the background: a favorites batch is 50 songs, and
// 50 queue requests before the first note would be a ten-second wait.
const queueHead = 5

// PlayURIs starts the given tracks playing, in order.
//
// Not through the play endpoint's "uris" list, which Spotify stopped
// honouring in November 2026: it answers 204 and then plays nothing,
// with any number of tracks, with or without offset, position or
// device. The queue still works, so the tracks are queued and the
// player skipped to the first of them. Anything the user had queued
// already plays first; that's the queue's order and not worth fighting.
//
// Returns once the first queueHead tracks are queued and the skip has
// gone through. The remaining tracks queue in the background, with
// failures only logged - by then playback has started, and the bar has
// nothing more to tell the user than that the tail of the list went
// missing.
func PlayURIs(accessToken string, uris []string) error {
	if len(uris) == 0 {
		return nil
	}

	head := min(len(uris), queueHead)
	for _, uri := range uris[:head] {
		if err := queueTrack(accessToken, uri); err != nil {
			return err
		}
	}
	if err := doPlayerRequest("POST", "https://api.spotify.com/v1/me/player/next", accessToken, nil); err != nil {
		return err
	}

	if rest := uris[head:]; len(rest) > 0 {
		go func() {
			for i, uri := range rest {
				if err := queueTrack(accessToken, uri); err != nil {
					logging.Printf("Queueing track %d of %d failed, the rest of the list is dropped: %v", head+i+1, len(uris), err)
					return
				}
			}
		}()
	}
	return nil
}

// queueTrack appends one track to the player's queue.
func queueTrack(accessToken, uri string) error {
	return doPlayerRequest("POST", "https://api.spotify.com/v1/me/player/queue?uri="+url.QueryEscape(uri), accessToken, nil)
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

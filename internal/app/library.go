package app

import (
	"strings"

	"spotmini-gui/internal/logging"
	"spotmini-gui/internal/playback"
)

// searchResultLimit caps how many tracks the picker's search returns -
// the panel is short, so more than this just adds scrolling.
const searchResultLimit = 8

// GetPlaylists lists the current user's playlists, for the playlist
// picker panel.
//
// The error is returned rather than swallowed into an empty list so the
// panel can tell a failed load from an account with no playlists - it
// caches what it gets, and caching a failure pinned the picker to
// nothing but Liked Songs for the rest of the session.
func (a *App) GetPlaylists() ([]playback.Playlist, error) {
	playlists, err := playback.GetPlaylists(a.getToken())
	if err != nil {
		logging.Printf("Failed to fetch playlists: %v", err)
		return nil, err
	}
	return playlists, nil
}

// PlayPlaylist starts playback of the playlist at uri from the
// beginning.
func (a *App) PlayPlaylist(uri string) {
	a.withTrackChange("playPlaylist", func(token string) error {
		return playback.PlayPlaylist(token, uri)
	})
}

// PlayLikedSongs plays the most recently saved tracks. Liked Songs has
// no playlist URI, so it plays explicit track URIs instead.
func (a *App) PlayLikedSongs() {
	token := a.getToken()
	uris, err := playback.GetLikedSongURIs(token)
	if err != nil {
		logging.Printf("Failed to fetch liked songs: %v", err)
		return
	}
	if len(uris) == 0 {
		return
	}
	a.withTrackChange("playLikedSongs", func(token string) error {
		return playback.PlayURIs(token, uris)
	})
}

// SearchTracks searches Spotify's catalog for the picker's search box.
// A blank query short-circuits rather than hitting the API.
func (a *App) SearchTracks(query string) []playback.TrackResult {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	results, err := playback.SearchTracks(a.getToken(), query, searchResultLimit)
	if err != nil {
		logging.Printf("Track search failed: %v", err)
		return nil
	}
	return results
}

// PlayTrack starts playback of a single track by URI - used when
// picking a song from search results.
func (a *App) PlayTrack(uri string) {
	a.withTrackChange("playTrack", func(token string) error {
		return playback.PlayURIs(token, []string{uri})
	})
}

// SaveTrackToLiked adds a track to the current user's Liked Songs.
// Returns the failure as well as logging it: the button turns into a
// tick on the strength of this call, so swallowing the error left the
// UI claiming a save that never happened.
func (a *App) SaveTrackToLiked(id string) error {
	if err := playback.SaveTrack(a.getToken(), id); err != nil {
		logging.Printf("Failed to save track: %v", err)
		return err
	}
	return nil
}

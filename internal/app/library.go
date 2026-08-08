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
func (a *App) GetPlaylists() []playback.Playlist {
	playlists, err := playback.GetPlaylists(a.getToken())
	if err != nil {
		return nil
	}
	return playlists
}

// PlayPlaylist starts playback of the playlist at uri from the
// beginning.
func (a *App) PlayPlaylist(uri string) {
	a.withDeviceRevival(func(token string) error {
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
	a.withDeviceRevival(func(token string) error {
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
	a.withDeviceRevival(func(token string) error {
		return playback.PlayURIs(token, []string{uri})
	})
}

// SaveTrackToLiked adds a track to the current user's Liked Songs.
func (a *App) SaveTrackToLiked(id string) {
	if err := playback.SaveTrack(a.getToken(), id); err != nil {
		logging.Printf("Failed to save track: %v", err)
	}
}

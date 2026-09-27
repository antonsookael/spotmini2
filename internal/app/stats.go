package app

import (
	"errors"
	"fmt"
	"time"

	"spotmini-gui/internal/logging"
	"spotmini-gui/internal/playback"
	"spotmini-gui/internal/stats"
)

// spotifyTopTTL is how long Spotify's top lists are reused. Spotify
// itself only recomputes them about once a day.
const spotifyTopTTL = time.Hour

type SpotifyTop struct {
	Tracks  []playback.TrackResult `json:"tracks"`
	Artists []playback.TopArtist   `json:"artists"`
}

type cachedTop struct {
	top       SpotifyTop
	fetchedAt time.Time
}

// GetSpotifyTop returns Spotify's own ranking of the user's top tracks
// and artists over timeRange.
func (a *App) GetSpotifyTop(timeRange string) (SpotifyTop, error) {
	if !stats.ValidRange(timeRange) {
		return SpotifyTop{}, fmt.Errorf("unknown time range %q", timeRange)
	}
	top, err := a.spotifyTop(timeRange)
	if err != nil {
		logging.Printf("Could not read top tracks: %v", err)
		return SpotifyTop{}, errors.New(failureMessage(err))
	}
	return top, nil
}

func (a *App) spotifyTop(timeRange string) (SpotifyTop, error) {
	a.topMu.Lock()
	cached, ok := a.topCache[timeRange]
	a.topMu.Unlock()
	if ok && time.Since(cached.fetchedAt) < spotifyTopTTL {
		return cached.top, nil
	}

	tracks, err := retryOnAuthFailure(a, func(token string) ([]playback.TrackResult, error) {
		return playback.GetTopTracks(token, timeRange, 50, 0)
	})
	if err != nil {
		return SpotifyTop{}, err
	}
	// Spotify ranks just under a hundred at most. The second page is a
	// nice-to-have: the first alone is a usable list.
	if len(tracks) == 50 {
		more, err := retryOnAuthFailure(a, func(token string) ([]playback.TrackResult, error) {
			return playback.GetTopTracks(token, timeRange, 49, 50)
		})
		if err != nil {
			logging.Printf("Could not read the second page of top tracks: %v", err)
		}
		tracks = append(tracks, more...)
	}

	artists, err := retryOnAuthFailure(a, func(token string) ([]playback.TopArtist, error) {
		return playback.GetTopArtists(token, timeRange, 20)
	})
	if err != nil {
		return SpotifyTop{}, err
	}

	top := SpotifyTop{Tracks: tracks, Artists: artists}
	a.topMu.Lock()
	if a.topCache == nil {
		a.topCache = make(map[string]cachedTop)
	}
	a.topCache[timeRange] = cachedTop{top: top, fetchedAt: time.Now()}
	a.topMu.Unlock()
	return top, nil
}

// GetLocalStats returns what spotmini's own play log says about
// timeRange.
func (a *App) GetLocalStats(timeRange string) (stats.LocalStats, error) {
	if !stats.ValidRange(timeRange) {
		return stats.LocalStats{}, fmt.Errorf("unknown time range %q", timeRange)
	}
	return a.stats.LocalStats(timeRange), nil
}

// historyBackfillInterval is how often Spotify's listening history is
// merged in. It only holds the last 50 plays, so this has to come round
// well before 50 songs can go by unseen - which only happens while
// playback is on another device and the app isn't reading, or the
// laptop is asleep.
const historyBackfillInterval = 30 * time.Minute

// backfillHistoryLoop merges Spotify's history now and then on a timer,
// picking up plays the app wasn't running for.
func (a *App) backfillHistoryLoop() {
	a.backfillHistory()
	ticker := time.NewTicker(historyBackfillInterval)
	defer ticker.Stop()
	for range ticker.C {
		a.backfillHistory()
	}
}

func (a *App) backfillHistory() {
	cursor := a.stats.RecentCursor()
	plays, err := retryOnAuthFailure(a, func(token string) ([]playback.RecentPlay, error) {
		return playback.GetRecentlyPlayed(token, cursor)
	})
	if err != nil {
		logging.Printf("Could not read listening history: %v", err)
		return
	}
	a.stats.MergeRecent(plays)
}

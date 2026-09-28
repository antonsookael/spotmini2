package app

import (
	"errors"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spotmini-gui/internal/logging"
	"spotmini-gui/internal/playback"
	"spotmini-gui/internal/stats"
)

// Why favorites mode ended, when it ended by itself, so the bar can say.
const (
	endedNone      = ""
	endedSwitched  = "switched"   // something else started playing
	endedCantStart = "cant-start" // the mix couldn't be started or refilled
)

func (a *App) announceFavoritesMode(ended string) {
	runtime.EventsEmit(a.ctx, "favorites-mode-changed", a.stats.Mode(), ended)
}

// StartFavorites plays a batch of the user's favorites for timeRange and
// switches favorites mode on.
func (a *App) StartFavorites(timeRange string) error {
	if !stats.ValidRange(timeRange) {
		return fmt.Errorf("unknown time range %q", timeRange)
	}
	top, err := a.spotifyTop(timeRange)
	if err != nil {
		logging.Printf("Favorites: could not read top tracks: %v", err)
		return errors.New(failureMessage(err))
	}
	batch, err := a.stats.StartFavorites(timeRange, top.Tracks)
	if errors.Is(err, stats.ErrNoFavorites) {
		return errors.New("No favorites for this range yet")
	}
	if err != nil {
		return err
	}
	a.playFavoritesBatch("favorites", batch)
	return nil
}

// playFavoritesBatch starts batch playing, and leaves favorites mode if
// that fails - a mode waiting on songs that never started would end
// itself on whatever plays next anyway, but in the meantime the bar
// would claim a mode that isn't happening.
func (a *App) playFavoritesBatch(name string, batch []string) {
	err := a.withTrackChange(name, func(token string) error {
		// Before the play request, so the batch starts at the top in the
		// mode's own order. Shuffle would throw away the head start the
		// biggest favorites were given, and repeat-all would replay this
		// batch instead of letting the next one be drawn. Repeat-one is
		// left alone: looping a favorite is still listening to it.
		if err := playback.ToggleShuffle(token, false); err != nil {
			return err
		}
		if repeat, err := playback.GetRepeatState(token); err == nil && repeat == "context" {
			if err := playback.SetRepeatState(token, "off"); err != nil {
				return err
			}
		}
		return playback.PlayURIs(token, batch)
	})
	ended := endedNone
	if err != nil {
		logging.Printf("Favorites: could not start a batch: %v", err)
		a.stats.StopFavorites()
		ended = endedCantStart
	}
	a.announceFavoritesMode(ended)
}

// nextFavoritesBatch keeps favorites mode going once a batch has played
// through, rather than leaving Spotify to autoplay something else.
func (a *App) nextFavoritesBatch() {
	mode := a.stats.Mode()
	if !mode.Active {
		return
	}
	// Without the ranking the batch still draws on the songs played and
	// kept here, which beats leaving Spotify to autoplay.
	top, err := a.spotifyTop(mode.Range)
	if err != nil {
		logging.Printf("Favorites: could not read top tracks for the next batch: %v", err)
	}
	batch, ok := a.stats.NextBatch(top.Tracks)
	if !ok {
		a.announceFavoritesMode(endedCantStart)
		return
	}
	a.playFavoritesBatch("favoritesNext", batch)
}

// PlayFavorite plays a song picked from the favorites list, as the start
// of a fresh mix rather than on its own, so the mode carries on.
func (a *App) PlayFavorite(uri string) error {
	mode := a.stats.Mode()
	if !mode.Active {
		return errors.New("Favorites mode isn't on")
	}
	top, err := a.spotifyTop(mode.Range)
	if err != nil {
		logging.Printf("Favorites: could not read top tracks for a picked song: %v", err)
	}
	a.playFavoritesBatch("favoritesPick", a.stats.StartFavoritesWith(uri, mode.Range, top.Tracks))
	return nil
}

// StopFavorites leaves favorites mode. Whatever is playing carries on.
func (a *App) StopFavorites() {
	a.stats.StopFavorites()
	a.announceFavoritesMode(endedNone)
}

func (a *App) GetFavoritesMode() stats.ModeStatus {
	return a.stats.Mode()
}

// GetFavorites lists the songs favorites mode picks from for timeRange,
// and the ones that have been dropped.
func (a *App) GetFavorites(timeRange string) (stats.FavoritesView, error) {
	if !stats.ValidRange(timeRange) {
		return stats.FavoritesView{}, fmt.Errorf("unknown time range %q", timeRange)
	}
	top, err := a.spotifyTop(timeRange)
	if err != nil {
		logging.Printf("Favorites: could not read top tracks: %v", err)
		return stats.FavoritesView{}, errors.New(failureMessage(err))
	}
	return a.stats.Favorites(timeRange, top.Tracks), nil
}

// DropCurrentFavorite takes the playing song out of favorites mode for
// good and moves on to the next. Returns the dropped song's name.
func (a *App) DropCurrentFavorite() (string, error) {
	info, err := a.stats.DropCurrent()
	if err != nil {
		return "", errors.New("Only songs from favorites mode can be dropped")
	}
	a.withTrackChange("drop", playback.NextTrack)
	return info.Name, nil
}

func (a *App) UndropFavorite(uri string) {
	a.stats.Undrop(uri)
}

// GetResumeFavorites reports whether a favorites mode still running at
// quit picks up again on the next launch.
func (a *App) GetResumeFavorites() bool {
	return a.stats.ResumeOnLaunch()
}

func (a *App) SetResumeFavorites(on bool) {
	a.stats.SetResumeOnLaunch(on)
}

// WakeFavorite ends a skipped song's rest early.
func (a *App) WakeFavorite(uri string) {
	a.stats.Wake(uri)
}

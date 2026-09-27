package app

import (
	"errors"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spotmini-gui/internal/logging"
	"spotmini-gui/internal/playback"
	"spotmini-gui/internal/stats"
)

func (a *App) announceFavoritesMode() {
	runtime.EventsEmit(a.ctx, "favorites-mode-changed", a.stats.Mode())
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
		return playback.PlayURIs(token, batch)
	})
	if err != nil {
		logging.Printf("Favorites: could not start a batch: %v", err)
		a.stats.StopFavorites()
	}
	a.announceFavoritesMode()
}

// nextFavoritesBatch keeps favorites mode going once a batch has played
// through, rather than leaving Spotify to autoplay something else.
func (a *App) nextFavoritesBatch() {
	batch, ok := a.stats.NextBatch()
	if !ok {
		a.announceFavoritesMode()
		return
	}
	a.playFavoritesBatch("favoritesNext", batch)
}

// StopFavorites leaves favorites mode. Whatever is playing carries on.
func (a *App) StopFavorites() {
	a.stats.StopFavorites()
	a.announceFavoritesMode()
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

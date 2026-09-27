package app

import (
	"time"

	"spotmini-gui/internal/logging"
	"spotmini-gui/internal/playback"
)

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

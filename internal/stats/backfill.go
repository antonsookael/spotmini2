package stats

import (
	"slices"
	"time"

	"spotmini-gui/internal/playback"
)

// sameListenSlack is added to a track's length when deciding whether a
// history entry and a play seen live are the same listen. Spotify's
// played_at and the start time recorded here sit up to a track's length
// apart, depending on which end of the listen Spotify stamps.
const sameListenSlack = time.Minute

// MergeRecent adds plays from Spotify's history that the app didn't see
// happen - made while it was closed, or asleep.
func (s *Service) MergeRecent(items []playback.RecentPlay) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mergeRecent(items)
}

// Caller holds mu.
func (s *Service) mergeRecent(items []playback.RecentPlay) {
	// A live play can only stand for one history entry: a song played
	// twice in a row is two entries and two local plays, not one.
	matched := make(map[int]bool)
	// Only plays from before this merge are searched: the ones it appends
	// are other history entries, and out of order until the sort below.
	existing := len(s.log.Plays)
	cursor := s.log.RecentCursor
	added := false

	for _, item := range items {
		uri := item.Track.URI
		if !isTrack(uri) || !item.PlayedAt.After(cursor) {
			continue
		}
		window := time.Duration(item.Track.DurationMs)*time.Millisecond + sameListenSlack

		// Still playing here, so it gets recorded live once it ends.
		if c := s.current; c != nil && c.uri == uri && within(c.startedAt, item.PlayedAt, window) {
			continue
		}

		found := false
		for i := existing - 1; i >= 0; i-- {
			p := s.log.Plays[i]
			if p.At.Before(item.PlayedAt.Add(-window)) {
				break
			}
			if !matched[i] && p.URI == uri && within(p.At, item.PlayedAt, window) {
				matched[i] = true
				found = true
				break
			}
		}
		if found {
			continue
		}

		s.log.Tracks[uri] = TrackInfo{Name: item.Track.Name, Artist: item.Track.Artist, DurationMs: item.Track.DurationMs}
		s.log.Plays = append(s.log.Plays, Play{
			URI: uri,
			At:  item.PlayedAt,
			// Spotify doesn't say how much was heard, only that it was
			// at least 30 seconds.
			ListenedMs: item.Track.DurationMs,
			Backfilled: true,
		})
		added = true
	}

	for _, item := range items {
		if item.PlayedAt.After(s.log.RecentCursor) {
			s.log.RecentCursor = item.PlayedAt
		}
	}

	if added {
		slices.SortStableFunc(s.log.Plays, func(a, b Play) int { return a.At.Compare(b.At) })
	}
	if added || !s.log.RecentCursor.Equal(cursor) {
		s.saveLog()
	}
}

func within(a, b time.Time, window time.Duration) bool {
	d := a.Sub(b)
	return d <= window && d >= -window
}

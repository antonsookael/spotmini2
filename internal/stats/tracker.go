package stats

import (
	"strings"
	"time"

	"spotmini-gui/internal/playback"
)

const (
	// Spotify's own threshold for a stream to count.
	minPlayMs = 30_000

	// Leaving a track before this much of it was heard is a skip.
	skipBeforeFraction = 0.5

	// An explicit Next from the app is a skip up to here - past it, the
	// song was essentially heard out.
	explicitSkipBeforeFraction = 0.8

	// Progress running backwards by more than this, after most of the
	// track was heard, is the track starting over (repeat one, or
	// restarted from the top) rather than a small seek back.
	restartJumpMs = 15_000

	// The most listening credited for the stretch between the last read
	// and the one that found something else playing. Reads come every
	// ten seconds or so while playing; a longer gap means the app wasn't
	// looking (a sleeping laptop), and guessing across it would only
	// inflate the count.
	maxUnseenGap = 15 * time.Second
)

// listen is the track currently being followed, from the first read
// that saw it until one that sees something else.
type listen struct {
	uri       string
	info      TrackInfo
	startedAt time.Time
	// The furthest point into the track any read has reported.
	heardMs  int
	lastSeen time.Time
	playing  bool

	inFavorites  bool
	explicitSkip bool
	dropped      bool
	// Left for a reason other than skipping it: Previous, or a new batch
	// replacing it.
	notASkip bool
}

func isTrack(uri string) bool {
	return strings.HasPrefix(uri, "spotify:track:")
}

func infoOf(item playback.Track) TrackInfo {
	artist := ""
	if len(item.Artists) > 0 {
		artist = item.Artists[0].Name
	}
	return TrackInfo{Name: item.Name, Artist: artist, DurationMs: item.DurationMs}
}

// Observe feeds in a playback read. Every successful read should come
// through here; plays are recorded when a read shows the track has
// changed or stopped.
func (s *Service) Observe(state playback.PlaybackState) Change {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.observe(state, time.Now())
}

// Caller holds mu.
func (s *Service) observe(state playback.PlaybackState, now time.Time) Change {
	uri := state.Item.URI
	if !isTrack(uri) {
		// Nothing on, or a podcast. Stopping isn't skipping, so whatever
		// was on ends as heard.
		if s.current != nil {
			s.finish(now, false, false)
			s.current = nil
		}
		return NoChange
	}

	if c := s.current; c != nil && c.uri == uri {
		if state.ProgressMs < c.heardMs-restartJumpMs && c.heardMs >= c.info.DurationMs/2 {
			s.finish(now.Add(-time.Duration(state.ProgressMs)*time.Millisecond), false, false)
			s.start(state, now)
			return NoChange
		}
		c.heardMs = max(c.heardMs, state.ProgressMs)
		c.lastSeen = now
		c.playing = state.IsPlaying
		return NoChange
	}

	prev := s.current
	if prev != nil {
		// The new track's progress is how long ago the switch happened,
		// which pins down how much of the old one played since last read.
		endAt := now
		if state.IsPlaying {
			endAt = now.Add(-time.Duration(state.ProgressMs) * time.Millisecond)
		}
		s.finish(endAt, true, s.mode != nil && s.mode.inBatch[uri])
	}
	change := s.modeChange(uri, prev, now)
	s.start(state, now)
	return change
}

// Caller holds mu.
func (s *Service) start(state playback.PlaybackState, now time.Time) {
	uri := state.Item.URI
	s.current = &listen{
		uri:       uri,
		info:      infoOf(state.Item),
		startedAt: now.Add(-time.Duration(state.ProgressMs) * time.Millisecond),
		heardMs:   state.ProgressMs,
		lastSeen:  now,
		playing:   state.IsPlaying,
	}
	if s.mode != nil && s.mode.inBatch[uri] {
		s.current.inFavorites = true
		s.mode.seen[uri] = true
	}
}

// listenedMs estimates how much of the track played, up to endAt.
func (l *listen) listenedMs(endAt time.Time) int {
	ms := l.heardMs
	if l.playing && endAt.After(l.lastSeen) {
		ms += int(min(endAt.Sub(l.lastSeen), maxUnseenGap).Milliseconds())
	}
	if l.info.DurationMs > 0 {
		ms = min(ms, l.info.DurationMs)
	}
	return ms
}

func (l *listen) skipped(listenedMs int, movedOn bool) bool {
	d := float64(l.info.DurationMs)
	if d == 0 {
		return false
	}
	heard := float64(listenedMs) / d
	if l.explicitSkip && heard < explicitSkipBeforeFraction {
		return true
	}
	return movedOn && heard < skipBeforeFraction
}

// finish records the current listen as having ended at endAt. movedOn
// says another track took over, as opposed to playback stopping, and
// intoBatch that the track was the favorites batch's next song.
//
// Caller holds mu.
func (s *Service) finish(endAt time.Time, movedOn, intoBatch bool) {
	c := s.current
	listened := c.listenedMs(endAt)
	skipped := c.skipped(listened, movedOn)

	// In favorites mode only a skip meant as one counts: Next, or moving
	// on to the batch's next song. Putting on something else entirely,
	// starting a new batch or going back isn't a verdict on the song.
	if c.inFavorites {
		skipped = skipped && !c.notASkip && (c.explicitSkip || intoBatch)
	}

	// Keeping a song is just listening to it; skipping it only means not
	// right now.
	if c.inFavorites && !c.dropped {
		if skipped {
			s.rest(c.uri, endAt)
			s.saveFav()
		} else if listened >= minPlayMs {
			s.fav.Kept[c.uri] = endAt
			s.saveFav()
		}
	}

	// A skip under the play threshold is still worth keeping from
	// favorites mode - it's what rests the song and nudges it down.
	if listened < minPlayMs && !(skipped && c.inFavorites) {
		return
	}

	s.log.Tracks[c.uri] = c.info
	s.log.Plays = append(s.log.Plays, Play{
		URI:         c.uri,
		At:          c.startedAt,
		ListenedMs:  listened,
		Skipped:     skipped,
		InFavorites: c.inFavorites,
	})
	s.saveLog()
}

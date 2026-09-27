package stats

import (
	"math"
	"time"

	"spotmini-gui/internal/playback"
)

const day = 24 * time.Hour

const (
	// What a skip in favorites mode is worth against a play's 1. Decays
	// like a play does, so the nudge wears off by itself.
	skipWeight = -0.3

	// How many plays the top of Spotify's ranking is worth, falling off
	// linearly down the list.
	rankPriorPlays = 5.0

	// The least the ranking ever counts for, once there's a full range
	// of history of our own to go on.
	minPriorShare = 0.2
)

// span is how far back a range reaches and how fast a play fades within
// it. Each play's weight halves every halfLife, which is what makes
// twenty plays this week outweigh thirty spread over six months.
type span struct {
	window   time.Duration // 0 = no limit
	halfLife time.Duration
}

var spans = map[string]span{
	playback.RangeShort:  {28 * day, 7 * day},
	playback.RangeMedium: {182 * day, 45 * day},
	playback.RangeLong:   {0, 365 * day},
}

// ValidRange reports whether r is one of Spotify's time range names.
func ValidRange(r string) bool {
	_, ok := spans[r]
	return ok
}

func (sp span) length() time.Duration {
	if sp.window == 0 {
		return sp.halfLife
	}
	return sp.window
}

func playWeight(p Play) float64 {
	if p.InFavorites && p.Skipped {
		return skipWeight
	}
	if p.ListenedMs >= minPlayMs {
		return 1
	}
	return 0
}

// localScores sums each track's plays as of now, each worth less the
// older it is. Plays after now don't count, so passing an earlier time
// gives the scores as they stood then.
//
// Caller holds mu.
func (s *Service) localScores(sp span, now time.Time) map[string]float64 {
	scores := make(map[string]float64)
	for _, p := range s.log.Plays {
		age := now.Sub(p.At)
		if age < 0 || (sp.window > 0 && age > sp.window) {
			continue
		}
		if w := playWeight(p); w != 0 {
			scores[p.URI] += w * math.Exp2(-float64(age)/float64(sp.halfLife))
		}
	}
	return scores
}

// priorShare is how much Spotify's ranking counts for: all of it with
// no history of our own, fading to minPriorShare once the log covers
// the whole range.
//
// Caller holds mu.
func (s *Service) priorShare(sp span, now time.Time) float64 {
	covered := min(1, float64(now.Sub(s.log.Started))/float64(sp.length()))
	return 1 - (1-minPriorShare)*max(0, covered)
}

// scores blends our own plays with Spotify's ranking for the range,
// given best first as ranking.
//
// Caller holds mu.
func (s *Service) scores(timeRange string, ranking []string, now time.Time) map[string]float64 {
	sp := spans[timeRange]
	scores := s.localScores(sp, now)
	share := s.priorShare(sp, now)
	for i, uri := range ranking {
		scores[uri] += share * rankPriorPlays * (1 - float64(i)/float64(len(ranking)))
	}
	return scores
}

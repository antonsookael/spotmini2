package stats

import (
	"cmp"
	"errors"
	"math"
	"slices"
	"time"

	"spotmini-gui/internal/playback"
)

const favoritesFile = "favorites.json"

const (
	// The highest-scoring songs that make up the pool, before kept songs
	// are added on top.
	poolSize = 100

	// Songs per play request. Drawn from the pool by score, so a big
	// favorite turns up in most batches and a marginal one now and then.
	batchSize = 50

	// A skipped song rests for baseRest, doubling with each further skip
	// inside skipMemory, up to maxRest.
	baseRest   = 12 * time.Hour
	maxRest    = 14 * day
	skipMemory = 14 * day

	// Something other than the batch playing this soon after it was
	// started is Spotify still reporting what was on before, not the
	// user choosing something else.
	modeGrace = 5 * time.Second

	// A batch counts as played through once this share of it has been
	// seen, so a few songs skipped between reads don't hold up the next.
	batchDoneShare = 0.9
)

var (
	ErrNoFavorites  = errors.New("no favorites for this range yet")
	ErrNotFavorites = errors.New("the current song isn't from favorites mode")
)

type rest struct {
	Until time.Time   `json:"until"`
	Skips []time.Time `json:"skips"`
}

type favState struct {
	// Songs listened to in favorites mode: they stay in the pool even
	// once they'd no longer score their way into it.
	Kept    map[string]time.Time `json:"kept"`
	Dropped map[string]time.Time `json:"dropped"`
	Resting map[string]rest      `json:"resting"`
}

// mode is favorites mode while it's on.
type mode struct {
	timeRange string
	ranking   []playback.TrackResult
	batch     []string
	inBatch   map[string]bool
	seen      map[string]bool
	startedAt time.Time
}

// Change is what a playback read did to favorites mode.
type Change int

const (
	NoChange Change = iota
	// Something else is playing, so the mode is over.
	ModeEnded
	// The batch has played through; the caller should start the next.
	BatchFinished
)

type ModeStatus struct {
	Active bool   `json:"active"`
	Range  string `json:"range"`
}

type FavoriteTrack struct {
	URI          string     `json:"uri"`
	Name         string     `json:"name"`
	Artist       string     `json:"artist"`
	Score        float64    `json:"score"`
	Kept         bool       `json:"kept"`
	RestingUntil *time.Time `json:"resting_until,omitempty"`
}

type FavoritesView struct {
	ModeStatus
	Pool    []FavoriteTrack `json:"pool"`
	Dropped []FavoriteTrack `json:"dropped"`
}

// Caller holds mu.
func (s *Service) saveFav() {
	s.save(favoritesFile, &s.fav)
}

func rankingURIs(ranking []playback.TrackResult) []string {
	uris := make([]string, len(ranking))
	for i, t := range ranking {
		uris[i] = t.URI
	}
	return uris
}

// pool is the songs favorites mode chooses from, best first, with the
// scores that ordered them.
//
// Caller holds mu.
func (s *Service) pool(timeRange string, ranking []playback.TrackResult, now time.Time) ([]string, map[string]float64) {
	scores := s.scores(timeRange, rankingURIs(ranking), now)

	var pool []string
	for uri, score := range scores {
		if _, dropped := s.fav.Dropped[uri]; !dropped && score > 0 && isTrack(uri) {
			pool = append(pool, uri)
		}
	}
	slices.SortFunc(pool, func(a, b string) int {
		return cmp.Or(cmp.Compare(scores[b], scores[a]), cmp.Compare(a, b))
	})
	pool = pool[:min(len(pool), poolSize)]

	in := make(map[string]bool, len(pool))
	for _, uri := range pool {
		in[uri] = true
	}
	var kept []string
	for uri := range s.fav.Kept {
		if _, dropped := s.fav.Dropped[uri]; !dropped && !in[uri] {
			kept = append(kept, uri)
		}
	}
	slices.Sort(kept)
	return append(pool, kept...), scores
}

// pickBatch draws up to batchSize songs from the pool, favoring higher
// scores, leaving out any that are resting - unless that would leave
// nothing at all.
//
// Caller holds mu.
func (s *Service) pickBatch(pool []string, scores map[string]float64, now time.Time) []string {
	awake := slices.DeleteFunc(slices.Clone(pool), func(uri string) bool {
		return s.fav.Resting[uri].Until.After(now)
	})
	if len(awake) == 0 {
		awake = slices.Clone(pool)
	}

	// Weighted sampling without replacement: each song draws u^(1/w)
	// and the highest draws win, which picks a song with twice the
	// weight about twice as readily while still leaving room for luck.
	keys := make(map[string]float64, len(awake))
	for _, uri := range awake {
		w := max(scores[uri], 0) + 0.5
		keys[uri] = math.Pow(s.rng.Float64(), 1/w)
	}
	slices.SortFunc(awake, func(a, b string) int {
		return cmp.Or(cmp.Compare(keys[b], keys[a]), cmp.Compare(a, b))
	})
	return awake[:min(len(awake), batchSize)]
}

// Caller holds mu.
func (s *Service) beginBatch(batch []string, now time.Time) {
	s.mode.batch = batch
	s.mode.inBatch = make(map[string]bool, len(batch))
	for _, uri := range batch {
		s.mode.inBatch[uri] = true
	}
	s.mode.seen = make(map[string]bool)
	s.mode.startedAt = now

	c := s.current
	if c == nil {
		return
	}
	// Already playing one of them - the play request restarts it, and
	// the read that sees that is the same track, so it'd never be marked.
	if s.mode.inBatch[c.uri] {
		c.inFavorites = true
		s.mode.seen[c.uri] = true
	}
	// Anything else is about to be replaced by the batch's first song,
	// which is the batch starting rather than the song being skipped.
	if c.uri != batch[0] {
		c.notASkip = true
	}
}

// StartFavorites switches favorites mode on and returns the songs to
// play. ranking is Spotify's top tracks for timeRange, best first.
func (s *Service) StartFavorites(timeRange string, ranking []playback.TrackResult) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startFavorites(timeRange, ranking, time.Now())
}

// Caller holds mu.
func (s *Service) startFavorites(timeRange string, ranking []playback.TrackResult, now time.Time) ([]string, error) {
	pool, scores := s.pool(timeRange, ranking, now)
	if len(pool) == 0 {
		return nil, ErrNoFavorites
	}
	s.mode = &mode{timeRange: timeRange, ranking: ranking}
	s.beginBatch(s.pickBatch(pool, scores, now), now)
	return s.mode.batch, nil
}

// NextBatch draws a fresh batch for the running mode. False if the mode
// has been switched off in the meantime.
func (s *Service) NextBatch() ([]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextBatch(time.Now())
}

// Caller holds mu.
func (s *Service) nextBatch(now time.Time) ([]string, bool) {
	if s.mode == nil {
		return nil, false
	}
	pool, scores := s.pool(s.mode.timeRange, s.mode.ranking, now)
	if len(pool) == 0 {
		s.mode = nil
		return nil, false
	}
	s.beginBatch(s.pickBatch(pool, scores, now), now)
	return s.mode.batch, true
}

func (s *Service) StopFavorites() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopFavorites()
}

// Caller holds mu.
func (s *Service) stopFavorites() {
	s.mode = nil
	// Leaving the mode and then skipping the song it left playing is
	// just skipping a song.
	if s.current != nil {
		s.current.inFavorites = false
	}
}

func (s *Service) Mode() ModeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.modeStatus()
}

// Caller holds mu.
func (s *Service) modeStatus() ModeStatus {
	if s.mode == nil {
		return ModeStatus{}
	}
	return ModeStatus{Active: true, Range: s.mode.timeRange}
}

// modeChange is what starting a listen of uri does to the mode. prev is
// the listen it replaces, if any.
//
// Caller holds mu.
func (s *Service) modeChange(uri string, prev *listen, now time.Time) Change {
	m := s.mode
	if m == nil || m.inBatch[uri] || now.Sub(m.startedAt) < modeGrace {
		return NoChange
	}
	last := m.batch[len(m.batch)-1]
	if (prev != nil && prev.uri == last) || float64(len(m.seen)) >= batchDoneShare*float64(len(m.batch)) {
		return BatchFinished
	}
	s.stopFavorites()
	return ModeEnded
}

// MarkSkip records that the user pressed Next, which in favorites mode
// is a skip even past the halfway point.
func (s *Service) MarkSkip() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.current; c != nil && c.inFavorites {
		c.explicitSkip = true
	}
}

// MarkBack records that the user pressed Previous: moving back to the
// song before isn't passing on this one.
func (s *Service) MarkBack() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.current; c != nil {
		c.notASkip = true
	}
}

// DropCurrent takes the song favorites mode is playing out of the mode
// for good. The caller moves playback on.
func (s *Service) DropCurrent() (TrackInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dropCurrent(time.Now())
}

// Caller holds mu.
func (s *Service) dropCurrent(now time.Time) (TrackInfo, error) {
	c := s.current
	if c == nil || !c.inFavorites {
		return TrackInfo{}, ErrNotFavorites
	}
	c.dropped = true
	s.fav.Dropped[c.uri] = now
	delete(s.fav.Kept, c.uri)
	delete(s.fav.Resting, c.uri)
	s.saveFav()

	// So the Dropped list can name it even if it went before counting as
	// a play.
	s.log.Tracks[c.uri] = c.info
	s.saveLog()
	return c.info, nil
}

func (s *Service) Undrop(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.fav.Dropped, uri)
	s.saveFav()
}

// rest sends a skipped song away for a while: baseRest for the first
// skip, doubling for each further one within skipMemory.
//
// Caller holds mu.
func (s *Service) rest(uri string, at time.Time) {
	r := s.fav.Resting[uri]
	r.Skips = slices.DeleteFunc(r.Skips, func(t time.Time) bool { return at.Sub(t) > skipMemory })
	r.Skips = append(r.Skips, at)
	// Capped as a shift count too, so a song skipped dozens of times
	// can't shift the duration into overflow.
	d := baseRest << min(len(r.Skips)-1, 8)
	r.Until = at.Add(min(d, maxRest))
	s.fav.Resting[uri] = r
}

// Favorites describes the pool for timeRange, and what's been dropped.
func (s *Service) Favorites(timeRange string, ranking []playback.TrackResult) FavoritesView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.favorites(timeRange, ranking, time.Now())
}

// Caller holds mu.
func (s *Service) favorites(timeRange string, ranking []playback.TrackResult, now time.Time) FavoritesView {
	info := make(map[string]TrackInfo, len(ranking))
	for _, t := range ranking {
		info[t.URI] = TrackInfo{Name: t.Name, Artist: t.Artist, DurationMs: t.DurationMs}
	}
	for uri, t := range s.log.Tracks {
		info[uri] = t
	}

	view := FavoritesView{ModeStatus: s.modeStatus()}
	pool, scores := s.pool(timeRange, ranking, now)
	for _, uri := range pool {
		t := FavoriteTrack{URI: uri, Name: info[uri].Name, Artist: info[uri].Artist, Score: scores[uri]}
		_, t.Kept = s.fav.Kept[uri]
		if until := s.fav.Resting[uri].Until; until.After(now) {
			t.RestingUntil = &until
		}
		view.Pool = append(view.Pool, t)
	}

	var dropped []string
	for uri := range s.fav.Dropped {
		dropped = append(dropped, uri)
	}
	slices.SortFunc(dropped, func(a, b string) int {
		return cmp.Or(s.fav.Dropped[b].Compare(s.fav.Dropped[a]), cmp.Compare(a, b))
	})
	for _, uri := range dropped {
		view.Dropped = append(view.Dropped, FavoriteTrack{URI: uri, Name: info[uri].Name, Artist: info[uri].Artist})
	}
	return view
}

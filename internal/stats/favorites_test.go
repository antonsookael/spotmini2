package stats

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"spotmini-gui/internal/playback"
)

func ranking(uris ...string) []playback.TrackResult {
	out := make([]playback.TrackResult, len(uris))
	for i, uri := range uris {
		out[i] = playback.TrackResult{URI: uri, Name: uri, Artist: "artist", DurationMs: 200_000}
	}
	return out
}

// favoritesPlaying starts favorites mode on a ranking of just uri and
// starts playing it, as the first read after the play request would.
func favoritesPlaying(t *testing.T, s *Service, uri string, at time.Time) {
	t.Helper()
	if _, err := s.startFavorites(playback.RangeShort, ranking(uri), at); err != nil {
		t.Fatal(err)
	}
	s.observe(state(uri, 0, true), at)
}

func TestPoolRules(t *testing.T) {
	s := newTestService(t)
	s.fav.Dropped["spotify:track:dropped"] = t0
	s.fav.Kept["spotify:track:kept"] = t0.Add(-300 * day)

	pool, _ := s.pool(playback.RangeShort, ranking("spotify:track:a", "spotify:track:dropped"), t0)

	if slices.Contains(pool, "spotify:track:dropped") {
		t.Error("a dropped song is in the pool")
	}
	if !slices.Contains(pool, "spotify:track:kept") {
		t.Error("a kept song fell out of the pool")
	}
	if pool[0] != "spotify:track:a" {
		t.Errorf("pool = %v, want the ranked song first", pool)
	}
}

func TestBatchesFavorHigherScores(t *testing.T) {
	s := newTestService(t)
	var uris []string
	for i := range 120 {
		uris = append(uris, fmt.Sprintf("spotify:track:%03d", i))
	}
	top, bottom := uris[0], uris[len(uris)-1]

	topHits, bottomHits := 0, 0
	for range 200 {
		batch, err := s.startFavorites(playback.RangeShort, ranking(uris...), t0)
		if err != nil {
			t.Fatal(err)
		}
		if len(batch) != batchSize {
			t.Fatalf("batch of %d, want %d", len(batch), batchSize)
		}
		if slices.Contains(batch, top) {
			topHits++
		}
		if slices.Contains(batch, bottom) {
			bottomHits++
		}
	}
	if topHits <= bottomHits*2 {
		t.Errorf("the top song made %d batches and the bottom one %d - the top should be far more frequent", topHits, bottomHits)
	}
}

func TestRestingSongsSitOutUnlessNothingElseIsLeft(t *testing.T) {
	s := newTestService(t)
	s.fav.Resting["spotify:track:a"] = rest{Until: t0.Add(day)}

	batch, _ := s.startFavorites(playback.RangeShort, ranking("spotify:track:a", "spotify:track:b"), t0)
	if slices.Contains(batch, "spotify:track:a") {
		t.Error("a resting song made the batch")
	}

	batch, _ = s.startFavorites(playback.RangeShort, ranking("spotify:track:a"), t0)
	if !slices.Contains(batch, "spotify:track:a") {
		t.Error("with everything resting, the batch came back empty instead of ignoring the rest")
	}
}

func TestNoFavorites(t *testing.T) {
	s := newTestService(t)
	if _, err := s.startFavorites(playback.RangeShort, nil, t0); !errors.Is(err, ErrNoFavorites) {
		t.Errorf("got %v, want ErrNoFavorites", err)
	}
}

func TestListeningKeepsASong(t *testing.T) {
	s := newTestService(t)
	favoritesPlaying(t, s, "spotify:track:a", t0)
	at := play(s, "spotify:track:a", t0, 10, 190)
	s.observe(playback.PlaybackState{}, at.Add(10*time.Second))

	if _, kept := s.fav.Kept["spotify:track:a"]; !kept {
		t.Error("listening through a song didn't keep it")
	}
	if !s.log.Plays[0].InFavorites {
		t.Error("the play wasn't marked as from favorites mode")
	}
}

func TestSkippingRestsASongLongerEachTime(t *testing.T) {
	s := newTestService(t)
	at := t0
	var rests []time.Duration
	for range 6 {
		favoritesPlaying(t, s, "spotify:track:a", at)
		skipAt := at.Add(20 * time.Second)
		s.MarkSkip()
		s.observe(state("spotify:track:other", 0, true), skipAt)
		rests = append(rests, s.fav.Resting["spotify:track:a"].Until.Sub(skipAt))
		at = at.Add(time.Hour)
	}

	want := []time.Duration{12 * time.Hour, day, 2 * day, 4 * day, 8 * day, maxRest}
	if !slices.Equal(rests, want) {
		t.Errorf("rests = %v, want %v", rests, want)
	}
	if _, kept := s.fav.Kept["spotify:track:a"]; kept {
		t.Error("a skipped song was kept")
	}
	if _, dropped := s.fav.Dropped["spotify:track:a"]; dropped {
		t.Error("skipping dropped a song - it should only ever rest it")
	}
}

func TestOldSkipsAreForgotten(t *testing.T) {
	s := newTestService(t)
	s.rest("spotify:track:a", t0)
	s.rest("spotify:track:a", t0.Add(30*day))
	if got := s.fav.Resting["spotify:track:a"].Until.Sub(t0.Add(30 * day)); got != baseRest {
		t.Errorf("a skip a month after the last rested it %v, want a fresh %v", got, baseRest)
	}
}

func TestPressingNextPastHalfwayIsStillASkip(t *testing.T) {
	s := newTestService(t)
	favoritesPlaying(t, s, "spotify:track:a", t0)
	at := play(s, "spotify:track:a", t0, 10, 130)
	s.MarkSkip()
	s.observe(state("spotify:track:other", 0, true), at.Add(time.Second))

	if !s.fav.Resting["spotify:track:a"].Until.After(at) {
		t.Error("pressing next at 65% didn't rest the song")
	}
}

func TestDropping(t *testing.T) {
	s := newTestService(t)
	favoritesPlaying(t, s, "spotify:track:a", t0)
	s.fav.Kept["spotify:track:a"] = t0

	info, err := s.dropCurrent(t0.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "spotify:track:a" {
		t.Errorf("dropped %q, want the playing song", info.Name)
	}
	s.observe(state("spotify:track:b", 0, true), t0.Add(2*time.Second))

	if _, kept := s.fav.Kept["spotify:track:a"]; kept {
		t.Error("a dropped song is still kept")
	}
	if _, resting := s.fav.Resting["spotify:track:a"]; resting {
		t.Error("the skip that followed a drop rested the song as well")
	}
	view := s.favorites(playback.RangeShort, ranking("spotify:track:a"), t0)
	if len(view.Pool) != 0 || len(view.Dropped) != 1 || view.Dropped[0].Name == "" {
		t.Errorf("view = %+v, want an empty pool and one named dropped song", view)
	}

	s.Undrop("spotify:track:a")
	if pool, _ := s.pool(playback.RangeShort, ranking("spotify:track:a"), t0); len(pool) != 1 {
		t.Error("undropping didn't bring the song back")
	}
}

func TestDroppingNeedsFavoritesMode(t *testing.T) {
	s := newTestService(t)
	s.observe(state("spotify:track:a", 0, true), t0)
	if _, err := s.dropCurrent(t0); !errors.Is(err, ErrNotFavorites) {
		t.Errorf("got %v, want ErrNotFavorites", err)
	}
}

func TestPlayingSomethingElseEndsTheMode(t *testing.T) {
	s := newTestService(t)
	for _, uri := range []string{"spotify:track:b", "spotify:track:c", "spotify:track:d"} {
		s.fav.Kept[uri] = t0
	}
	batch, err := s.startFavorites(playback.RangeShort, ranking("spotify:track:a"), t0)
	if err != nil {
		t.Fatal(err)
	}
	// Early in the batch, so it can't be mistaken for having played out.
	s.observe(state(batch[0], 0, true), t0.Add(10*time.Second))

	if got := s.observe(state("spotify:track:elsewhere", 0, true), t0.Add(20*time.Second)); got != ModeEnded {
		t.Fatalf("got %v, want ModeEnded", got)
	}
	if s.modeStatus().Active {
		t.Error("the mode is still on")
	}
}

func TestStaleReadsRightAfterStartingDontEndTheMode(t *testing.T) {
	s := newTestService(t)
	s.fav.Kept["spotify:track:b"] = t0
	if _, err := s.startFavorites(playback.RangeShort, ranking("spotify:track:a"), t0); err != nil {
		t.Fatal(err)
	}
	if got := s.observe(state("spotify:track:before", 0, true), t0.Add(time.Second)); got != NoChange {
		t.Errorf("got %v right after starting, want NoChange", got)
	}
	if !s.modeStatus().Active {
		t.Error("a stale read switched the mode off")
	}
}

func TestReachingTheEndOfABatchAsksForTheNext(t *testing.T) {
	s := newTestService(t)
	favoritesPlaying(t, s, "spotify:track:a", t0)
	at := play(s, "spotify:track:a", t0, 10, 190)

	if got := s.observe(state("spotify:track:autoplay", 0, true), at.Add(10*time.Second)); got != BatchFinished {
		t.Fatalf("got %v, want BatchFinished", got)
	}
	if !s.modeStatus().Active {
		t.Error("finishing a batch switched the mode off")
	}
	if _, ok := s.nextBatch(at.Add(11 * time.Second)); !ok {
		t.Error("no next batch")
	}
}

func TestStoppingTheModeStopsCountingSkips(t *testing.T) {
	s := newTestService(t)
	favoritesPlaying(t, s, "spotify:track:a", t0)
	s.StopFavorites()
	s.observe(state("spotify:track:b", 0, true), t0.Add(20*time.Second))

	if _, resting := s.fav.Resting["spotify:track:a"]; resting {
		t.Error("a skip after leaving favorites mode rested the song")
	}
}

func TestFavoritesSurviveReopening(t *testing.T) {
	s := newTestService(t)
	favoritesPlaying(t, s, "spotify:track:a", t0)
	s.MarkSkip()
	s.observe(state("spotify:track:b", 0, true), t0.Add(20*time.Second))

	reopened := open(s.pathFor, t0)
	if _, resting := reopened.fav.Resting["spotify:track:a"]; !resting {
		t.Error("a rest didn't survive reopening")
	}
}

// twoSongBatch starts favorites mode on a batch of a then b, with a
// playing.
func twoSongBatch(t *testing.T, s *Service) {
	t.Helper()
	s.fav.Kept["spotify:track:a"] = t0
	s.fav.Kept["spotify:track:b"] = t0
	if _, err := s.startFavorites(playback.RangeShort, nil, t0); err != nil {
		t.Fatal(err)
	}
	s.mode.batch = []string{"spotify:track:a", "spotify:track:b"}
	s.observe(state("spotify:track:a", 0, true), t0)
}

func TestSkippingInSpotifyToTheNextFavoriteCounts(t *testing.T) {
	s := newTestService(t)
	twoSongBatch(t, s)
	s.observe(state("spotify:track:b", 0, true), t0.Add(20*time.Second))

	if _, resting := s.fav.Resting["spotify:track:a"]; !resting {
		t.Error("moving on to the next favorite early didn't rest the song")
	}
}

func TestPuttingOnSomethingElseIsNotASkip(t *testing.T) {
	s := newTestService(t)
	twoSongBatch(t, s)
	at := play(s, "spotify:track:a", t0, 10, 40)
	s.observe(state("spotify:track:playlist", 0, true), at.Add(time.Second))

	if _, resting := s.fav.Resting["spotify:track:a"]; resting {
		t.Error("switching to a playlist rested the song")
	}
	for _, p := range s.log.Plays {
		if p.Skipped {
			t.Errorf("switching away logged %+v as a skip, which would count against the song", p)
		}
	}
}

func TestGoingBackIsNotASkip(t *testing.T) {
	s := newTestService(t)
	twoSongBatch(t, s)
	s.observe(state("spotify:track:b", 0, true), t0.Add(3*time.Minute+30*time.Second))
	s.MarkBack()
	s.observe(state("spotify:track:a", 0, true), t0.Add(4*time.Minute))

	if _, resting := s.fav.Resting["spotify:track:b"]; resting {
		t.Error("pressing previous rested the song it left")
	}
}

func TestStartingANewBatchIsNotASkip(t *testing.T) {
	s := newTestService(t)
	twoSongBatch(t, s)
	// Out of the next batch, so moving from it to that batch's first
	// song would otherwise look just like skipping it.
	delete(s.fav.Kept, "spotify:track:a")
	s.fav.Kept["spotify:track:c"] = t0
	batch, _ := s.startFavorites(playback.RangeShort, nil, t0.Add(10*time.Second))
	s.observe(state(batch[0], 0, true), t0.Add(20*time.Second))

	if _, resting := s.fav.Resting["spotify:track:a"]; resting {
		t.Error("starting favorites again rested the song it replaced")
	}
}

package stats

import (
	"path/filepath"
	"testing"
	"time"

	"spotmini-gui/internal/playback"
)

var t0 = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	return open(func(name string) (string, error) { return filepath.Join(dir, name), nil }, t0)
}

func state(uri string, progress time.Duration, playing bool) playback.PlaybackState {
	return playback.PlaybackState{
		IsPlaying:  playing,
		ProgressMs: int(progress.Milliseconds()),
		Item: playback.Track{
			URI:        uri,
			Name:       uri,
			DurationMs: int((200 * time.Second).Milliseconds()),
			Artists:    []playback.Artist{{Name: "artist"}},
		},
	}
}

// play feeds reads every ten seconds, as the bar does, from `from` to
// `to` seconds into uri, starting at `at`. Returns the time of the last
// read.
func play(s *Service, uri string, at time.Time, from, to int) time.Time {
	for p := from; p <= to; p += 10 {
		at = at.Add(10 * time.Second)
		s.observe(state(uri, time.Duration(p)*time.Second, true), at)
	}
	return at
}

func TestAFullListenIsAPlay(t *testing.T) {
	s := newTestService(t)
	at := play(s, "spotify:track:a", t0, 0, 190)
	s.observe(state("spotify:track:b", 5*time.Second, true), at.Add(10*time.Second))

	if len(s.log.Plays) != 1 {
		t.Fatalf("got %d plays, want 1", len(s.log.Plays))
	}
	p := s.log.Plays[0]
	if p.URI != "spotify:track:a" || p.Skipped {
		t.Errorf("got %+v, want an unskipped play of a", p)
	}
	if p.ListenedMs < 195_000 {
		t.Errorf("listened %dms, want close to the whole 200s", p.ListenedMs)
	}
	if s.log.Tracks["spotify:track:a"].Artist != "artist" {
		t.Error("track info wasn't recorded alongside the play")
	}
}

func TestLeavingEarlyIsASkip(t *testing.T) {
	s := newTestService(t)
	at := play(s, "spotify:track:a", t0, 0, 60)
	s.observe(state("spotify:track:b", 2*time.Second, true), at.Add(10*time.Second))

	if len(s.log.Plays) != 1 || !s.log.Plays[0].Skipped {
		t.Fatalf("got %+v, want one skipped play", s.log.Plays)
	}
}

func TestUnderThirtySecondsIsNotAPlay(t *testing.T) {
	s := newTestService(t)
	at := play(s, "spotify:track:a", t0, 0, 10)
	s.observe(state("spotify:track:b", 0, true), at.Add(5*time.Second))

	if len(s.log.Plays) != 0 {
		t.Fatalf("got %+v, want nothing recorded", s.log.Plays)
	}
}

func TestAFavoritesSkipIsKeptEvenWhenShort(t *testing.T) {
	s := newTestService(t)
	at := play(s, "spotify:track:a", t0, 0, 10)
	s.current.inFavorites = true
	s.current.explicitSkip = true
	s.observe(state("spotify:track:b", 0, true), at.Add(5*time.Second))

	if len(s.log.Plays) != 1 || !s.log.Plays[0].Skipped || !s.log.Plays[0].InFavorites {
		t.Fatalf("got %+v, want one favorites skip", s.log.Plays)
	}
}

func TestStoppingIsNotASkip(t *testing.T) {
	s := newTestService(t)
	at := play(s, "spotify:track:a", t0, 0, 60)
	s.observe(playback.PlaybackState{}, at.Add(10*time.Second))

	if len(s.log.Plays) != 1 || s.log.Plays[0].Skipped {
		t.Fatalf("got %+v, want one unskipped play", s.log.Plays)
	}
	if s.current != nil {
		t.Error("still following a track after playback stopped")
	}
}

func TestExplicitSkipCountsPastHalfway(t *testing.T) {
	s := newTestService(t)
	at := play(s, "spotify:track:a", t0, 0, 130)
	s.current.explicitSkip = true
	s.observe(state("spotify:track:b", 0, true), at.Add(time.Second))

	if len(s.log.Plays) != 1 || !s.log.Plays[0].Skipped {
		t.Fatalf("got %+v, want a skip at 65%% after pressing next", s.log.Plays)
	}
}

// Repeat-one is how a song gets played twenty times in a week, so every
// loop has to count.
func TestRepeatOneCountsEveryLoop(t *testing.T) {
	s := newTestService(t)
	at := play(s, "spotify:track:a", t0, 0, 190)
	at = play(s, "spotify:track:a", at, 5, 195)
	play(s, "spotify:track:a", at, 5, 100)

	if len(s.log.Plays) != 2 {
		t.Fatalf("got %d plays, want 2 finished loops", len(s.log.Plays))
	}
	if s.log.Plays[1].At.Sub(s.log.Plays[0].At) < 150*time.Second {
		t.Errorf("loops started %s apart, want about a track's length", s.log.Plays[1].At.Sub(s.log.Plays[0].At))
	}
}

func TestSeekingBackEarlyIsTheSameListen(t *testing.T) {
	s := newTestService(t)
	at := play(s, "spotify:track:a", t0, 0, 60)
	at = play(s, "spotify:track:a", at, 5, 190)
	s.observe(state("spotify:track:b", 0, true), at.Add(10*time.Second))

	if len(s.log.Plays) != 1 {
		t.Fatalf("got %d plays, want 1", len(s.log.Plays))
	}
}

func TestPodcastsAreIgnored(t *testing.T) {
	s := newTestService(t)
	at := play(s, "spotify:episode:x", t0, 0, 600)
	s.observe(state("spotify:track:b", 0, true), at.Add(10*time.Second))

	if len(s.log.Plays) != 0 {
		t.Fatalf("got %+v, want nothing recorded for an episode", s.log.Plays)
	}
}

func TestHistorySurvivesReopening(t *testing.T) {
	dir := t.TempDir()
	pathFor := func(name string) (string, error) { return filepath.Join(dir, name), nil }

	s := open(pathFor, t0)
	at := play(s, "spotify:track:a", t0, 0, 190)
	s.observe(playback.PlaybackState{}, at.Add(10*time.Second))

	reopened := open(pathFor, t0.Add(time.Hour))
	if len(reopened.log.Plays) != 1 {
		t.Fatalf("got %d plays after reopening, want 1", len(reopened.log.Plays))
	}
	if !reopened.log.Started.Equal(t0) {
		t.Errorf("started = %v, want the original %v", reopened.log.Started, t0)
	}
}

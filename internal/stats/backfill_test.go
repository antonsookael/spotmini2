package stats

import (
	"testing"
	"time"

	"spotmini-gui/internal/playback"
)

func recent(uri string, at time.Time) playback.RecentPlay {
	return playback.RecentPlay{
		Track:    playback.TrackResult{URI: uri, Name: uri, DurationMs: 200_000},
		PlayedAt: at,
	}
}

func TestMergeAddsUnseenPlays(t *testing.T) {
	s := newTestService(t)
	s.mergeRecent([]playback.RecentPlay{
		recent("spotify:track:b", t0.Add(-time.Hour)),
		recent("spotify:track:a", t0.Add(-2*time.Hour)),
	})

	if len(s.log.Plays) != 2 {
		t.Fatalf("got %d plays, want 2", len(s.log.Plays))
	}
	if s.log.Plays[0].URI != "spotify:track:a" {
		t.Error("plays aren't kept oldest first")
	}
	if !s.log.Plays[0].Backfilled {
		t.Error("merged play not marked as backfilled")
	}
	if !s.log.RecentCursor.Equal(t0.Add(-time.Hour)) {
		t.Errorf("cursor = %v, want the newest entry", s.log.RecentCursor)
	}
}

func TestMergeSkipsPlaysAlreadySeenLive(t *testing.T) {
	s := newTestService(t)
	at := play(s, "spotify:track:a", t0, 0, 190)
	s.observe(playback.PlaybackState{}, at.Add(10*time.Second))

	// Stamped at the end of the listen rather than the start.
	s.mergeRecent([]playback.RecentPlay{recent("spotify:track:a", at)})

	if len(s.log.Plays) != 1 {
		t.Fatalf("got %d plays, want the live one only", len(s.log.Plays))
	}
}

func TestMergeCountsEachRepeatOnce(t *testing.T) {
	s := newTestService(t)
	at := play(s, "spotify:track:a", t0, 0, 190)
	s.observe(playback.PlaybackState{}, at.Add(10*time.Second))

	// Two history entries, one live play: the second was missed.
	s.mergeRecent([]playback.RecentPlay{
		recent("spotify:track:a", at.Add(200*time.Second)),
		recent("spotify:track:a", at),
	})

	if len(s.log.Plays) != 2 {
		t.Fatalf("got %d plays, want 2", len(s.log.Plays))
	}
}

func TestMergeLeavesTheCurrentListenToBeRecordedLive(t *testing.T) {
	s := newTestService(t)
	at := play(s, "spotify:track:a", t0, 0, 60)
	s.mergeRecent([]playback.RecentPlay{recent("spotify:track:a", at)})

	if len(s.log.Plays) != 0 {
		t.Fatalf("got %+v, want nothing until the listen ends", s.log.Plays)
	}
}

func TestMergeIgnoresEntriesBeforeTheCursor(t *testing.T) {
	s := newTestService(t)
	s.mergeRecent([]playback.RecentPlay{recent("spotify:track:a", t0)})
	s.mergeRecent([]playback.RecentPlay{recent("spotify:track:a", t0)})

	if len(s.log.Plays) != 1 {
		t.Fatalf("got %d plays, want the entry merged once", len(s.log.Plays))
	}
}

package stats

import (
	"math"
	"testing"
	"time"

	"spotmini-gui/internal/playback"
)

// addPlays logs n full plays of uri spread evenly over the `over` before now.
func addPlays(s *Service, uri string, n int, over time.Duration, now time.Time) {
	for i := range n {
		at := now.Add(-over * time.Duration(i+1) / time.Duration(n+1))
		s.log.Plays = append(s.log.Plays, Play{URI: uri, At: at, ListenedMs: 180_000})
	}
	s.log.Tracks[uri] = TrackInfo{Name: uri, Artist: "artist " + uri, DurationMs: 200_000}
}

// Twenty plays this week has to beat thirty spread over six months -
// unless you've asked for all time, where volume is the point.
func TestRecentListeningOutweighsOld(t *testing.T) {
	now := t0.Add(365 * day)
	s := newTestService(t)
	s.log.Started = t0
	addPlays(s, "spotify:track:week", 20, 7*day, now)
	addPlays(s, "spotify:track:halfyear", 30, 180*day, now)

	for _, r := range []string{playback.RangeShort, playback.RangeMedium} {
		sc := s.scores(r, nil, now)
		if sc["spotify:track:week"] <= sc["spotify:track:halfyear"] {
			t.Errorf("%s: week %.1f should beat half-year %.1f", r, sc["spotify:track:week"], sc["spotify:track:halfyear"])
		}
	}
	sc := s.scores(playback.RangeLong, nil, now)
	if sc["spotify:track:halfyear"] <= sc["spotify:track:week"] {
		t.Errorf("all time: half-year %.1f should beat week %.1f", sc["spotify:track:halfyear"], sc["spotify:track:week"])
	}
}

func TestPlaysOutsideTheRangeDontCount(t *testing.T) {
	s := newTestService(t)
	s.log.Plays = []Play{{URI: "spotify:track:a", At: t0.Add(-60 * day), ListenedMs: 180_000}}

	if sc := s.scores(playback.RangeShort, nil, t0); sc["spotify:track:a"] != 0 {
		t.Errorf("a play two months ago scored %.2f in the 4-week range", sc["spotify:track:a"])
	}
	if sc := s.scores(playback.RangeMedium, nil, t0); sc["spotify:track:a"] == 0 {
		t.Error("a play two months ago didn't count in the 6-month range")
	}
}

func TestRankingFadesAsHistoryBuildsUp(t *testing.T) {
	s := newTestService(t)
	ranking := []string{"spotify:track:top", "spotify:track:second"}

	s.log.Started = t0
	fresh := s.scores(playback.RangeShort, ranking, t0)
	if fresh["spotify:track:top"] != rankPriorPlays {
		t.Errorf("day one: top of the ranking scored %.2f, want the full %.1f", fresh["spotify:track:top"], rankPriorPlays)
	}
	if fresh["spotify:track:second"] >= fresh["spotify:track:top"] {
		t.Error("lower in the ranking should score lower")
	}

	s.log.Started = t0.Add(-60 * day)
	seasoned := s.scores(playback.RangeShort, ranking, t0)
	want := minPriorShare * rankPriorPlays
	if math.Abs(seasoned["spotify:track:top"]-want) > 1e-9 {
		t.Errorf("with a full range of history: top scored %.2f, want %.2f", seasoned["spotify:track:top"], want)
	}
}

func TestFavoritesSkipsNudgeDownAndWearOff(t *testing.T) {
	s := newTestService(t)
	addPlays(s, "spotify:track:a", 5, 7*day, t0)
	before := s.scores(playback.RangeShort, nil, t0)["spotify:track:a"]

	skip := Play{URI: "spotify:track:a", At: t0.Add(-time.Hour), ListenedMs: 10_000, Skipped: true, InFavorites: true}
	s.log.Plays = append(s.log.Plays, skip)
	after := s.scores(playback.RangeShort, nil, t0)["spotify:track:a"]
	if after >= before {
		t.Errorf("a skip left the score at %.2f, was %.2f", after, before)
	}
	if before-after > 0.5 {
		t.Errorf("a single skip cost %.2f - meant to be a nudge", before-after)
	}

	// A skip outside favorites mode says nothing about the song.
	s.log.Plays[len(s.log.Plays)-1].InFavorites = false
	if got := s.scores(playback.RangeShort, nil, t0)["spotify:track:a"]; got != before {
		t.Errorf("a short skip outside favorites changed the score to %.2f from %.2f", got, before)
	}
}

func TestLocalStats(t *testing.T) {
	now := t0.Add(60 * day)
	s := newTestService(t)
	s.log.Started = t0
	addPlays(s, "spotify:track:steady", 12, 28*day, now.Add(-7*day))
	addPlays(s, "spotify:track:new", 6, 6*day, now)
	s.log.Plays = append(s.log.Plays, Play{URI: "spotify:track:new", At: now.Add(-time.Hour), ListenedMs: 5_000})
	s.log.Tracks["spotify:track:new"] = TrackInfo{Name: "new", Artist: "shared"}
	s.log.Tracks["spotify:track:steady"] = TrackInfo{Name: "steady", Artist: "shared"}

	st := s.localStats(playback.RangeMedium, now, time.UTC)

	if st.Plays != 18 {
		t.Errorf("plays = %d, want 18 (the 5-second listen doesn't count)", st.Plays)
	}
	if len(st.Rising) != 1 || st.Rising[0].URI != "spotify:track:new" {
		t.Errorf("rising = %+v, want just the new song", st.Rising)
	}
	if st.Rising[0].PlaysThisWeek != 6 {
		t.Errorf("plays this week = %d, want 6", st.Rising[0].PlaysThisWeek)
	}
	if len(st.Artists) != 1 || st.Artists[0].Plays != 18 {
		t.Errorf("artists = %+v, want one artist with 18 plays", st.Artists)
	}
	if len(st.Days) != chartDays || st.Days[chartDays-1].Day != now.Format(time.DateOnly) {
		t.Errorf("chart = %+v, want %d days ending today", st.Days, chartDays)
	}
	total := 0
	for _, d := range st.Days {
		total += d.Minutes
	}
	if total == 0 {
		t.Error("the chart shows no listening over a week of plays")
	}
	if !st.Since.Equal(t0) {
		t.Errorf("since = %v, want %v", st.Since, t0)
	}
}

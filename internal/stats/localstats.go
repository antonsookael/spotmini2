package stats

import (
	"cmp"
	"slices"
	"time"
)

const (
	statsTrackLimit  = 30
	statsArtistLimit = 20
	risingLimit      = 10
	risingLookback   = 7 * day
	// A song has to have gained at least about a play's worth to be
	// rising, or every single new listen would qualify.
	minRise   = 1.0
	chartDays = 14
)

type TrackStat struct {
	URI           string  `json:"uri"`
	Name          string  `json:"name"`
	Artist        string  `json:"artist"`
	Plays         int     `json:"plays"`
	PlaysThisWeek int     `json:"plays_this_week"`
	Score         float64 `json:"score"`
	Rise          float64 `json:"rise"`
}

type ArtistStat struct {
	Name    string `json:"name"`
	Plays   int    `json:"plays"`
	Minutes int    `json:"minutes"`
}

type DayMinutes struct {
	Day     string `json:"day"` // YYYY-MM-DD, local time
	Minutes int    `json:"minutes"`
}

// LocalStats is what the app's own log says about a time range.
type LocalStats struct {
	Tracks  []TrackStat  `json:"tracks"`
	Rising  []TrackStat  `json:"rising"`
	Artists []ArtistStat `json:"artists"`
	// The last two weeks, oldest first, whatever the range.
	Days    []DayMinutes `json:"days"`
	Plays   int          `json:"plays"`
	Minutes int          `json:"minutes"`
	// How far back the log goes.
	Since time.Time `json:"since"`
}

func (s *Service) LocalStats(timeRange string) LocalStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.localStats(timeRange, time.Now(), time.Local)
}

// Caller holds mu.
func (s *Service) localStats(timeRange string, now time.Time, loc *time.Location) LocalStats {
	sp := spans[timeRange]
	out := LocalStats{Since: s.log.Started}

	tracks := make(map[string]*TrackStat)
	artists := make(map[string]*ArtistStat)
	minutesByDay := make(map[string]int)
	chartStart := startOfDay(now.In(loc)).AddDate(0, 0, -(chartDays - 1))

	for _, p := range s.log.Plays {
		if p.At.Before(out.Since) {
			out.Since = p.At
		}
		if !p.At.Before(chartStart) {
			minutesByDay[p.At.In(loc).Format(time.DateOnly)] += p.ListenedMs
		}

		age := now.Sub(p.At)
		if age < 0 || (sp.window > 0 && age > sp.window) || p.ListenedMs < minPlayMs {
			continue
		}
		info := s.log.Tracks[p.URI]

		t := tracks[p.URI]
		if t == nil {
			t = &TrackStat{URI: p.URI, Name: info.Name, Artist: info.Artist}
			tracks[p.URI] = t
		}
		t.Plays++
		if age <= risingLookback {
			t.PlaysThisWeek++
		}

		if info.Artist != "" {
			a := artists[info.Artist]
			if a == nil {
				a = &ArtistStat{Name: info.Artist}
				artists[info.Artist] = a
			}
			a.Plays++
			a.Minutes += p.ListenedMs
		}

		out.Plays++
		out.Minutes += p.ListenedMs
	}
	out.Minutes /= 60_000

	current := s.localScores(sp, now)
	before := s.localScores(sp, now.Add(-risingLookback))
	for uri, t := range tracks {
		t.Score = current[uri]
		t.Rise = current[uri] - before[uri]
	}

	all := make([]TrackStat, 0, len(tracks))
	for _, t := range tracks {
		all = append(all, *t)
	}
	slices.SortFunc(all, func(a, b TrackStat) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), cmp.Compare(a.URI, b.URI))
	})
	out.Tracks = all[:min(len(all), statsTrackLimit)]

	for _, t := range all {
		if t.Rise >= minRise {
			out.Rising = append(out.Rising, t)
		}
	}
	slices.SortFunc(out.Rising, func(a, b TrackStat) int {
		return cmp.Or(cmp.Compare(b.Rise, a.Rise), cmp.Compare(a.URI, b.URI))
	})
	out.Rising = out.Rising[:min(len(out.Rising), risingLimit)]

	for _, a := range artists {
		a.Minutes /= 60_000
		out.Artists = append(out.Artists, *a)
	}
	slices.SortFunc(out.Artists, func(a, b ArtistStat) int {
		return cmp.Or(cmp.Compare(b.Plays, a.Plays), cmp.Compare(a.Name, b.Name))
	})
	out.Artists = out.Artists[:min(len(out.Artists), statsArtistLimit)]

	for i := range chartDays {
		d := chartStart.AddDate(0, 0, i).Format(time.DateOnly)
		out.Days = append(out.Days, DayMinutes{Day: d, Minutes: minutesByDay[d] / 60_000})
	}
	return out
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

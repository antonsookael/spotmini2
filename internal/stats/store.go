// Package stats keeps spotmini's own listening history. Spotify only
// ever says which tracks rank highest, never how often or how recently
// they were played, so the counts behind the stats tab and favorites
// mode are recorded here from what the app itself sees playing.
package stats

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"sync"
	"time"

	"spotmini-gui/internal/logging"
	"spotmini-gui/internal/paths"
)

const playsFile = "plays.json"

type TrackInfo struct {
	Name       string `json:"name"`
	Artist     string `json:"artist"`
	DurationMs int    `json:"duration_ms"`
}

// Play is one listen of a track.
type Play struct {
	URI string `json:"uri"`
	// When the listen started, for plays seen live. For backfilled ones
	// it's Spotify's played_at, which is within a track's length of that.
	At         time.Time `json:"at"`
	ListenedMs int       `json:"listened_ms"`
	Skipped    bool      `json:"skipped,omitempty"`
	// Played by favorites mode, the only place a skip counts against a
	// song - skipping through someone else's playlist says nothing about
	// how much you like what's in it.
	InFavorites bool `json:"in_favorites,omitempty"`
	// Recovered from Spotify's history rather than seen live - played
	// while the app was closed, or on another device.
	Backfilled bool `json:"backfilled,omitempty"`
}

type playLog struct {
	Started time.Time            `json:"started"`
	Tracks  map[string]TrackInfo `json:"tracks"`
	// Oldest first.
	Plays []Play `json:"plays"`
	// The newest played_at already merged from Spotify's history.
	RecentCursor time.Time `json:"recent_cursor"`
}

type Service struct {
	mu      sync.Mutex
	log     playLog
	fav     favState
	current *listen
	// nil while favorites mode is off.
	mode *mode

	rng     *rand.Rand
	pathFor func(name string) (string, error)
}

// Open loads the saved history, starting a fresh one if there isn't any.
func Open() *Service {
	return open(paths.ConfigFile, time.Now())
}

func open(pathFor func(string) (string, error), now time.Time) *Service {
	s := &Service{
		pathFor: pathFor,
		rng:     rand.New(rand.NewPCG(uint64(now.UnixNano()), 0)),
	}
	if err := s.load(playsFile, &s.log); err != nil {
		logging.Printf("Could not load listening history: %v", err)
	}
	if s.log.Tracks == nil {
		s.log.Tracks = make(map[string]TrackInfo)
	}
	if s.log.Started.IsZero() {
		s.log.Started = now
	}

	if err := s.load(favoritesFile, &s.fav); err != nil {
		logging.Printf("Could not load favorites: %v", err)
	}
	if s.fav.Kept == nil {
		s.fav.Kept = make(map[string]time.Time)
	}
	if s.fav.Dropped == nil {
		s.fav.Dropped = make(map[string]time.Time)
	}
	if s.fav.Resting == nil {
		s.fav.Resting = make(map[string]rest)
	}
	return s
}

// load reads name into v. A missing file leaves v alone. An unreadable
// one is moved aside rather than left to be overwritten by the next
// save, which would throw away history that might still be recoverable.
func (s *Service) load(name string, v any) error {
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		aside := fmt.Sprintf("%s.broken-%d", path, time.Now().Unix())
		if renameErr := os.Rename(path, aside); renameErr != nil {
			return fmt.Errorf("parsing %s: %w (and could not move it aside: %v)", name, err, renameErr)
		}
		return fmt.Errorf("parsing %s: %w (moved to %s)", name, err, aside)
	}
	return nil
}

// save writes v to name through a temporary file, so a crash mid-write
// leaves the previous version rather than half of this one.
func (s *Service) save(name string, v any) {
	path, err := s.pathFor(name)
	if err != nil {
		logging.Printf("Could not resolve %s: %v", name, err)
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		logging.Printf("Could not encode %s: %v", name, err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		logging.Printf("Could not write %s: %v", name, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		logging.Printf("Could not replace %s: %v", name, err)
	}
}

// Caller holds mu.
func (s *Service) saveLog() {
	s.save(playsFile, &s.log)
}

// RecentCursor is the point Spotify's history has been merged up to.
func (s *Service) RecentCursor() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.log.RecentCursor
}

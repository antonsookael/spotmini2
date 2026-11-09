package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"spotmini-gui/internal/hotkeys"
	"spotmini-gui/internal/logging"
	"spotmini-gui/internal/playback"
)

// The Linux build talks to the desktop over a Unix socket: on the
// user's own setup the now-playing strip is drawn by the Touch Bar
// daemon and the top bar's status island rather than by this window,
// and the panels open from buttons there. Each line on the socket is
// one JSON object.
//
//	server -> client  {"type":"state", ...ipcState}   on connect and after every playback read
//	client -> server  {"cmd":"playPause"}             any name in hotkeys.Actions
//
// Progress is sent as it was at the moment of the read ("at", Unix
// milliseconds), the same way the frontend gets it: the client counts
// the seconds up itself in between, so nothing here has to poll Spotify
// any faster for the sake of a progress line.

// ipcState is what a client is told about playback.
type ipcState struct {
	Type string `json:"type"`
	// At is when the read happened, Unix milliseconds, for counting
	// ProgressMs forward from.
	At int64 `json:"at"`
	// SignedIn is false while the launch sign-in is still running, or
	// when the last read failed; Error then says why in the words the
	// bar itself would use.
	SignedIn bool   `json:"signed_in"`
	Error    string `json:"error,omitempty"`
	// Playing is false both when paused and when nothing is on; an
	// empty URI is what says nothing is on.
	Playing    bool   `json:"playing"`
	URI        string `json:"uri"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	ProgressMs int    `json:"progress_ms"`
	DurationMs int    `json:"duration_ms"`
	Shuffle    bool   `json:"shuffle"`
	// Repeat is Spotify's own "off", "context" or "track".
	Repeat string `json:"repeat"`
}

type ipcCommand struct {
	Cmd string `json:"cmd"`
}

// ipcStaleAfter is how long the server lets the state go without a
// fresh read before reading Spotify itself, while someone is listening.
// The frontend's own polling normally keeps it fresher than this; the
// fallback is for a hidden or throttled window, so a bar never shows a
// song that ended minutes ago.
const ipcStaleAfter = 12 * time.Second

type ipcServer struct {
	path     string
	listener net.Listener

	mu      sync.Mutex
	clients map[net.Conn]struct{}
	last    ipcState
	lastAt  time.Time
	hasLast bool
}

// ipcSocketPath is $XDG_RUNTIME_DIR/spotmini.sock - per user, cleared at
// logout - with the temp dir as the fallback for a session without one.
func ipcSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "spotmini.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("spotmini-%d.sock", os.Getuid()))
}

// startIPC opens the socket and serves it until stopIPC. A socket file
// left by a crashed run is replaced; one another running spotmini is
// answering on is left alone, since taking it over would cut that one's
// clients off.
func (a *App) startIPC() {
	path := ipcSocketPath()

	if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
		conn.Close()
		logging.Printf("Another spotmini is already serving %s - not taking it over", path)
		return
	}
	// Stale, or never existed. Either way the listen below needs it gone.
	os.Remove(path)

	listener, err := net.Listen("unix", path)
	if err != nil {
		logging.Printf("Could not open the control socket at %s: %v", path, err)
		return
	}
	// Owner only: the socket takes playback commands.
	os.Chmod(path, 0o600)

	a.ipc = &ipcServer{
		path:     path,
		listener: listener,
		clients:  make(map[net.Conn]struct{}),
	}
	fmt.Printf("Control socket at %s\n", path)

	go a.ipcAccept()
	go a.ipcKeepFresh()
}

// stopIPC closes every connection and removes the socket file, so a
// client doesn't sit on a dead socket after the app has quit.
func (a *App) stopIPC() {
	s := a.ipc
	if s == nil {
		return
	}
	s.listener.Close()
	s.mu.Lock()
	for c := range s.clients {
		c.Close()
	}
	s.clients = nil
	s.mu.Unlock()
	os.Remove(s.path)
}

func (a *App) ipcAccept() {
	s := a.ipc
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// Closed by stopIPC, or broken for good - either way this
			// loop is over.
			return
		}
		s.mu.Lock()
		if s.clients == nil {
			s.mu.Unlock()
			conn.Close()
			return
		}
		s.clients[conn] = struct{}{}
		last, has := s.last, s.hasLast
		s.mu.Unlock()

		// A new client gets the latest state at once rather than waiting
		// for the next read, so a bar coming up shows the song straight
		// away.
		if has {
			ipcSend(conn, last)
		}
		go a.ipcServe(conn)
	}
}

// ipcServe reads commands from one client until it hangs up.
func (a *App) ipcServe(conn net.Conn) {
	defer func() {
		s := a.ipc
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var cmd ipcCommand
		if err := json.Unmarshal(line, &cmd); err != nil {
			logging.Printf("Control socket: unreadable command %q: %v", line, err)
			continue
		}
		if !slices.Contains(hotkeys.Actions, cmd.Cmd) {
			logging.Printf("Control socket: unknown command %q", cmd.Cmd)
			continue
		}
		// The same functions the hotkeys run, so a tap on the bar and a
		// keypress can never drift apart in what they do.
		fmt.Printf("[socket] %s\n", cmd.Cmd)
		a.hotkeyAction(cmd.Cmd)()
	}
}

// ipcPublish is called with the result of every playback read, and
// tells every client. A failed read is published too, as not signed in
// plus the reason: a bar showing the last song forever is worse than
// one saying Spotify can't be reached.
func (a *App) ipcPublish(state playback.PlaybackState, err error) {
	s := a.ipc
	if s == nil {
		return
	}

	now := time.Now()
	msg := ipcState{Type: "state", At: now.UnixMilli()}
	if err != nil {
		msg.Error = failureMessage(err)
	} else {
		msg.SignedIn = true
		msg.Playing = state.IsPlaying
		msg.URI = state.Item.URI
		msg.Title = state.Item.Name
		msg.Artist = ipcArtist(state.Item)
		msg.ProgressMs = state.ProgressMs
		msg.DurationMs = state.Item.DurationMs
		msg.Shuffle = state.ShuffleState
		msg.Repeat = state.RepeatState
	}

	s.mu.Lock()
	s.last, s.lastAt, s.hasLast = msg, now, true
	conns := make([]net.Conn, 0, len(s.clients))
	for c := range s.clients {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	for _, c := range conns {
		ipcSend(c, msg)
	}
}

// ipcArtist is the line under the title: the first artist, or the show
// for a podcast episode.
func ipcArtist(item playback.Track) string {
	if len(item.Artists) > 0 {
		return item.Artists[0].Name
	}
	if item.Show != nil {
		return item.Show.Name
	}
	return ""
}

// ipcSend writes one message. A failed write is left alone: the
// connection belongs to ipcServe, whose read fails too when the client
// is really gone and takes it out of the set. Closing it here instead
// lost commands - a client that sends one and hangs up straight away
// makes the welcome state's write fail, and closing on that threw its
// command away unread.
func ipcSend(conn net.Conn, msg ipcState) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	conn.Write(append(data, '\n'))
}

// ipcKeepFresh reads Spotify itself when the frontend's reads have
// stopped coming while a client is connected - see ipcStaleAfter.
func (a *App) ipcKeepFresh() {
	s := a.ipc
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		closed := s.clients == nil
		listening := len(s.clients) > 0
		stale := !s.hasLast || time.Since(s.lastAt) > ipcStaleAfter
		s.mu.Unlock()
		if closed {
			return
		}
		if !listening || !stale {
			continue
		}
		// Publishes on its own, success or failure, so the result isn't
		// needed here.
		a.GetNowPlaying()
	}
}

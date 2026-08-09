import { EventsOn, Environment, WindowGetPosition, WindowSetAlwaysOnTop, BrowserOpenURL, Quit } from '../wailsjs/runtime/runtime'
import {
  GetNowPlaying,
  GetHotkeyConfig,
  SetHotkeyBinding,
  ToggleSettingsPanel,
  TogglePlaylistsPanel,
  ToggleHotkeysPanel,
  GetPlaylists,
  PlayPlaylist,
  PlayLikedSongs,
  SearchTracks,
  PlayTrack,
  SaveTrackToLiked,
  ReportTrackChange,
  IsAutostartEnabled,
  SetAutostart,
  GetVersion,
  GetUpdateInfo,
  OpenReleasePage,
  SnapWindowToEdges,
  SetWindowWidth,
  SetPanelHeight,
  DragWindowTo,
  BeginDrag,
  EndDrag,
} from '../wailsjs/go/app/App'

const trackInfoEl = document.getElementById('track-info')
const timerEl = document.getElementById('timer')
const statusDotEl = document.getElementById('status-dot')
const shuffleIconEl = document.getElementById('shuffle-icon')
const loopIconEl = document.getElementById('loop-icon')
const loopOneBadgeEl = document.getElementById('loop-one-badge')

let currentSeconds = 0
let totalSeconds = 0
let currentSong = ''
let currentArtist = ''
let currentSpotifyURI = ''
let isCurrentlyPlaying = false
let isShuffled = false
let repeatState = 'off'
// Timestamp a notice holds the track-info slot until; read by render(),
// set by showBarMessage() further down.
let barMessageUntil = 0

// The spotify: URI scheme hands off to the desktop app, unlike an
// https:// link which always opens a browser. currentSpotifyURI is
// empty when there's nothing to link to.
trackInfoEl.addEventListener('click', () => {
  if (currentSpotifyURI) BrowserOpenURL(currentSpotifyURI)
})

// --- Customization: auto-fit window width ---
// Off by default (titles just ellipsize). On, the window widens to fit
// the track name and artist, up to MAX_WIDTH.
// DEFAULT_WIDTH matches windowWidth in internal/app/app.go: the width
// the window starts at and returns to when auto-fit is switched off.
//
// Auto-fit itself is free to go below it, down to AUTOFIT_MIN_WIDTH -
// with nothing playing the bar is just "Nothing playing" and the
// buttons, so holding it at the full default would leave a wide, mostly
// empty bar.
//
// AUTOFIT_MIN_WIDTH is above what that idle bar actually measures
// (~245px), so it's the floor rather than the content that decides how
// narrow things get - shrink-wrapping it exactly left the window
// looking cramped. Keep it at or above minWindowWidth in app.go, which
// the OS enforces.
const DEFAULT_WIDTH = 440
const AUTOFIT_MIN_WIDTH = 300
const MAX_WIDTH = 640

const autoFitToggle = document.getElementById('auto-fit-toggle')
autoFitToggle.checked = localStorage.getItem('autoFitWidth') === 'true'
autoFitToggle.addEventListener('change', (e) => {
  localStorage.setItem('autoFitWidth', e.target.checked)
  if (e.target.checked) {
    updateAutoWidth()
  } else {
    resetWidth()
  }
})

// Briefly frees #now-playing from its 100% width and #track-info from
// its ellipsis constraints to measure how wide the bar really needs to
// be, then restores both. The bar's own width has to be freed too, or
// scrollWidth can only ever report growth, never shrinkage.
function measureNeededBarWidth() {
  const bar = document.getElementById('now-playing')
  const prevBarWidth = bar.style.width
  const prevFlex = trackInfoEl.style.flex
  const prevOverflow = trackInfoEl.style.overflow

  bar.style.width = 'max-content'
  trackInfoEl.style.flex = '0 0 auto'
  trackInfoEl.style.overflow = 'visible'

  const needed = bar.scrollWidth

  bar.style.width = prevBarWidth
  trackInfoEl.style.flex = prevFlex
  trackInfoEl.style.overflow = prevOverflow

  return needed
}

// Both go through Go's SetWindowWidth rather than the Wails runtime's
// WindowSetSize: resizing anchors the top-left and grows rightward, so
// widening near a screen edge needs the monitor bounds to stay
// on-screen, and only Go can see those. It also no-ops when the width
// is already right, so there's no need to check here.
function updateAutoWidth() {
  if (!autoFitToggle.checked) return
  SetWindowWidth(Math.min(MAX_WIDTH, Math.max(AUTOFIT_MIN_WIDTH, measureNeededBarWidth() + 8)))
}

function resetWidth() {
  SetWindowWidth(DEFAULT_WIDTH)
}

function formatTime(totalSecs) {
  const minutes = Math.floor(totalSecs / 60)
  const seconds = Math.floor(totalSecs % 60)
  return `${minutes}:${seconds.toString().padStart(2, '0')}`
}

function render() {
  // A notice owns the track-info slot until it expires - otherwise the
  // once-a-second tick below would overwrite it almost immediately,
  // long before it could be read.
  if (Date.now() < barMessageUntil) return

  trackInfoEl.classList.toggle('clickable', !!currentSpotifyURI)

  if (!currentSong) {
    trackInfoEl.textContent = 'Nothing playing'
    timerEl.textContent = ''
    statusDotEl.classList.add('hidden')
    shuffleIconEl.classList.add('hidden')
    loopIconEl.classList.add('hidden')
    return
  }

  statusDotEl.classList.remove('hidden')
  statusDotEl.classList.toggle('playing', isCurrentlyPlaying)
  statusDotEl.classList.toggle('paused', !isCurrentlyPlaying)

  shuffleIconEl.classList.remove('hidden')
  shuffleIconEl.classList.toggle('active', isShuffled)

  loopIconEl.classList.remove('hidden')
  loopIconEl.classList.toggle('active', repeatState !== 'off')
  // A "1" badge distinguishes repeat-one from repeat-all.
  loopOneBadgeEl.classList.toggle('hidden', repeatState !== 'track')

  trackInfoEl.textContent = currentArtist ? `${currentSong} - ${currentArtist}` : currentSong
  timerEl.textContent =
    `${formatTime(currentSeconds)}/${formatTime(totalSeconds)}`
}

// Reads are never cancelled, only superseded: a request that's been
// overtaken while in flight has its answer thrown away. Without this the
// last response to *arrive* wins rather than the last one issued, so
// during rapid skips an early read still carrying the old track could
// land after a newer, correct one and overwrite it.
let fetchSeq = 0

// Commits a state read from Spotify to the bar. Split out from
// fetchNowPlaying so the track-change confirmation can read without
// committing - it needs to look at an answer before deciding whether
// showing it would be an improvement or a step backwards.
function applyPlaybackState(state) {
  if (!state.item || !state.item.name) {
    currentSong = ''
    currentSpotifyURI = ''
    render()
    updateAutoWidth()
    return
  }
  currentSong = state.item.name
  // A track has artists; a podcast episode has a show instead - pick
  // whichever is actually present rather than assuming it's a track.
  currentArtist = state.item.artists && state.item.artists.length > 0
    ? state.item.artists[0].name
    : state.item.show
    ? state.item.show.name
    : ''
  currentSpotifyURI = state.item.uri || ''
  currentSeconds = Math.floor(state.progress_ms / 1000)
  totalSeconds = Math.floor(state.item.duration_ms / 1000)
  isCurrentlyPlaying = state.is_playing
  isShuffled = state.shuffle_state
  repeatState = state.repeat_state || 'off'
  render()
  updateAutoWidth()
}

// True when the last read failed. Two things depend on knowing the
// difference between "Spotify says nothing is playing" and "Spotify
// didn't answer": the bar says so instead of claiming idleness, and the
// ticker below keeps its slow cadence rather than the two-second one it
// uses while genuinely idle - which, against an endpoint that just rate
// limited us, is the difference between recovering and staying stuck.
let readFailed = false

// Re-syncs the local counter with Spotify's actual state.
async function fetchNowPlaying() {
  const seq = ++fetchSeq
  try {
    const state = await GetNowPlaying()
    if (seq !== fetchSeq) return
    readFailed = false
    applyPlaybackState(state)
  } catch (err) {
    if (seq !== fetchSeq) return
    readFailed = true
    // The track state is deliberately left alone. It's the last thing
    // Spotify actually said, and a failed read is no reason to claim
    // the song stopped - it only means we can't see it right now.
    showBarMessage(String((err && err.message) || err), 4000)
  }
}

// Local ticker - counts up each second without hitting Spotify,
// re-syncing periodically.
let secondsSinceSync = 0
let idleSecondsSinceCheck = 0

// True while a pick from search is being confirmed. The resyncs below
// stand down for the duration: they commit whatever Spotify says, and
// Spotify lags its own writes, so one landing mid-confirmation would
// flick the bar back to the song that was just replaced.
//
// Declared here, above the ticker that reads it, rather than down with
// the rest of the track-change state: the ticker is registered before
// that point, so if anything ever threw in between, the callback would
// hit this in its dead zone and fail every second from then on.
let confirmingPick = false

// How often to re-check while nothing is playing. Two seconds normally,
// so starting a song elsewhere shows up promptly - but a read that
// failed backs off to the same cadence as ordinary playback, because
// the usual reason a read fails is that we asked too often.
const IDLE_POLL_SECONDS = 2
const FAILED_POLL_SECONDS = 10

setInterval(() => {
  if (!currentSong) {
    idleSecondsSinceCheck++
    const due = readFailed ? FAILED_POLL_SECONDS : IDLE_POLL_SECONDS
    if (idleSecondsSinceCheck >= due && !confirmingPick) {
      idleSecondsSinceCheck = 0
      fetchNowPlaying()
    }
    return
  }
  idleSecondsSinceCheck = 0
  secondsSinceSync++

  if (isCurrentlyPlaying) {
    currentSeconds++
    render()
  }

  // A pick is mid-confirmation and already reading Spotify on its own
  // schedule. The counter above keeps running; only the resyncs below
  // stand down, since committing a read the confirmation has
  // deliberately not committed yet is the one thing that undoes it.
  if (confirmingPick) return

  // Resync as the track runs out, to pick up whatever Spotify moved on
  // to. Gated on actually playing, and on a couple of seconds having
  // passed: the counter stays pinned at the duration once playback
  // stops at the end of a queue, and nothing resets it, so an
  // ungated check re-fetched every single second for as long as the app
  // was left sitting there.
  if (isCurrentlyPlaying && totalSeconds > 0 && currentSeconds >= totalSeconds && secondsSinceSync >= 2) {
    secondsSinceSync = 0
    fetchNowPlaying()
    return
  }

  if (secondsSinceSync >= 10) {
    secondsSinceSync = 0
    fetchNowPlaying()
  }
}, 1000)

EventsOn('logged-in', () => {
  fetchNowPlaying()
})

// How long to wait before each attempt at reading back a track change.
// Spotify's read API lags its own writes, so the first read after a skip
// usually still describes the track we skipped away from - backing off
// lets the common case land on the first attempt while still allowing
// longer overall than one fixed wait would.
const trackChangeRetryDelays = [300, 400, 700, 1100, 1600]

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

// Bumped by each new skip so an older confirmation loop stops rather
// than carrying on polling for a change that's already been overtaken.
let trackChangeAttempt = 0

// The track a search pick asked for. A skip has no idea where it will
// land, but picking a song from the results does - and knowing the
// answer in advance is what lets the bar show it immediately and lets
// the confirmation below wait for that exact track rather than merely
// for the title to change.
let pendingTrackURI = ''

// Puts a picked track in the bar right away, before Spotify has been
// asked about it. The panel closes on the same click, so the bar is the
// first thing the eye lands on - and it was still showing the previous
// song until the confirmation reads came back, which reads as the click
// having missed.
//
// Safe to show something unconfirmed here because the loop below is
// already on its way to Spotify for the truth, and replaces this with
// whatever it finds.
function showPendingTrack(track) {
  pendingTrackURI = track.uri
  currentSong = track.name
  currentArtist = track.artist || ''
  currentSpotifyURI = track.uri
  isCurrentlyPlaying = true
  currentSeconds = 0
  // A search result carries no duration, so the timer starts blank
  // rather than keeping the previous track's length, which would be
  // wrong in a way that looks deliberate. The first confirmed read
  // fills it in.
  totalSeconds = 0
  render()
  updateAutoWidth()
}

// Reads until Spotify agrees the track changed, rather than trusting
// the first answer. Gives up after the last delay - a track that
// genuinely doesn't change (a single-song context on repeat) would
// otherwise poll forever.
//
// startedAt is the keypress, handed over by Go, so the reported total
// covers the whole thing rather than just the part after the command
// came back.
async function confirmTrackChange(startedAt) {
  const attempt = ++trackChangeAttempt
  // Claimed for this run: a later command starts from no expectation
  // rather than inheriting this one's.
  const target = pendingTrackURI
  pendingTrackURI = ''
  const previousURI = currentSpotifyURI
  const elapsed = () => Math.round(Date.now() - startedAt)
  let reads = 0
  let latest = null

  confirmingPick = !!target
  try {
    return await runConfirmation()
  } finally {
    // Cleared on the way out, but only by the run that still owns it. A
    // superseded attempt returning early must leave the flag alone: the
    // newer run set it for itself and is still relying on it, and
    // clearing it here let the periodic resync fire mid-confirmation and
    // commit the previous song - the exact flick this flag prevents.
    if (target && attempt === trackChangeAttempt) confirmingPick = false
  }

  async function runConfirmation() {
  for (const delay of trackChangeRetryDelays) {
    await sleep(delay)
    // Superseded by a newer command, which will report its own total -
    // timing this one to a title that's already been replaced would
    // just be noise.
    if (attempt !== trackChangeAttempt) return

    // Without a target there's nothing to compare against but the
    // title we started from, so every read is worth showing.
    if (!target) {
      await fetchNowPlaying()
      reads++
      if (currentSpotifyURI !== previousURI) {
        ReportTrackChange(elapsed(), reads, true)
        return
      }
      continue
    }

    // With one, read without committing. Spotify lags its own writes,
    // so an early read still describes the previous song - and showing
    // it would flick the bar back to the track just replaced before
    // flicking forward again.
    //
    // The sequence is claimed anyway, so an older read still in flight
    // can't commit over the top of what this loop decides.
    fetchSeq++
    try {
      latest = await GetNowPlaying()
    } catch (err) {
      latest = null
    }
    reads++
    if (attempt !== trackChangeAttempt) return
    if (latest && latest.item && latest.item.uri === target) {
      applyPlaybackState(latest)
      ReportTrackChange(elapsed(), reads, true)
      return
    }
  }

  // Out of attempts. With a target the bar is still showing an
  // unconfirmed guess, so hand it the last thing Spotify actually said
  // rather than leaving the guess standing indefinitely.
  if (target && latest) applyPlaybackState(latest)
  ReportTrackChange(elapsed(), reads, false)
  }
}

// Emitted after any playback command, so the UI reflects it right away
// instead of waiting for the next poll. A command that lands on a
// different track says so, and that read has to be confirmed rather than
// taken at face value.
EventsOn('playback-changed', (expectTrackChange, startedAt) => {
  if (expectTrackChange) {
    confirmTrackChange(startedAt)
    return
  }
  setTimeout(fetchNowPlaying, 300)
})

// --- Toast (volume changes, brief status/error messages) ---
const toastEl = document.getElementById('toast')
let toastTimeout = null

function showToast(message, duration = 1200) {
  toastEl.textContent = message
  toastEl.classList.remove('hidden')
  toastEl.classList.add('visible')

  clearTimeout(toastTimeout)
  toastTimeout = setTimeout(() => {
    toastEl.classList.remove('visible')
  }, duration)
}

EventsOn('volume-changed', (percent) => {
  showToast(`Vol ${percent}%`)
})

// Takes over the track-info slot for a few seconds. Used where a
// command was rejected for a reason worth explaining - otherwise
// nothing happens and there's no hint why. Shown here rather than as a
// toast so it's read where the user is already looking.
let barMessageTimeout = null

function showBarMessage(text, ms) {
  barMessageUntil = Date.now() + ms
  trackInfoEl.textContent = text
  timerEl.textContent = ''
  statusDotEl.classList.add('hidden')
  shuffleIconEl.classList.add('hidden')
  loopIconEl.classList.add('hidden')

  // Resize in the same tick the text changes, before the browser gets a
  // chance to paint. Without this the window keeps whatever width the
  // last song needed - and auto-fit shrinks it right down while idle,
  // which is exactly when these messages tend to fire, so the message
  // would show up truncated and only widen later, if at all.
  updateAutoWidth()

  clearTimeout(barMessageTimeout)
  barMessageTimeout = setTimeout(() => {
    barMessageUntil = 0
    fetchNowPlaying()
  }, ms)
}

// Spotify rejects every playback command on a Free account, so without
// this a Free user sees nothing happen and no reason why.
// Kept short enough to fit the bar at its default width - the longer
// wording this replaced was silently ellipsized.
// A command that was refused never reaches confirmTrackChange, which is
// the only thing that clears the pending pick. Left set, it becomes the
// target of whatever skip comes next - which then spends the full 4s
// backoff waiting for a track Spotify was never asked to play, holding
// the bar on the old title the whole time and logging a false
// unconfirmed change.
function clearPendingTrack() {
  pendingTrackURI = ''
}

EventsOn('premium-required', () => {
  clearPendingTrack()
  showBarMessage('Spotify Premium required for playback', 3000)
})

// The login flow ended without a token - consent refused, port 8888
// already taken by another copy of the app, or the code exchange
// rejected. Nothing retries it, so the bar has to say so rather than
// sitting on "Loading..." looking hung.
EventsOn('login-failed', () => {
  showBarMessage('Spotify login failed - restart to retry', 10000)
})

// A command gave up before it could run, because reading the current
// state failed. The wording comes from Go rather than living here: only
// it knows which of the causes it was, and the whole point is that the
// bar names one instead of sending anyone off to find a log file.
EventsOn('command-failed', (message) => {
  clearPendingTrack()
  showBarMessage(message || 'Spotify request failed - restart app', 4000)
})

// Nothing is connected to Spotify Connect - the app is closed rather
// than merely idle, so there's no device to hand playback to and the
// command can't go anywhere.
EventsOn('no-device', () => {
  clearPendingTrack()
  showBarMessage("Spotify isn't open or playing anywhere", 5000)
})

// Fired whenever a panel is toggled, by button or hotkey.
EventsOn('panel-changed', (panel) => {
  document.getElementById('settings-panel').classList.toggle('hidden', panel !== 'settings')
  document.getElementById('playlists-panel').classList.toggle('hidden', panel !== 'playlists')
  document.getElementById('hotkeys-panel').classList.toggle('hidden', panel !== 'hotkeys')

  if (panel === 'playlists') {
    openPlaylistsPanel()
  }
  updatePanelHeight()
})

// The window is only ever as tall as the open panel needs, so adding a
// row doesn't mean hand-tuning a height in Go to match. Capped because
// the playlist list has no natural end - past this it scrolls instead
// of growing the window.
const PANEL_MAX_HEIGHT = 340

function updatePanelHeight() {
  const panel = document.querySelector(
    '#settings-panel:not(.hidden), #playlists-panel:not(.hidden), #hotkeys-panel:not(.hidden)'
  )
  if (!panel) return

  // Playlists is fixed rather than measured. Its list loads
  // asynchronously, so measuring on open catches it empty - and even
  // once loaded, sizing to it would make the window jump on every
  // keystroke as the search filters the list down.
  if (panel.id === 'playlists-panel') {
    SetPanelHeight(PANEL_MAX_HEIGHT)
    return
  }

  // Panels normally flex to fill the window and scroll what doesn't
  // fit, so they report the window's height rather than their own.
  // Freeing them briefly is what makes them state what they actually
  // want - the same trick measureNeededBarWidth uses for width.
  const prevFlex = panel.style.flex
  const prevHeight = panel.style.height
  const prevOverflow = panel.style.overflow

  panel.style.flex = 'none'
  panel.style.height = 'auto'
  panel.style.overflow = 'visible'
  const needed = Math.ceil(panel.getBoundingClientRect().height)

  panel.style.flex = prevFlex
  panel.style.height = prevHeight
  panel.style.overflow = prevOverflow

  SetPanelHeight(Math.min(PANEL_MAX_HEIGHT, needed))
}

// --- Playlist + song picker ---
const playlistSearchInput = document.getElementById('playlist-search-input')
const playlistListEl = document.getElementById('playlist-list')
// Cached after first load; playlists rarely change mid-session and the
// Spotify client ID is shared across all users. Track search can't be
// cached, so it stays a live debounced call.
let allPlaylists = null

function makeDivider(label) {
  const el = document.createElement('li')
  el.className = 'results-divider'
  el.textContent = label
  return el
}

// Renders playlists and tracks as two groups, only labelled with
// dividers when both actually have results.
function renderResults(playlists, tracks) {
  playlistListEl.innerHTML = ''

  if (playlists.length === 0 && tracks.length === 0) {
    const empty = document.createElement('li')
    empty.className = 'playlist-empty'
    empty.textContent = playlistSearchInput.value.trim() ? 'No matches' : 'No playlists found'
    playlistListEl.appendChild(empty)
    return
  }

  const showDividers = playlists.length > 0 && tracks.length > 0

  if (playlists.length > 0) {
    if (showDividers) playlistListEl.appendChild(makeDivider('Playlists'))
    for (const playlist of playlists) {
      const item = document.createElement('li')
      item.className = 'playlist-item'
      item.textContent = playlist.name
      item.addEventListener('click', () => {
        if (playlist.liked) {
          PlayLikedSongs()
        } else {
          PlayPlaylist(playlist.uri)
        }
        TogglePlaylistsPanel()
      })
      playlistListEl.appendChild(item)
    }
  }

  if (tracks.length > 0) {
    if (showDividers) playlistListEl.appendChild(makeDivider('Songs'))
    for (const track of tracks) {
      const item = document.createElement('li')
      item.className = 'track-item'

      const info = document.createElement('div')
      info.className = 'track-item-info'
      info.addEventListener('click', () => {
        PlayTrack(track.uri)
        // Before the panel closes over it, so the bar has already
        // changed by the time it's visible again.
        showPendingTrack(track)
        TogglePlaylistsPanel()
      })

      const name = document.createElement('div')
      name.className = 'track-item-name'
      name.textContent = track.name
      const artist = document.createElement('div')
      artist.className = 'track-item-artist'
      artist.textContent = track.artist
      info.appendChild(name)
      info.appendChild(artist)

      const saveBtn = document.createElement('button')
      saveBtn.className = 'track-item-save'
      saveBtn.title = 'Add to Liked Songs'
      saveBtn.textContent = '+'
      // Without stopPropagation this also triggers the row's play.
      saveBtn.addEventListener('click', (e) => {
        e.stopPropagation()
        // Disabled up front so a double-click can't fire twice, but the
        // tick waits for the call to actually succeed - it used to
        // appear unconditionally, reporting a save that had 403'd.
        saveBtn.disabled = true
        SaveTrackToLiked(track.id)
          .then(() => {
            saveBtn.textContent = '✓'
          })
          .catch(() => {
            saveBtn.disabled = false
            showToast('Could not save to Liked Songs', 2000)
          })
      })

      item.appendChild(info)
      item.appendChild(saveBtn)
      playlistListEl.appendChild(item)
    }
  }
}

// Guards against a slow response overwriting fresher results.
let searchToken = 0
let trackSearchTimeout = null

function filterPlaylists() {
  if (!allPlaylists) return
  const query = playlistSearchInput.value.trim().toLowerCase()
  const filteredPlaylists = query
    ? allPlaylists.filter((p) => p.name.toLowerCase().includes(query))
    : allPlaylists

  clearTimeout(trackSearchTimeout)
  searchToken++

  if (!query) {
    renderResults(filteredPlaylists, [])
    return
  }

  // Playlists match locally and render now; songs need a debounced
  // API call and fill in once it resolves.
  renderResults(filteredPlaylists, [])
  const thisSearch = searchToken
  trackSearchTimeout = setTimeout(async () => {
    let tracks
    try {
      tracks = (await SearchTracks(query)) || []
    } catch (err) {
      if (thisSearch !== searchToken) return
      // Said out loud rather than rendered as an empty result set: a
      // failed search and a search with no matches looked identical, so
      // a rejected token read as "this song isn't on Spotify".
      renderResults(filteredPlaylists, [])
      showToast(String((err && err.message) || err), 2500)
      return
    }
    if (thisSearch !== searchToken) return
    renderResults(filteredPlaylists, tracks)
  }, 350)
}

async function openPlaylistsPanel() {
  playlistSearchInput.value = ''
  playlistSearchInput.focus()

  if (!allPlaylists) {
    playlistListEl.innerHTML = '<li class="playlist-empty">Loading...</li>'

    let playlists
    try {
      playlists = await GetPlaylists()
    } catch (err) {
      // Deliberately left uncached so reopening the panel retries.
      // Caching the failure stuck the picker on Liked Songs alone for
      // the rest of the session, with no way to reload short of a
      // restart.
      playlistListEl.innerHTML = '<li class="playlist-empty">Could not load playlists</li>'
      return
    }

    // Liked Songs isn't a real playlist, so it's pinned on manually -
    // still searchable like the rest.
    allPlaylists = [{ name: 'Liked Songs', liked: true }, ...(playlists || [])]
  }
  filterPlaylists()
}

playlistSearchInput.addEventListener('input', filterPlaylists)

const playlistsToggleBtn = document.getElementById('playlists-toggle-btn')
playlistsToggleBtn.addEventListener('click', () => {
  TogglePlaylistsPanel()
})

// --- Dragging (JS-driven, not Wails' --wails-draggable) ---
// The native drag runs a modal OS move loop that swallows mouseup, so
// there's no reliable "drag ended" moment - which edge-snapping needs.
const nowPlayingEl = document.getElementById('now-playing')
const edgeSnapToggle = document.getElementById('edge-snap-toggle')

edgeSnapToggle.checked = localStorage.getItem('edgeSnapEnabled') !== 'false'
edgeSnapToggle.addEventListener('change', (e) => {
  localStorage.setItem('edgeSnapEnabled', e.target.checked)
})

// --- Customization: always on top ---
// main.go starts with AlwaysOnTop: true; this just allows turning it
// off.
const alwaysOnTopToggle = document.getElementById('always-on-top-toggle')

const savedAlwaysOnTop = localStorage.getItem('alwaysOnTop') !== 'false'
alwaysOnTopToggle.checked = savedAlwaysOnTop
WindowSetAlwaysOnTop(savedAlwaysOnTop)

alwaysOnTopToggle.addEventListener('change', (e) => {
  localStorage.setItem('alwaysOnTop', e.target.checked)
  WindowSetAlwaysOnTop(e.target.checked)
})

// --- Customization: start on startup ---
// Source of truth is the OS itself (registry key on Windows, LaunchAgent
// on macOS), not localStorage - it can be changed outside the app
// (Task Manager's Startup tab, deleting the LaunchAgent by hand), so
// the checkbox reflects whatever's actually there rather than a cached
// guess.
const autostartToggle = document.getElementById('autostart-toggle')

IsAutostartEnabled().then((enabled) => {
  autostartToggle.checked = enabled
})

autostartToggle.addEventListener('change', async (e) => {
  const enabled = e.target.checked
  try {
    await SetAutostart(enabled)
  } catch (err) {
    e.target.checked = !enabled
    showToast('Failed to update startup setting', 2000)
  }
})

// Flipping here rather than in Go keeps localStorage, the checkbox and
// the window state in sync - see ToggleAlwaysOnTop.
EventsOn('toggle-always-on-top', () => {
  const next = !alwaysOnTopToggle.checked
  alwaysOnTopToggle.checked = next
  localStorage.setItem('alwaysOnTop', next)
  WindowSetAlwaysOnTop(next)
})

let dragging = false
// True from mousedown to mouseup regardless of whether the async
// WindowGetPosition() has resolved, so mouseup can cancel a drag that's
// still starting up.
let dragSessionActive = false
let dragStartMouseX = 0
let dragStartMouseY = 0
let dragStartWinX = 0
let dragStartWinY = 0

nowPlayingEl.addEventListener('mousedown', async (e) => {
  // #track-info is only spared when it's actually a link. The class is
  // kept in step with currentSpotifyURI by render(), so with nothing
  // playing - no track, no URI, nothing to open - the title stops being
  // an exception and becomes part of the drag handle like the rest of
  // the bar. Excluding it unconditionally meant the one state with no
  // click to protect was the state with almost nowhere left to grab.
  if (
    e.button !== 0 ||
    e.target.closest('#settings-toggle-btn') ||
    e.target.closest('#playlists-toggle-btn') ||
    e.target.closest('#close-btn') ||
    e.target.closest('#track-info.clickable')
  ) return

  dragStartMouseX = e.screenX
  dragStartMouseY = e.screenY
  dragSessionActive = true

  const pos = await WindowGetPosition()
  // The button may already have been released while we were waiting on
  // that - without this check, mouseup (which only clears `dragging`)
  // would have no effect on a drag that hadn't started yet, and this
  // would then start it anyway, leaving the window stuck following the
  // cursor with no button held.
  if (!dragSessionActive) return

  dragStartWinX = pos.x
  dragStartWinY = pos.y
  dragging = true
  BeginDrag()
})

// Mousemove can fire far more often than once per rendered frame, and
// each drag update is an async round trip to Go (see DragWindowTo
// below) - forwarding every single event makes them pile up and lag
// behind the cursor. Coalescing to one update per animation frame
// keeps it responsive without flooding the Go side.
let pendingDragTarget = null
let dragFrameScheduled = false

document.addEventListener('mousemove', (e) => {
  if (!dragging) return

  pendingDragTarget = {
    x: dragStartWinX + (e.screenX - dragStartMouseX),
    y: dragStartWinY + (e.screenY - dragStartMouseY),
  }

  if (dragFrameScheduled) return
  dragFrameScheduled = true
  requestAnimationFrame(() => {
    dragFrameScheduled = false
    if (!pendingDragTarget) return
    // Goes through Go (DragWindowTo) rather than calling the Wails
    // runtime's WindowSetPosition directly - on Windows that runtime
    // call doesn't take an absolute desktop coordinate the way it
    // looks like it should, and only Go can correctly compensate for
    // that.
    DragWindowTo(pendingDragTarget.x, pendingDragTarget.y)
    pendingDragTarget = null
  })
})

document.addEventListener('mouseup', () => {
  dragSessionActive = false
  if (!dragging) return
  dragging = false
  pendingDragTarget = null
  EndDrag()
  if (edgeSnapToggle.checked) {
    SnapWindowToEdges()
  }
})

// --- Customization: accent + background color ---
const accentColorInput = document.getElementById('accent-color-input')
const bgColorInput = document.getElementById('bg-color-input')
const bgGradientToggle = document.getElementById('bg-gradient-toggle')
const bgGradientColorInput = document.getElementById('bg-gradient-color-input')
const bgGradientRow = document.getElementById('bg-gradient-row')
const root = document.documentElement

const DEFAULT_ACCENT = '#1db954'
const DEFAULT_BG = '#121212'
const DEFAULT_BG_GRADIENT = '#1db954'

function applyAccentColor(hex) {
  root.style.setProperty('--accent-color', hex)
}

// isLightColor estimates perceived brightness (ITU-R BT.601) of a #rrggbb
// color - good enough to tell whether light text would still be readable
// against it.
function isLightColor(hex) {
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  const brightness = (r * 299 + g * 587 + b * 114) / 1000
  return brightness > 180
}

// applyBackground sets --bg to either a plain color or a gradient
// function - both are valid values for the `background` shorthand, so
// nothing downstream needs to know which one it got. It also flips --fg
// to a dark color whenever the background (or, with a gradient, either
// end of it) is light enough that the usual light text would wash out.
function applyBackground() {
  const gradientOn = bgGradientToggle.checked
  const bg = bgColorInput.value
  const gradientColor = bgGradientColorInput.value

  if (gradientOn) {
    root.style.setProperty('--bg', `linear-gradient(135deg, ${bg}, ${gradientColor})`)
  } else {
    root.style.setProperty('--bg', bg)
  }

  const needsDarkText = isLightColor(bg) || (gradientOn && isLightColor(gradientColor))
  root.style.setProperty('--fg', needsDarkText ? '#121212' : '#e0e0e0')
}

const savedAccent = localStorage.getItem('accentColor') || DEFAULT_ACCENT
const savedBg = localStorage.getItem('bgColor') || DEFAULT_BG
const savedGradientEnabled = localStorage.getItem('bgGradientEnabled') === 'true'
const savedGradientColor = localStorage.getItem('bgGradientColor') || DEFAULT_BG_GRADIENT

accentColorInput.value = savedAccent
applyAccentColor(savedAccent)

bgColorInput.value = savedBg
bgGradientToggle.checked = savedGradientEnabled
bgGradientColorInput.value = savedGradientColor
bgGradientRow.classList.toggle('hidden', !savedGradientEnabled)
applyBackground()

accentColorInput.addEventListener('input', (e) => {
  applyAccentColor(e.target.value)
  localStorage.setItem('accentColor', e.target.value)
})

bgColorInput.addEventListener('input', (e) => {
  localStorage.setItem('bgColor', e.target.value)
  applyBackground()
})

bgGradientToggle.addEventListener('change', (e) => {
  bgGradientRow.classList.toggle('hidden', !e.target.checked)
  localStorage.setItem('bgGradientEnabled', e.target.checked)
  applyBackground()
  // Showing/hiding the colour row changes how tall the panel needs to
  // be, and it's the one row that appears while the panel is already open.
  updatePanelHeight()
})

bgGradientColorInput.addEventListener('input', (e) => {
  localStorage.setItem('bgGradientColor', e.target.value)
  applyBackground()
})

// --- Customization: hotkeys ---
const settingsToggleBtn = document.getElementById('settings-toggle-btn')
settingsToggleBtn.addEventListener('click', () => {
  ToggleSettingsPanel()
})

// This frameless window has no OS title bar, so there's otherwise no
// way to close it short of Task Manager.
const closeBtn = document.getElementById('close-btn')
closeBtn.addEventListener('click', () => {
  Quit()
})

const KEY_LABELS = { space: 'Space', left: '←', right: '→', up: '↑', down: '↓' }
const MOD_LABELS = { ctrl: 'Ctrl', alt: 'Alt', shift: 'Shift', cmd: 'Cmd' }
// Alt is labelled Option on Mac keyboards. Spelled out rather than the
// native ⌃⌥⇧⌘ symbols, which aren't obvious to everyone.
const MOD_LABELS_MAC = { ctrl: 'Ctrl', alt: 'Opt', shift: 'Shift', cmd: 'Cmd' }

// Swapped to the macOS set once Environment() resolves (see below).
let modLabels = MOD_LABELS

function formatBinding(binding) {
  if (!binding) return '...'
  const parts = binding.mods.map((m) => modLabels[m] || m)
  parts.push(KEY_LABELS[binding.key] || binding.key.toUpperCase())
  return parts.join('+')
}

// Maps a keydown event to one of the key names the Go side understands.
// Returns null for keys we don't support binding to.
function normalizeKeyEvent(e) {
  const named = { ' ': 'space', ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' }
  if (named[e.key]) return named[e.key]
  if (/^[a-zA-Z]$/.test(e.key)) return e.key.toLowerCase()
  return null
}

function recordHotkey(button, action) {
  const previousLabel = button.textContent
  button.textContent = 'Press keys...'
  button.disabled = true

  // The combo is captured on keydown but not committed until that same
  // key is released - so holding modifiers down while deciding, or
  // pressing several keys before settling on one, doesn't submit early.
  let candidateKey = null
  let candidateMods = []

  function keydownHandler(e) {
    e.preventDefault()
    e.stopPropagation()

    if (e.key === 'Escape') {
      cleanup()
      button.textContent = previousLabel
      return
    }

    // Pure modifier presses don't count as the binding's key - keep waiting.
    if (['Control', 'Alt', 'Shift', 'Meta'].includes(e.key)) return

    const keyName = normalizeKeyEvent(e)
    if (!keyName) return

    const mods = []
    if (e.ctrlKey) mods.push('ctrl')
    if (e.altKey) mods.push('alt')
    if (e.shiftKey) mods.push('shift')
    if (e.metaKey) mods.push('cmd')

    // No modifier is required - a binding with none registers as a
    // global hotkey on that key alone, firing system-wide even while
    // typing that key elsewhere.
    candidateKey = keyName
    candidateMods = mods
    button.textContent = formatBinding({ mods, key: keyName })
  }

  function keyupHandler(e) {
    if (!candidateKey || normalizeKeyEvent(e) !== candidateKey) return
    e.preventDefault()
    e.stopPropagation()

    const mods = candidateMods
    const key = candidateKey
    cleanup()

    SetHotkeyBinding(action, mods, key)
      .then(() => {
        button.textContent = formatBinding({ mods, key })
      })
      .catch((err) => {
        button.textContent = previousLabel
        alert('Could not set hotkey: ' + err)
      })
  }

  function cleanup() {
    document.removeEventListener('keydown', keydownHandler, true)
    document.removeEventListener('keyup', keyupHandler, true)
    button.disabled = false
  }

  document.addEventListener('keydown', keydownHandler, true)
  document.addEventListener('keyup', keyupHandler, true)
}

document.querySelectorAll('.hotkey-btn').forEach((button) => {
  button.addEventListener('click', () => recordHotkey(button, button.dataset.action))
})

// Short reference list of the current bindings, shown even with the
// rebind buttons hidden - built from the same live config, so it stays
// correct if hotkeys.json is ever edited by hand or the rebind UI comes
// back.
const HOTKEY_ACTIONS = ['settings', 'playlists', 'playPause', 'next', 'previous', 'shuffle', 'loop', 'volumeUp', 'volumeDown', 'alwaysOnTop']
const HOTKEY_LABELS = {
  settings: 'Settings',
  playlists: 'Playlists',
  playPause: 'Play/Pause',
  next: 'Next',
  previous: 'Prev',
  shuffle: 'Shuffle',
  loop: 'Loop',
  volumeUp: 'Vol+',
  volumeDown: 'Vol-',
  alwaysOnTop: 'On top',
}

// Fetched alongside the config so labels are already correct before
// anything formats - otherwise the summary renders wrong, then fixes
// itself.
Promise.all([GetHotkeyConfig(), Environment()]).then(([cfg, env]) => {
  if (env.platform === 'darwin') {
    modLabels = MOD_LABELS_MAC
  }

  document.querySelectorAll('.hotkey-btn').forEach((button) => {
    button.textContent = formatBinding(cfg[button.dataset.action])
  })

  renderHotkeyList(cfg)
})

// One row per action, action name left and combo right. Built from the
// live config rather than hardcoded, so it stays correct once these
// become rebindable.
function renderHotkeyList(cfg) {
  const list = document.getElementById('hotkey-list')
  list.innerHTML = ''

  for (const action of HOTKEY_ACTIONS) {
    const row = document.createElement('li')
    row.className = 'hotkey-row'

    const name = document.createElement('span')
    name.textContent = HOTKEY_LABELS[action]

    const key = document.createElement('span')
    key.className = 'key'
    key.textContent = formatBinding(cfg[action])

    row.appendChild(name)
    row.appendChild(key)
    list.appendChild(row)
  }
}

// The hotkeys panel has no global hotkey of its own - it's reached from
// the settings panel and goes back there, so the two buttons are the
// only way in and out.
document.getElementById('hotkeys-open-btn').addEventListener('click', () => {
  ToggleHotkeysPanel()
})

document.getElementById('hotkeys-back-btn').addEventListener('click', () => {
  ToggleSettingsPanel()
})

// Stamped in at build time (see internal/app/version.go); reads "dev"
// for local builds.
const versionEl = document.getElementById('app-version')
let runningVersion = ''
let availableUpdate = null

// The two halves of this line arrive independently - the running version
// from a local call, the update from a GitHub round trip - so it's
// rendered from whatever is known so far rather than assembled by
// whichever one happens to land second. Interpolating the version inside
// the update handler printed a blank one whenever the update won.
//
// The version line is the notice's permanent home rather than a bar
// message: the bar is transient by design, and an update that scrolled
// past while the user was looking elsewhere may as well not have been
// mentioned. The line stays until they act on it.
function renderVersionLine() {
  if (!runningVersion) return
  versionEl.textContent = availableUpdate
    ? `spotmini ${runningVersion} — ${availableUpdate.version} available`
    : `spotmini ${runningVersion}`
  versionEl.classList.toggle('update-available', !!availableUpdate)
}

// Attached once, up front, rather than when an update turns up - so it
// can't be added twice and there's nothing to tear down if it never is.
versionEl.addEventListener('click', () => {
  if (availableUpdate) OpenReleasePage()
})

function setAvailableUpdate(update) {
  if (!update) return
  availableUpdate = update
  renderVersionLine()
}

// Subscribed before the value is asked for, deliberately: an update
// found between the two would slip through the gap the other way round.
EventsOn('update-available', setAvailableUpdate)

// Asked for as well as listened for. The check starts before this window
// exists, so an update found quickly is emitted with nothing listening
// and lost - this covers that case, and the event covers a check that
// finishes after the window is up. See GetUpdateInfo in update.go.
GetUpdateInfo().then(setAvailableUpdate)

GetVersion().then((v) => {
  runningVersion = v
  renderVersionLine()
})

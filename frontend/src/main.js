import { EventsOn, Environment, WindowGetPosition, WindowGetSize, WindowSetSize, WindowSetAlwaysOnTop, BrowserOpenURL, Quit } from '../wailsjs/runtime/runtime'
import {
  GetNowPlaying,
  GetHotkeyConfig,
  SetHotkeyBinding,
  ToggleSettingsPanel,
  TogglePlaylistsPanel,
  GetPlaylists,
  PlayPlaylist,
  PlayLikedSongs,
  SearchTracks,
  PlayTrack,
  SaveTrackToLiked,
  IsAutostartEnabled,
  SetAutostart,
  SnapWindowToEdges,
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
// Timestamp the premium notice holds the track-info slot until; read by
// render(), set by the premium-required handler further down.
let premiumMessageUntil = 0

// The spotify: URI scheme hands off to the desktop app, unlike an
// https:// link which always opens a browser. currentSpotifyURI is
// empty when there's nothing to link to.
trackInfoEl.addEventListener('click', () => {
  if (currentSpotifyURI) BrowserOpenURL(currentSpotifyURI)
})

// --- Customization: auto-fit window width ---
// Off by default (titles just ellipsize). On, the window widens to fit
// the track name and artist, up to MAX_WIDTH.
const MIN_WIDTH = 320
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

async function updateAutoWidth() {
  if (!autoFitToggle.checked) return

  const target = Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, measureNeededBarWidth() + 8))
  const size = await WindowGetSize()
  if (size.w !== target) {
    WindowSetSize(target, size.h)
  }
}

async function resetWidth() {
  const size = await WindowGetSize()
  if (size.w !== MIN_WIDTH) {
    WindowSetSize(MIN_WIDTH, size.h)
  }
}

function formatTime(totalSecs) {
  const minutes = Math.floor(totalSecs / 60)
  const seconds = Math.floor(totalSecs % 60)
  return `${minutes}:${seconds.toString().padStart(2, '0')}`
}

function render() {
  // The premium notice owns the track-info slot until it expires -
  // otherwise the once-a-second tick below would overwrite it almost
  // immediately, long before it could be read.
  if (Date.now() < premiumMessageUntil) return

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

// Re-syncs the local counter with Spotify's actual state.
async function fetchNowPlaying() {
  try {
    const state = await GetNowPlaying()
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
  } catch (err) {
    currentSpotifyURI = ''
    trackInfoEl.classList.remove('clickable')
    trackInfoEl.textContent = 'Error loading playback'
    timerEl.textContent = ''
  }
}

// Local ticker - counts up each second without hitting Spotify,
// re-syncing periodically.
let secondsSinceSync = 0
let idleSecondsSinceCheck = 0

setInterval(() => {
  if (!currentSong) {
    idleSecondsSinceCheck++
    if (idleSecondsSinceCheck >= 2) {
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

  if (totalSeconds > 0 && currentSeconds >= totalSeconds) {
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

// Emitted after any playback command, so the UI reflects it right away
// instead of waiting for the next poll.
EventsOn('playback-changed', () => {
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

// Spotify rejects every playback command on a Free account, so without
// this a Free user sees nothing happen and no reason why. Shown in the
// track-info slot rather than as a toast, then cleared by re-fetching.
const PREMIUM_MESSAGE_MS = 3000
let premiumMessageTimeout = null

EventsOn('premium-required', () => {
  premiumMessageUntil = Date.now() + PREMIUM_MESSAGE_MS
  trackInfoEl.textContent = 'Spotify Premium is required for playback control'
  timerEl.textContent = ''
  statusDotEl.classList.add('hidden')
  shuffleIconEl.classList.add('hidden')
  loopIconEl.classList.add('hidden')

  clearTimeout(premiumMessageTimeout)
  premiumMessageTimeout = setTimeout(() => {
    premiumMessageUntil = 0
    fetchNowPlaying()
  }, PREMIUM_MESSAGE_MS)
})

// Fired whenever a panel is toggled, by button or hotkey.
EventsOn('panel-changed', (panel) => {
  const settingsPanel = document.getElementById('settings-panel')
  const playlistsPanel = document.getElementById('playlists-panel')

  settingsPanel.classList.toggle('hidden', panel !== 'settings')
  playlistsPanel.classList.toggle('hidden', panel !== 'playlists')

  if (panel === 'playlists') {
    openPlaylistsPanel()
  }
})

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
        SaveTrackToLiked(track.id)
        saveBtn.textContent = '✓'
        saveBtn.disabled = true
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
    const tracks = (await SearchTracks(query)) || []
    if (thisSearch !== searchToken) return
    renderResults(filteredPlaylists, tracks)
  }, 350)
}

async function openPlaylistsPanel() {
  playlistSearchInput.value = ''
  playlistSearchInput.focus()

  if (!allPlaylists) {
    playlistListEl.innerHTML = '<li class="playlist-empty">Loading...</li>'
    // Liked Songs isn't a real playlist, so it's pinned on manually -
    // still searchable like the rest.
    allPlaylists = [{ name: 'Liked Songs', liked: true }, ...((await GetPlaylists()) || [])]
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
  if (e.button !== 0 || e.target.closest('#settings-toggle-btn') || e.target.closest('#playlists-toggle-btn') || e.target.closest('#close-btn') || e.target.closest('#track-info')) return

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

  const summaryEl = document.getElementById('hotkey-summary')
  if (summaryEl) {
    summaryEl.textContent = HOTKEY_ACTIONS.map(
      (action) => `${HOTKEY_LABELS[action]} ${formatBinding(cfg[action])}`
    ).join(' · ')
  }
})

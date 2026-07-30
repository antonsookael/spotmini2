import { EventsOn, WindowGetPosition } from '../wailsjs/runtime/runtime'
import {
  GetNowPlaying,
  GetHotkeyConfig,
  SetHotkeyBinding,
  ToggleSettingsPanel,
  SnapWindowToEdges,
  DragWindowTo,
  BeginDrag,
  EndDrag,
} from '../wailsjs/go/main/App'

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
let isCurrentlyPlaying = false
let isShuffled = false
let repeatState = 'off'

function formatTime(totalSecs) {
  const minutes = Math.floor(totalSecs / 60)
  const seconds = Math.floor(totalSecs % 60)
  return `${minutes}:${seconds.toString().padStart(2, '0')}`
}

function render() {
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
  // Repeat-one gets a small "1" badge on the same icon, so it's
  // distinguishable from repeating the whole playlist/album at a glance.
  loopOneBadgeEl.classList.toggle('hidden', repeatState !== 'track')

  trackInfoEl.textContent = `${currentSong} - ${currentArtist}`
  timerEl.textContent =
    `${formatTime(currentSeconds)}/${formatTime(totalSeconds)}`
}

// fetchNowPlaying calls into Go, which calls Spotify, and resets our
// local counter to match reality.
async function fetchNowPlaying() {
  try {
    const state = await GetNowPlaying()
    if (!state.item || !state.item.name) {
      currentSong = ''
      render()
      return
    }
    currentSong = state.item.name
    currentArtist = state.item.artists[0].name
    currentSeconds = Math.floor(state.progress_ms / 1000)
    totalSeconds = Math.floor(state.item.duration_ms / 1000)
    isCurrentlyPlaying = state.is_playing
    isShuffled = state.shuffle_state
    repeatState = state.repeat_state || 'off'
    render()
  } catch (err) {
    trackInfoEl.textContent = 'Error loading playback'
    timerEl.textContent = ''
  }
}

// Local ticker - counts up once per second without calling Go/Spotify,
// re-syncing periodically. Same pattern as the old Fyne version.
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

  if (secondsSinceSync >= 15) {
    secondsSinceSync = 0
    fetchNowPlaying()
  }
}, 1000)

EventsOn('logged-in', () => {
  fetchNowPlaying()
})

// Emitted by Go right after a play/pause/next/previous/shuffle command -
// re-syncs immediately instead of waiting for the next periodic poll, so
// the status dot reflects the change right away.
EventsOn('playback-changed', () => {
  setTimeout(fetchNowPlaying, 300)
})

// Listen for the toggle-settings event emitted from Go when Ctrl+Alt+C is pressed
EventsOn('toggle-settings', (isExpanded) => {
  const settingsPanel = document.getElementById('settings-panel')
  if (isExpanded) {
    settingsPanel.classList.remove('hidden')
  } else {
    settingsPanel.classList.add('hidden')
  }
})

// --- Dragging (driven from JS instead of Wails' native --wails-draggable) ---
// The native drag hands the OS a modal move loop that swallows mouseup,
// so there's no reliable moment to know the drag actually ended - which
// is what edge-snapping needs. Tracking the drag ourselves means mouseup
// fires normally and we can snap right on release, never mid-drag.
const nowPlayingEl = document.getElementById('now-playing')
const edgeSnapToggle = document.getElementById('edge-snap-toggle')

edgeSnapToggle.checked = localStorage.getItem('edgeSnapEnabled') !== 'false'
edgeSnapToggle.addEventListener('change', (e) => {
  localStorage.setItem('edgeSnapEnabled', e.target.checked)
})

let dragging = false
// True from mousedown until mouseup, regardless of whether the async
// WindowGetPosition() below has resolved yet - lets mouseup cancel a
// drag that's still starting up instead of only being able to cancel
// one that's already flagged `dragging`.
let dragSessionActive = false
let dragStartMouseX = 0
let dragStartMouseY = 0
let dragStartWinX = 0
let dragStartWinY = 0

nowPlayingEl.addEventListener('mousedown', async (e) => {
  if (e.button !== 0 || e.target.closest('#settings-toggle-btn')) return

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

const KEY_LABELS = { space: 'Space', left: '←', right: '→', up: '↑', down: '↓' }
const MOD_LABELS = { ctrl: 'Ctrl', alt: 'Alt', shift: 'Shift', cmd: 'Cmd' }

function formatBinding(binding) {
  if (!binding) return '...'
  const parts = binding.mods.map((m) => MOD_LABELS[m] || m)
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
const HOTKEY_ACTIONS = ['settings', 'playPause', 'next', 'previous', 'shuffle', 'loop']
const HOTKEY_LABELS = {
  settings: 'Settings',
  playPause: 'Play/Pause',
  next: 'Next',
  previous: 'Prev',
  shuffle: 'Shuffle',
  loop: 'Loop',
}

GetHotkeyConfig().then((cfg) => {
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

import { EventsOn, WindowGetPosition, WindowSetPosition } from '../wailsjs/runtime/runtime'
import {
  GetNowPlaying,
  GetHotkeyConfig,
  SetHotkeyBinding,
  ToggleSettingsPanel,
  SnapWindowToEdges,
} from '../wailsjs/go/main/App'

const trackInfoEl = document.getElementById('track-info')
const timerEl = document.getElementById('timer')

let currentSeconds = 0
let totalSeconds = 0
let currentSong = ''
let currentArtist = ''
let isCurrentlyPlaying = false

function formatTime(totalSecs) {
  const minutes = Math.floor(totalSecs / 60)
  const seconds = Math.floor(totalSecs % 60)
  return `${minutes}:${seconds.toString().padStart(2, '0')}`
}

function render() {
  if (!currentSong) {
    trackInfoEl.textContent = 'Nothing playing'
    timerEl.textContent = ''
    return
  }

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
let dragging = false
let dragStartMouseX = 0
let dragStartMouseY = 0
let dragStartWinX = 0
let dragStartWinY = 0

nowPlayingEl.addEventListener('mousedown', async (e) => {
  if (e.button !== 0 || e.target.closest('#settings-toggle-btn')) return

  dragStartMouseX = e.screenX
  dragStartMouseY = e.screenY
  const pos = await WindowGetPosition()
  dragStartWinX = pos.x
  dragStartWinY = pos.y
  dragging = true
})

document.addEventListener('mousemove', (e) => {
  if (!dragging) return
  WindowSetPosition(
    dragStartWinX + (e.screenX - dragStartMouseX),
    dragStartWinY + (e.screenY - dragStartMouseY)
  )
})

document.addEventListener('mouseup', () => {
  if (!dragging) return
  dragging = false
  SnapWindowToEdges()
})

// --- Customization: accent color ---
const accentColorInput = document.getElementById('accent-color-input')
const root = document.documentElement

const DEFAULT_ACCENT = '#1db954'

function applyAccentColor(hex) {
  root.style.setProperty('--accent-color', hex)
}

const savedAccent = localStorage.getItem('accentColor') || DEFAULT_ACCENT

accentColorInput.value = savedAccent
applyAccentColor(savedAccent)

accentColorInput.addEventListener('input', (e) => {
  applyAccentColor(e.target.value)
  localStorage.setItem('accentColor', e.target.value)
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

  function handler(e) {
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

    // Require at least one modifier so a bare letter can't become a global
    // hotkey that hijacks normal typing everywhere.
    if (mods.length === 0) return

    cleanup()
    SetHotkeyBinding(action, mods, keyName)
      .then(() => {
        button.textContent = formatBinding({ mods, key: keyName })
      })
      .catch((err) => {
        button.textContent = previousLabel
        alert('Could not set hotkey: ' + err)
      })
  }

  function cleanup() {
    document.removeEventListener('keydown', handler, true)
    button.disabled = false
  }

  document.addEventListener('keydown', handler, true)
}

document.querySelectorAll('.hotkey-btn').forEach((button) => {
  button.addEventListener('click', () => recordHotkey(button, button.dataset.action))
})

GetHotkeyConfig().then((cfg) => {
  document.querySelectorAll('.hotkey-btn').forEach((button) => {
    button.textContent = formatBinding(cfg[button.dataset.action])
  })
})

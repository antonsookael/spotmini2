import { EventsOn } from '../wailsjs/runtime/runtime'
import { GetNowPlaying } from '../wailsjs/go/main/App'

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

// --- Customization: accent color + panel opacity ---
const accentColorInput = document.getElementById('accent-color-input')
const opacityInput = document.getElementById('opacity-input')
const root = document.documentElement

const DEFAULT_ACCENT = '#1db954'
const DEFAULT_OPACITY = '1'

function applyAccentColor(hex) {
  root.style.setProperty('--accent-color', hex)
}

function applyOpacity(value) {
  root.style.setProperty('--panel-opacity', value)
}

const savedAccent = localStorage.getItem('accentColor') || DEFAULT_ACCENT
const savedOpacity = localStorage.getItem('panelOpacity') || DEFAULT_OPACITY

accentColorInput.value = savedAccent
opacityInput.value = savedOpacity
applyAccentColor(savedAccent)
applyOpacity(savedOpacity)

accentColorInput.addEventListener('input', (e) => {
  applyAccentColor(e.target.value)
  localStorage.setItem('accentColor', e.target.value)
})

opacityInput.addEventListener('input', (e) => {
  applyOpacity(e.target.value)
  localStorage.setItem('panelOpacity', e.target.value)
})

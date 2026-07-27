import { EventsOn } from '../wailsjs/runtime/runtime'
import { GetNowPlaying } from '../wailsjs/go/main/App'

const nowPlayingEl = document.getElementById('now-playing')

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
    nowPlayingEl.textContent = 'Nothing playing'
    return
  }
  nowPlayingEl.textContent =
    `${currentSong} - ${currentArtist}  ${formatTime(currentSeconds)}/${formatTime(totalSeconds)}`
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
    nowPlayingEl.textContent = 'Error loading playback'
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

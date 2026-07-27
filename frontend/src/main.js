import { EventsOn } from '../wailsjs/runtime/runtime'

const nowPlayingEl = document.getElementById('now-playing')

// This event gets fired from Go (app.go's startup function) once
// login/token-refresh finishes in the background.
EventsOn('logged-in', () => {
  nowPlayingEl.textContent = 'Connected'
  // Next step: call a Go method here to fetch and display the
  // actual now-playing info, and set up polling/hotkey updates.
})

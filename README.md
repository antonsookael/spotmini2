# spotmini

A tiny, frameless, always-on-top Spotify now-playing strip built with Go and Wails.

## Features

- Small corner-sized window, no title bar, stays on top of other apps
- Shows current song, artist, and live progress
- Play/pause control wip
- Customizable accent color with hotkey ctrl+alt+c on windows and ctrl+option+c on macOS
- Background OAuth login with automatic token refresh
- Draggable window despite having no title bar

## Setup

1. Create an app on the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard) and grab your Client ID and Client Secret.
2. Set the app's Redirect URI to:
   ```
   http://127.0.0.1:8888/callback
   ```
3. Create a `.env` file in the project root:
   ```
   clientID=your_client_id_here
   clientSecret=your_client_secret_here
   ```
4. Install dependencies:
   ```
   go mod tidy
   cd frontend && npm install
   ```

## Running

```
wails dev
```

On first run, a browser window opens for Spotify login. After that, a saved token is refreshed automatically and no further login is needed until it's revoked.

## Building

```
wails build
```

## Tech

- Go backend, talking directly to the Spotify Web API
- [Wails v2](https://wails.io) for the frameless desktop window
- Vanilla HTML/CSS/JS frontend

## Status

Actively in progress - global hotkeys, previous/next/shuffle controls, and playlist selection are planned next.

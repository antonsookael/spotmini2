# spotmini

A tiny, frameless, always-on-top Spotify now-playing strip built with Go and Wails.

## Features

- Small corner-sized window, no title bar, stays on top of other apps
- Shows current song, artist, and live progress
- Play/pause control wip
- Customizable accent color with hotkey ctrl+alt+c on windows and ctrl+option+c on macOS
- Background OAuth login with automatic token refresh
- Draggable window despite having no title bar

## Download

Grab the latest build from [Releases](../../releases) - no setup needed, just run it and log in with Spotify when prompted. Windows may warn that it's from an unrecognized publisher (it isn't code-signed); click "More info" -> "Run anyway".

Login uses OAuth Authorization Code with PKCE, so there's no client secret involved at all - only a public Client ID (baked into the build) and a one-time proof value generated fresh on your machine for each login.

## Developing

1. Install dependencies:
   ```
   go mod tidy
   cd frontend && npm install
   ```
2. (Optional) to test against your own Spotify app instead of spotmini's shared one, create one on the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard), set its Redirect URI to `http://127.0.0.1:8888/callback`, and add a `.env` file in the project root:
   ```
   SPOTIFY_CLIENT_ID=your_client_id_here
   ```
   No client secret is needed even here - PKCE doesn't use one.

## Running

```
wails dev
```

On first run, a browser window opens for Spotify login. After that, a saved token is refreshed automatically and no further login is needed until it's revoked.

## Building

```
wails build
```

A tagged push (`git tag vX.Y.Z && git push --tags`) also triggers a GitHub Actions workflow that builds and attaches a Windows binary to a new Release automatically.

## Tech

- Go backend, talking directly to the Spotify Web API
- [Wails v2](https://wails.io) for the frameless desktop window
- Vanilla HTML/CSS/JS frontend

## Status

Actively in progress - global hotkeys, previous/next/shuffle controls, and playlist selection are planned next.

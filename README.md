# spotmini

A tiny, frameless, always-on-top Spotify now-playing strip built with Go and Wails.

## Features

- Small corner-sized window, no title bar, stays on top of other apps
- Shows current song, artist, and live progress
- Play/pause, next/previous, shuffle, and loop (repeat off/playlist/track) controls
- Volume up/down with a brief on-screen indicator
- Playlist picker with instant search over your own playlists
- Global hotkeys for every control above, so it all works even when the window isn't focused
- Customizable accent color and background (solid or gradient)
- Auto-fit window width, so long song/artist names aren't clipped
- Edge snapping when dragging the window near a screen edge
- Draggable window despite having no title bar
- Background OAuth login with automatic token refresh

## Default hotkeys

All use Ctrl+Alt on Windows and Linux, and Ctrl+Option on macOS:

| Action          | Key   |
| --------------- | ----- |
| Toggle settings | C     |
| Toggle playlists| P     |
| Play / Pause    | Space |
| Next track      | →     |
| Previous track  | ←     |
| Shuffle         | S     |
| Loop            | L     |
| Volume up       | ↑     |
| Volume down     | ↓     |
| Always on top   | T     |

Rebinding these to different keys is currently WIP - the backend supports it, but there's no UI exposed for it yet.

## Download

Grab the latest build for your OS from [Releases](../../releases) - `spotmini-vX.Y.Zwin.exe` for Windows, `spotmini-vX.Y.Zmac.zip` for macOS, `spotmini-vX.Y.Zlinux.tar.gz` for Linux.

Login uses OAuth Authorization Code with PKCE, so there's no client secret involved at all - only a public Client ID (baked into the build) and a one-time proof value generated fresh on your machine for each login.

### Windows

It isn't code-signed, so Windows will warn that it's from an unrecognized publisher. Click "More info" -> "Run anyway".

Windows Defender (or another antivirus) may also flag or block it outright as a false positive - unsigned, low-reputation executables get flagged more aggressively, and Go binaries specifically tend to trigger this more than average. If that happens:

1. In Windows Security -> Virus & threat protection -> Protection history, find the detection and choose **Allow** (or restore it from quarantine) to run it without disabling real-time protection entirely.
2. Optionally [report it to Microsoft](https://www.microsoft.com/en-us/wdsi/filesubmission) as a false positive so Defender stops flagging it for everyone, not just you - see [CONTRIBUTING.md](CONTRIBUTING.md) for exact steps.

### macOS

The build is neither code-signed with a paid Apple Developer ID nor notarized, so macOS Gatekeeper blocks it outright ("Apple cannot check it for malicious software") rather than just warning. To run it:

1. Unzip `spotmini-vX.Y.Zmac.zip` - this gives you `spotmini.app`.
2. Clear the quarantine flag Gatekeeper attached on download:
   ```
   xattr -cr /path/to/spotmini.app
   ```
3. Open the app normally (double-click, or move it to `/Applications` first).
4. For the global hotkeys to work, grant it Accessibility access: **System Settings -> Privacy & Security -> Accessibility**, add `spotmini` (via the **+** button if it's not already listed), and enable it. Quit and relaunch the app afterward.

Because the build is unsigned, macOS ties both the quarantine flag and the Accessibility grant to that exact binary - you'll need to repeat steps 2 and 4 for every new release you download, since each one is a different build.

### Linux

The build is a single x86-64 binary that uses the system's GTK 3 and WebKitGTK, so those need to be installed:

- Arch: `sudo pacman -S webkit2gtk-4.1`
- Debian/Ubuntu: `sudo apt install libwebkit2gtk-4.1-0`
- Fedora: `sudo dnf install webkit2gtk4.1`

Then unpack and run it:

```
tar -xzf spotmini-vX.Y.Zlinux.tar.gz
./spotmini
```

On a Wayland session the app runs through XWayland rather than as a native Wayland window, so XWayland has to be available (it is by default on the major desktops). That's deliberate: Wayland doesn't let a window position itself or stay on top, which is most of what spotmini does. Set `GDK_BACKEND` yourself to override it.

The global hotkeys are X11 key grabs. On an X11 session they work everywhere. On Wayland it's up to the desktop whether key presses typed in other apps are passed on to X11 ones: KDE Plasma 6 does this by default for combinations with Ctrl or Alt held, which covers all of the defaults (the setting is on the *Legacy X11 App Support* page in System Settings). On a desktop that doesn't, the hotkeys only fire while spotmini or another X11 app has focus.

## Where your data is stored

The saved login token, hotkey bindings, and a diagnostic log live in a per-user app-data folder, not next to the executable. This folder name is deliberately unchanged from the app's old `spotmini-gui` name, so upgrading doesn't lose your saved login:

- macOS: `~/Library/Application Support/spotmini-gui/`
- Windows: `%AppData%\spotmini-gui\`
- Linux: `~/.config/spotmini-gui/`

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

On Linux, add `-tags webkit2_41` to both `wails dev` and `wails build` - Wails otherwise looks for the older WebKitGTK 4.0, which current distros no longer ship. Building needs the GTK 3, WebKitGTK 4.1 and X11 development packages (`webkit2gtk-4.1` on Arch; `libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev` on Debian/Ubuntu).

On macOS this defaults to an Intel-only (`darwin/amd64`) binary - pass `-platform darwin/universal` to build one that runs on both Intel and Apple Silicon without Rosetta.

A tagged push (`git tag vX.Y.Z && git push origin vX.Y.Z`) triggers a GitHub Actions workflow that builds Windows, macOS (universal) and Linux binaries and attaches them to a new Release automatically.

## Tech

- Go backend, talking directly to the Spotify Web API
- [Wails v2](https://wails.io) for the frameless desktop window
- Vanilla HTML/CSS/JS frontend

## Status

Actively in progress. Playback controls, volume, and global hotkeys are all working; a UI for rebinding hotkeys exists in the code but is currently hidden pending more testing. Code signing/notarization is intentionally skipped for now (see the macOS setup steps above for the workaround this requires). Playlist selection is still planned.

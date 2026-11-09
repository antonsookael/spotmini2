package app

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spotmini-gui/internal/logging"
)

const popupMode = true

// desktopThemePath is $XDG_STATE_HOME/rice/spotmini.json, where the rice
// puts its generated files.
func desktopThemePath() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "rice", "spotmini.json")
}

func readDesktopTheme() DesktopTheme {
	data, err := os.ReadFile(desktopThemePath())
	if err != nil {
		return DesktopTheme{}
	}
	var t DesktopTheme
	if err := json.Unmarshal(data, &t); err != nil {
		logging.Printf("Desktop theme at %s is unreadable: %v", desktopThemePath(), err)
		return DesktopTheme{}
	}
	t.Available = true
	return t
}

// desktopThemePoll is how often the theme file is checked for a change.
// Polled rather than signalled: the rice's reload hook for other apps is
// SIGUSR1, which WebKit's garbage collector already uses in this process.
const desktopThemePoll = 2 * time.Second

// startDesktopThemeWatch loads the theme and keeps it current, telling
// the frontend about each change.
func (a *App) startDesktopThemeWatch() {
	a.setDesktopTheme(readDesktopTheme())

	go func() {
		path := desktopThemePath()
		last := modTime(path)
		for range time.Tick(desktopThemePoll) {
			mod := modTime(path)
			if mod.Equal(last) {
				continue
			}
			last = mod
			t := readDesktopTheme()
			a.setDesktopTheme(t)
			runtime.EventsEmit(a.ctx, "desktop-theme-changed", t)
		}
	}()
}

func modTime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// hyprScale is the focused monitor's scale under Hyprland, 1 anywhere
// else. The popup offsets the desktop gives are in Hyprland's logical
// pixels; this window lives in X11 (see screen_linux.go), where a pixel
// is a device pixel, so they have to be scaled across.
func hyprScale() float64 {
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		return 1
	}
	out, err := exec.Command("hyprctl", "monitors", "-j").Output()
	if err != nil {
		return 1
	}
	var monitors []struct {
		Scale   float64 `json:"scale"`
		Focused bool    `json:"focused"`
	}
	if json.Unmarshal(out, &monitors) != nil {
		return 1
	}
	for _, m := range monitors {
		if m.Focused && m.Scale > 0 {
			return m.Scale
		}
	}
	if len(monitors) > 0 && monitors[0].Scale > 0 {
		return monitors[0].Scale
	}
	return 1
}

// popupRect is where the popup opens: under the top bar of the monitor
// the pointer is on, right-aligned, in GDK coordinates.
func (a *App) popupRect() (x, y, width int, ok bool) {
	px, py, ok := pointerPositionNative()
	if !ok {
		return 0, 0, 0, false
	}
	_, top, right, _, gdkScale, ok := monitorAt(px, py)
	if !ok {
		return 0, 0, 0, false
	}

	offTop, offRight, w := popupDefaultTop, popupDefaultRight, popupDefaultWidth
	if t := a.desktopThemeNow(); t.Available && t.Popup.Width > 0 {
		offTop, offRight, w = t.Popup.Top, t.Popup.Right, t.Popup.Width
	}

	// Desktop logical -> device pixels -> GDK logical.
	f := hyprScale() / float64(max(gdkScale, 1))
	sc := func(v int) int { return int(math.Round(float64(v) * f)) }

	width = sc(w)
	return right - sc(offRight) - width, top + sc(offTop), width, true
}

// togglePopupPanel is togglePanel for popup mode: opening the first
// panel shows the window as a popup, closing the last one hides it,
// and switching between panels leaves it where it is. The height is
// whatever SetPanelHeight asked for last; the frontend measures the new
// panel and corrects it straight after.
func (a *App) togglePopupPanel(panel string) {
	a.panelMu.Lock()
	defer a.panelMu.Unlock()

	wasOpen := a.expandedPanel != ""
	if a.expandedPanel == panel {
		a.expandedPanel = ""
	} else {
		a.expandedPanel = panel
	}

	switch {
	case a.expandedPanel == "":
		runtime.WindowHide(a.ctx)
	case !wasOpen:
		x, y, width, placed := a.popupRect()
		if !placed {
			width = popupDefaultWidth
		}
		_, height := a.windowSize()
		a.setWindowSize(width, height)
		if placed {
			a.setAbsoluteWindowPosition(x, y)
		}
		runtime.WindowShow(a.ctx)
		// Again once mapped: the window manager has its own idea of where
		// a new window goes.
		if placed {
			a.setAbsoluteWindowPosition(x, y)
		}
		a.focusWindow()
	}
	runtime.EventsEmit(a.ctx, "panel-changed", a.expandedPanel)
}

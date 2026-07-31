package hotkeys

import (
	"encoding/json"
	"fmt"
	"os"

	"golang.design/x/hotkey"

	"spotmini-gui/paths"
)

type HotkeyBinding struct {
	Mods []string `json:"mods"`
	Key  string   `json:"key"`
}

type HotkeyConfig struct {
	Settings   HotkeyBinding `json:"settings"`
	PlayPause  HotkeyBinding `json:"playPause"`
	Next       HotkeyBinding `json:"next"`
	Previous   HotkeyBinding `json:"previous"`
	Shuffle    HotkeyBinding `json:"shuffle"`
	Loop       HotkeyBinding `json:"loop"`
	VolumeUp   HotkeyBinding `json:"volumeUp"`
	VolumeDown HotkeyBinding `json:"volumeDown"`
}

var Actions = []string{"settings", "playPause", "next", "previous", "shuffle", "loop", "volumeUp", "volumeDown"}

func defaultConfig() HotkeyConfig {
	mods := []string{"ctrl", "alt"}
	return HotkeyConfig{
		Settings:   HotkeyBinding{Mods: mods, Key: "c"},
		PlayPause:  HotkeyBinding{Mods: mods, Key: "space"},
		Next:       HotkeyBinding{Mods: mods, Key: "right"},
		Previous:   HotkeyBinding{Mods: mods, Key: "left"},
		Shuffle:    HotkeyBinding{Mods: mods, Key: "s"},
		Loop:       HotkeyBinding{Mods: mods, Key: "l"},
		VolumeUp:   HotkeyBinding{Mods: mods, Key: "up"},
		VolumeDown: HotkeyBinding{Mods: mods, Key: "down"},
	}
}

const configPath = "hotkeys.json"

func LoadConfig() HotkeyConfig {
	// Start from the defaults rather than a blank HotkeyConfig{}, so an
	// older saved file that predates a newly added action (like
	// volumeUp/volumeDown) leaves that action at its default binding
	// instead of an empty one - json.Unmarshal only overwrites fields
	// actually present in the saved data.
	cfg := defaultConfig()

	path, err := paths.ConfigFile(configPath)
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig()
	}
	return cfg
}

func SaveConfig(cfg HotkeyConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path, err := paths.ConfigFile(configPath)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c HotkeyConfig) Binding(action string) (HotkeyBinding, bool) {
	switch action {
	case "settings":
		return c.Settings, true
	case "playPause":
		return c.PlayPause, true
	case "next":
		return c.Next, true
	case "previous":
		return c.Previous, true
	case "shuffle":
		return c.Shuffle, true
	case "loop":
		return c.Loop, true
	case "volumeUp":
		return c.VolumeUp, true
	case "volumeDown":
		return c.VolumeDown, true
	}
	return HotkeyBinding{}, false
}

func (c *HotkeyConfig) SetBinding(action string, b HotkeyBinding) bool {
	switch action {
	case "settings":
		c.Settings = b
	case "playPause":
		c.PlayPause = b
	case "next":
		c.Next = b
	case "previous":
		c.Previous = b
	case "shuffle":
		c.Shuffle = b
	case "loop":
		c.Loop = b
	case "volumeUp":
		c.VolumeUp = b
	case "volumeDown":
		c.VolumeDown = b
	default:
		return false
	}
	return true
}

var keyLookup = map[string]hotkey.Key{
	"a": hotkey.KeyA, "b": hotkey.KeyB, "c": hotkey.KeyC, "d": hotkey.KeyD,
	"e": hotkey.KeyE, "f": hotkey.KeyF, "g": hotkey.KeyG, "h": hotkey.KeyH,
	"i": hotkey.KeyI, "j": hotkey.KeyJ, "k": hotkey.KeyK, "l": hotkey.KeyL,
	"m": hotkey.KeyM, "n": hotkey.KeyN, "o": hotkey.KeyO, "p": hotkey.KeyP,
	"q": hotkey.KeyQ, "r": hotkey.KeyR, "s": hotkey.KeyS, "t": hotkey.KeyT,
	"u": hotkey.KeyU, "v": hotkey.KeyV, "w": hotkey.KeyW, "x": hotkey.KeyX,
	"y": hotkey.KeyY, "z": hotkey.KeyZ,
	"space": hotkey.KeySpace,
	"left":  hotkey.KeyLeft,
	"right": hotkey.KeyRight,
	"up":    hotkey.KeyUp,
	"down":  hotkey.KeyDown,
}

// BindingToHotkey builds an unregistered hotkey.Hotkey from a binding.
// A binding with no modifiers is allowed - it registers as a global
// hotkey on that key alone, which means it fires system-wide, even
// while typing that key in another app.
func BindingToHotkey(b HotkeyBinding) (*hotkey.Hotkey, error) {
	key, ok := keyLookup[b.Key]
	if !ok {
		return nil, fmt.Errorf("unknown key %q", b.Key)
	}
	mods := make([]hotkey.Modifier, 0, len(b.Mods))
	for _, m := range b.Mods {
		mod, ok := modifierFromString(m)
		if !ok {
			return nil, fmt.Errorf("unknown modifier %q", m)
		}
		mods = append(mods, mod)
	}
	return hotkey.New(mods, key), nil
}

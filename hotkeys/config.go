package hotkeys

import (
	"encoding/json"
	"fmt"
	"os"

	"golang.design/x/hotkey"
)

type HotkeyBinding struct {
	Mods []string `json:"mods"`
	Key  string   `json:"key"`
}

type HotkeyConfig struct {
	Settings  HotkeyBinding `json:"settings"`
	PlayPause HotkeyBinding `json:"playPause"`
	Next      HotkeyBinding `json:"next"`
	Previous  HotkeyBinding `json:"previous"`
	Shuffle   HotkeyBinding `json:"shuffle"`
}

var Actions = []string{"settings", "playPause", "next", "previous", "shuffle"}

func defaultConfig() HotkeyConfig {
	mods := []string{"ctrl", "alt"}
	return HotkeyConfig{
		Settings:  HotkeyBinding{Mods: mods, Key: "c"},
		PlayPause: HotkeyBinding{Mods: mods, Key: "space"},
		Next:      HotkeyBinding{Mods: mods, Key: "right"},
		Previous:  HotkeyBinding{Mods: mods, Key: "left"},
		Shuffle:   HotkeyBinding{Mods: mods, Key: "s"},
	}
}

const configPath = "hotkeys.json"

func LoadConfig() HotkeyConfig {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return defaultConfig()
	}
	var cfg HotkeyConfig
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
	return os.WriteFile(configPath, data, 0644)
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
// At least one modifier is required so a plain letter key (e.g. "c") can
// never become a global hotkey that hijacks normal typing.
func BindingToHotkey(b HotkeyBinding) (*hotkey.Hotkey, error) {
	if len(b.Mods) == 0 {
		return nil, fmt.Errorf("hotkey needs at least one modifier")
	}
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

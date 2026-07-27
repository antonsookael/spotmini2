package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spotmini-gui/playback"
)

type App struct {
	ctx         context.Context
	accessToken string
}

func NewApp() *App {
	return &App{}
}

// startup runs once the window actually exists. Instead of blocking
// here (which would freeze the window before it even shows), we fetch
// the token in a goroutine and emit an event to the frontend once
// it's ready.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	go func() {
		token := getAccessToken()
		a.accessToken = token
		runtime.EventsEmit(a.ctx, "logged-in")
	}()
}

func (a *App) PlayPause() {
	playing, err := playback.IsPlaying(a.accessToken)
	if err != nil {
		return
	}
	if playing {
		playback.PausePlayback(a.accessToken)
	} else {
		playback.PlayPlayback(a.accessToken)
	}
}

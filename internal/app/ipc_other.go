//go:build !linux

package app

import (
	"errors"

	"spotmini-gui/internal/playback"
)

// SendCommand backs `spotmini -cmd`, which only the Linux build has a
// socket for.
func SendCommand(cmd string) error {
	return errors.New("-cmd only works on Linux, where spotmini has a control socket")
}

// The control socket is Linux only: Windows and macOS keep the window
// as the now-playing strip, and have nothing else on the desktop to
// talk to. See ipc_linux.go.

type ipcServer struct{}

func (a *App) startIPC()                                          {}
func (a *App) stopIPC()                                           {}
func (a *App) ipcPublish(state playback.PlaybackState, err error) {}

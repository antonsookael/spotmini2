//go:build !linux

package app

import "spotmini-gui/internal/playback"

// The control socket is Linux only: Windows and macOS keep the window
// as the now-playing strip, and have nothing else on the desktop to
// talk to. See ipc_linux.go.

type ipcServer struct{}

func (a *App) startIPC()                                          {}
func (a *App) stopIPC()                                           {}
func (a *App) ipcPublish(state playback.PlaybackState, err error) {}

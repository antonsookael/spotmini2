//go:build !darwin

package main

// setFloatOverFullscreen has no implementation outside macOS - the
// "window belongs to one Space and can't be drawn over a fullscreen
// app's Space" problem it works around is macOS-specific, and
// WindowSetAlwaysOnTop alone already floats over fullscreen windows on
// other platforms.
func setFloatOverFullscreen(enabled bool) {}

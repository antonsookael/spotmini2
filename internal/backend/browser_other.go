//go:build !windows

package backend

import (
	"os/exec"
	"runtime"
)

// openURL launches the platform's URL handler: "open" on macOS,
// "xdg-open" on Linux and other unix-likes.
//
// Neither goes through a shell - exec passes the URL as a single
// argument - so its & characters stay part of the URL rather than being
// read as anything, which is the trap the Windows path fell into.
func openURL(url string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}

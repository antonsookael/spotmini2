package backend

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openBrowser launches the system's default browser to the given URL.
// The command differs per OS: Windows needs "cmd /c start", macOS uses
// "open", and Linux typically has "xdg-open".
func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // linux and other unix-likes
		cmd = exec.Command("xdg-open", url)
	}

	if err := cmd.Start(); err != nil {
		fmt.Println("Couldn't open browser automatically:", err)
		fmt.Println("Open this URL manually:", url)
	}
}

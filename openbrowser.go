package main

import (
	"fmt"
	"os/exec"
)

// openBrowser launches the system's default browser to the given URL.
// "start" is a Windows-specific shell command for opening things with
// their default program - on Mac this would be "open", on Linux
// typically "xdg-open".
func openBrowser(url string) {
	cmd := exec.Command("cmd", "/c", "start", "", url)
	if err := cmd.Start(); err != nil {
		fmt.Println("Couldn't open browser automatically:", err)
		fmt.Println("Open this URL manually:", url)
	}
}

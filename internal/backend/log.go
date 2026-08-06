package backend

import (
	"fmt"
	"os"
	"time"

	"spotmini-gui/internal/paths"
)

// logFile is where auth diagnostics go, since the built app runs with
// no console attached - fmt.Println alone would be invisible whenever
// something like a login or refresh failure needs debugging after the
// fact.
const logFile = "spotmini.log"

// logLine appends a timestamped line to logFile and also prints it, so
// it still shows up normally when running via `wails dev` or a
// terminal.
func logLine(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(msg)

	path, err := paths.ConfigFile(logFile)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), msg)
}

// Package logging appends diagnostics to a log file kept alongside the
// app's other per-user data, and mirrors them to stdout.
//
// A built app runs with no console attached, so a plain fmt.Println is
// invisible the moment something needs debugging after the fact - which
// is exactly when it matters. It lives in its own package rather than
// with one of its callers so backend, playback and app can all write to
// the same file without importing each other.
package logging

import (
	"fmt"
	"os"
	"time"

	"spotmini-gui/internal/paths"
)

const logFile = "spotmini.log"

// Printf writes a timestamped line to the log file and prints it too,
// so it still shows up as normal under `wails dev` or a terminal.
//
// Best-effort: if the file can't be opened, the message is still
// printed. Logging failing is never worth failing an action over.
func Printf(format string, args ...interface{}) {
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

// Nib is a small markdown editor for exactly one existing file. pi uses it as
// $EDITOR: the process is the session, and its exit code is the answer.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dyike/keel/ui/el"
	_ "github.com/dyike/keel/ui/highlight" // markdown syntax highlighting
	"github.com/dyike/keel/ui/window"

	"github.com/mystery4f/nib/internal/app"
	"github.com/mystery4f/nib/internal/document"
	"github.com/mystery4f/nib/internal/niblog"
	"github.com/mystery4f/nib/internal/ui"
)

func main() {
	log, err := niblog.Open()
	if err != nil {
		log.Errorf("log file unavailable: %v", err)
	}

	path, err := singlePath(os.Args[1:])
	if err != nil {
		log.Errorf("start: %v", err)
		exit(app.ExitFailed)
	}
	if err := document.Probe(path); err != nil {
		log.Errorf("start: %s cannot be written: %v", path, err)
		exit(app.ExitFailed)
	}
	session, err := app.Open(path, document.OS{})
	if err != nil {
		log.Errorf("start: %v", err)
		exit(app.ExitFailed)
	}
	log.Infof("session start: %s", path)

	view := ui.New(session, log, exit)
	window.Open(window.Options{
		Title:   "nib — " + filepath.Base(path),
		Width:   1040,
		Height:  760,
		Content: el.Root(view),
		OnClose: view.Closed,
	})
	window.Main() // never returns: the process ends with the session's exit code
}

// singlePath enforces the one positional argument the editor takes.
func singlePath(args []string) (string, error) {
	switch len(args) {
	case 1:
		if args[0] == "" {
			return "", fmt.Errorf("empty path")
		}
		return args[0], nil
	case 0:
		return "", fmt.Errorf("usage: nib <file.md>")
	default:
		return "", fmt.Errorf("usage: nib <file.md>: got %d arguments", len(args))
	}
}

// exit is the only way this process ends. Everything that had to reach the disk
// was already written and synced by the command that decided the code.
func exit(code int) {
	os.Exit(code)
}

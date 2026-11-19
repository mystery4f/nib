package app

import (
	"errors"
	"fmt"

	"github.com/mystery4f/nib/internal/document"
)

// App is the session's domain object: one document, one exit code.
type App struct {
	doc        *document.Document
	confirming bool
	notice     string
	code       int
	done       bool
}

// Open loads path and returns the session before any window exists.
func Open(path string, files document.FS) (*App, error) {
	if path == "" {
		return nil, errors.New("no file given")
	}
	doc, err := document.Load(path, files)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &App{doc: doc}, nil
}

// Handle applies one command. Commands arriving after the session ended are
// ignored, so a second save or a second quit cannot change the exit code.
func (a *App) Handle(c Command) {
	if a.done {
		return
	}
	switch c.Kind {
	case EditText:
		if a.confirming {
			return
		}
		a.doc.SetText(c.Text)
		a.notice = ""
	case Save:
		a.save()
	case SaveAndQuit:
		if a.save() {
			a.finish(ExitAccepted)
		}
	case Quit:
		if a.doc.Dirty() {
			a.confirming = true
			return
		}
		a.finish(ExitAccepted)
	case DiscardConfirmed:
		if a.confirming {
			a.finish(ExitDiscarded)
		}
	case ReturnToEditing:
		a.confirming = false
		a.notice = ""
	}
}

// save reports whether the document is on disk now.
func (a *App) save() bool {
	if err := a.doc.Save(); err != nil {
		a.notice = "save failed: " + err.Error()
		return false
	}
	a.notice = "saved"
	return true
}

func (a *App) finish(code int) {
	a.code = code
	a.done = true
	a.confirming = false
}

// View snapshots the session for the renderer.
func (a *App) View() View {
	return View{
		Path:       a.doc.Path(),
		Text:       a.doc.Text(),
		Dirty:      a.doc.Dirty(),
		Confirming: a.confirming,
		Notice:     a.notice,
	}
}

// ExitCode reports the code to exit with and whether the session has ended.
func (a *App) ExitCode() (int, bool) { return a.code, a.done }

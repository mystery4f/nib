// Package ui projects the session onto Keel: it renders the current View and
// turns keys and buttons into commands. It holds no document state of its own.
package ui

import (
	"path/filepath"
	"time"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/markdown"

	"github.com/mystery4f/nib/internal/app"
	"github.com/mystery4f/nib/internal/preview"
)

// Logger is the slice of the session log the UI uses.
type Logger interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
	Path() string
}

// previewKey carries the edit version, so a keystroke restarts the debounce
// timer: Keel cancels a timer that a frame stops declaring.
type previewKey struct{ edited uint64 }

// Session is the window content and the only place the app meets Keel.
type Session struct {
	app    *app.App
	editor *kit.CodeEditorView
	doc    *markdown.Doc

	edited   uint64
	rendered uint64
	lastEdit time.Time
	forced   bool
	split    bool

	exit    func(int)
	log     Logger
	logPath string
}

// New wires the editor to the session. exit is the only way out of the process.
func New(a *app.App, log Logger, exit func(int)) *Session {
	v := a.View()
	s := &Session{app: a, doc: markdown.New(v.Text), split: true, exit: exit, log: log, logPath: log.Path()}
	s.editor = kit.CodeEditor(v.Text).
		Language("markdown").
		Name(filepath.Base(v.Path)).
		Fill().
		Searchable(true).
		SoftWrap(true).
		OnChange(s.textChanged)
	s.editor.Focus()
	return s
}

// Render builds the frame. Keel calls it on the render goroutine.
func (s *Session) Render(cx *el.Context) el.Element {
	v := s.app.View()
	s.bindKeys(cx, v)
	s.refreshIfDue(cx)
	return el.Div().HFull().WFull().Col().Child(
		s.header(cx, v),
		s.panes(cx, v),
		s.conclude(cx, v),
	)
}

// Closed runs when the window is gone. Keel cannot intercept the close button,
// so a forced close with unsaved changes is a discard, never a silent save.
func (s *Session) Closed() {
	if code, done := s.app.ExitCode(); done {
		s.exit(code)
	}
	if s.app.View().Dirty {
		s.log.Infof("window closed with unsaved changes, exit code %d", app.ExitDiscarded)
		s.exit(app.ExitDiscarded)
	}
	s.log.Infof("window closed, exit code %d", app.ExitAccepted)
	s.exit(app.ExitAccepted)
}

func (s *Session) textChanged(text string) {
	s.app.Handle(app.Edit(text))
	s.edited++
	s.lastEdit = time.Now()
}

func (s *Session) dispatch(kind app.Kind) {
	s.app.Handle(app.Command{Kind: kind})
	if kind == app.Save {
		s.forced = true
	}
	if code, done := s.app.ExitCode(); done {
		s.log.Infof("session ended with exit code %d", code)
		s.exit(code)
	}
}

func (s *Session) togglePane() { s.split = !s.split }

func (s *Session) bindKeys(cx *el.Context, v app.View) {
	cx.Shortcut("mod+s", func() { s.dispatch(app.Save) })
	cx.Shortcut("mod+enter", func() { s.dispatch(app.SaveAndQuit) })
	cx.Shortcut("mod+q", func() { s.dispatch(app.Quit) })
	cx.Shortcut("f9", s.togglePane)
	if v.Confirming {
		cx.Shortcut("esc", func() { s.dispatch(app.ReturnToEditing) })
	}
}

// refreshIfDue rebuilds the rendered document once the source has been idle for
// the debounce, or immediately after a save.
func (s *Session) refreshIfDue(cx *el.Context) {
	if s.edited == s.rendered {
		return
	}
	if preview.Stale(s.edited, s.rendered, time.Since(s.lastEdit), s.forced) {
		s.forced = false
		s.refreshPreview()
		return
	}
	cx.After(previewKey{s.edited}, preview.Debounce, s.refreshPreview)
}

func (s *Session) refreshPreview() {
	if s.edited == s.rendered {
		return
	}
	s.doc.SetSource(s.app.View().Text)
	s.rendered = s.edited
}

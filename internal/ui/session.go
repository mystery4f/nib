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
	"github.com/mystery4f/nib/internal/blocks"
	"github.com/mystery4f/nib/internal/preview"
)

// Logger is the slice of the session log the UI uses.
type Logger interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
	Path() string
}

// mode is which of the three views the window shows.
type mode int

const (
	// modeRendered shows the document as it renders; the block under the last
	// click is the only editable one.
	modeRendered mode = iota
	// modeSplit shows the source beside the rendered document.
	modeSplit
	// modeSource shows the source alone.
	modeSource
)

// previewKey carries the edit version, so a keystroke restarts the debounce
// timer: Keel cancels a timer that a frame stops declaring.
type previewKey struct{ edited uint64 }

// Session is the window content and the only place the app meets Keel.
type Session struct {
	app    *app.App
	editor *kit.CodeEditorView // source pane, and the only editor in source mode
	doc    *markdown.Doc       // rendered document for the split view

	mode mode

	// The active block owns [activeStart, activeEnd) of the source and is shown
	// as an editor instead of its rendered form. Its start is fixed while it is
	// active, so an edit never has to move the caret.
	active                 *kit.CodeEditorView
	activeStart, activeEnd int
	// The block slice carries blank lines at its edges. The editor shows the
	// text without them so the caret crosses into the next block on the first
	// try; every edit puts them back unchanged.
	activeLead, activeTrail string
	// activeKind is the block's Markdown kind, and activeBody the text the
	// editor holds without those edges; Enter compares against it.
	activeKind, activeBody string
	// suppress silences the editor's change callback while the session rewrites
	// the editor's own value, as it does when Enter starts a new list item.
	suppress bool
	// span is the one inline span opened for editing in place; nil when the
	// document is only rendered or a whole block is open.
	span *spanEdit

	sliced   []blocks.Block
	sliceKey string
	docs     []*markdown.Doc
	docsKey  string

	edited   uint64
	rendered uint64
	lastEdit time.Time
	// pendingMove is an arrow that could not move the caret inside the active
	// block (-1 up, +1 down): the next frame opens the block beside it.
	pendingMove    int
	pendingAtStart bool
	// A new block editor cannot take focus while the frame that builds it is
	// still running: its widget does not exist yet. focusArmed marks that frame,
	// focusNext the one after, which is where Focus() lands on a live widget.
	focusArmed, focusNext bool
	forced                bool

	exit    func(int)
	log     Logger
	logPath string
}

// New wires the editor to the session. exit is the only way out of the process.
func New(a *app.App, log Logger, exit func(int)) *Session {
	v := a.View()
	s := &Session{app: a, doc: markdown.New(v.Text), mode: modeRendered, exit: exit, log: log, logPath: log.Path()}
	s.editor = kit.CodeEditor(v.Text).
		Language("markdown").
		Name(filepath.Base(v.Path)).
		Fill().
		Searchable(true).
		SoftWrap(true).
		OnChange(s.textChanged)
	s.editor.Focus()
	s.activate(len(v.Text)) // the document opens ready to type at its end
	return s
}

// Render builds the frame. Keel calls it on the render goroutine.
func (s *Session) Render(cx *el.Context) el.Element {
	v := s.app.View()
	s.bindKeys(cx, v)
	s.refreshIfDue(cx)
	if s.focusNext {
		s.focusNext = false
		if s.active != nil {
			s.active.Focus()
		}
	}
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
	s.closeSpan()
	s.app.Handle(app.Command{Kind: kind})
	if kind == app.Save {
		s.forced = true
	}
	if code, done := s.app.ExitCode(); done {
		s.log.Infof("session ended with exit code %d", code)
		s.exit(code)
	}
}

// cycleView moves to the next view: rendered, split, source.
func (s *Session) cycleView() {
	s.closeSpan()
	s.deactivate()
	switch s.mode {
	case modeRendered:
		s.mode = modeSplit
		s.editor.SetValue(s.app.View().Text)
		s.editor.Focus()
	case modeSplit:
		s.mode = modeSource
	default:
		s.mode = modeRendered
	}
}

func (s *Session) bindKeys(cx *el.Context, v app.View) {
	cx.Shortcut("mod+s", func() { s.dispatch(app.Save) })
	cx.Shortcut("mod+enter", func() { s.dispatch(app.SaveAndQuit) })
	cx.Shortcut("mod+q", func() { s.dispatch(app.Quit) })
	cx.Shortcut("f9", s.cycleView)
	switch {
	case v.Confirming:
		cx.Shortcut("esc", func() { s.dispatch(app.ReturnToEditing) })
	case s.span != nil:
		cx.Shortcut("esc", s.discardSpan)
	case s.active != nil:
		cx.Shortcut("esc", s.deactivate)
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

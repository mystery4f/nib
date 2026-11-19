package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"

	"github.com/mystery4f/nib/internal/app"
)

// header carries the file name and the modified marker. Keel fixes the window
// title when the window opens, so the marker lives inside the window instead.
func (s *Session) header(cx *el.Context, v app.View) el.Element {
	return el.Div().Row().Items(el.Center).Gap(10).Px(16).Py(10).Bg(theme.Surface).Child(
		el.Text(filepath.Base(v.Path)).TextSize(theme.TextSm).Bold(),
		el.Text(modifiedMark(v)).TextSize(theme.TextSm).TextColor(theme.Muted),
		el.Div().Grow(),
		kit.Button(s.viewLabel(), s.cycleView).Render(cx),
	)
}

// panes draws the view the session is in.
func (s *Session) panes(cx *el.Context, v app.View) el.Element {
	switch s.mode {
	case modeRendered:
		return s.renderedPane(cx, v)
	case modeSource:
		return el.Div().Grow().Row().Items(el.Stretch).Child(
			el.Div().Grow().Child(s.editor.Render(cx)),
		)
	default:
		return el.Div().Grow().Row().Items(el.Stretch).Child(
			el.Div().Grow().Child(s.editor.Render(cx)),
			el.Div().W(el.Dp(1)).Bg(theme.Border),
			el.Div().Grow().ScrollY().P(20).Child(s.doc.Render(cx)),
		)
	}
}

// conclude stacks the confirmation bar over the status bar.
func (s *Session) conclude(cx *el.Context, v app.View) el.Element {
	if !v.Confirming {
		return s.statusBar(v)
	}
	return el.Div().Col().Child(s.confirmationBar(cx), s.statusBar(v))
}

// confirmationBar is the in-window replacement for a message box: Keel has none.
func (s *Session) confirmationBar(cx *el.Context) el.Element {
	return el.Div().Row().Items(el.Center).Gap(10).Px(16).Py(10).Bg(theme.Surface).
		Border(1, theme.Border).Child(
		el.Text("Save changes before closing?").TextSize(theme.TextSm).Bold(),
		el.Div().Grow(),
		kit.Button("Save & close", func() { s.dispatch(app.SaveAndQuit) }).Render(cx),
		kit.Button("Discard", func() { s.dispatch(app.DiscardConfirmed) }).Render(cx),
		kit.Button("Keep editing", func() { s.dispatch(app.ReturnToEditing) }).Render(cx),
	)
}

func (s *Session) statusBar(v app.View) el.Element {
	return el.Div().Row().Items(el.Center).Gap(16).Px(16).Py(6).Bg(theme.Surface).Child(
		el.Text(metrics(v.Text)).TextSize(theme.TextXs).TextColor(theme.Muted),
		el.Text(s.blockHint(v)).TextSize(theme.TextXs).TextColor(theme.Muted),
		el.Div().Grow(),
		el.Text(v.Notice).TextSize(theme.TextXs).TextColor(theme.Muted),
		el.Text("log "+s.logPath).TextSize(theme.TextXs).TextColor(theme.Muted),
		el.Text("Ctrl+S save · Ctrl+Enter save & close · Ctrl+Q close · F9 view").
			TextSize(theme.TextXs).TextColor(theme.Muted),
	)
}

func modifiedMark(v app.View) string {
	if !v.Dirty {
		return ""
	}
	return "● unsaved"
}

func metrics(text string) string {
	lines := 1
	if text != "" {
		lines = strings.Count(text, "\n") + 1
	}
	return fmt.Sprintf("%d lines · %d chars", lines, utf8.RuneCountInString(text))
}

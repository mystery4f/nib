package ui

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/theme"

	"github.com/mystery4f/nib/internal/app"
	"github.com/mystery4f/nib/internal/blocks"
)

// renderedPane shows the whole document rendered, with the active block opened
// as source in the place where it renders.
func (s *Session) renderedPane(cx *el.Context, v app.View) el.Element {
	bs := s.documentBlocks(v.Text)
	if s.pendingMove != 0 {
		s.moveAcrossBlock(v, bs)
		bs = s.documentBlocks(v.Text)
	}
	docs := s.blockDocuments(v.Text, bs)
	if s.active == nil && len(bs) == 0 {
		s.activate(0) // an empty document still has to be typeable
	}
	page := el.Div().Col().WFull().P(24).Gap(4)
	placed := s.active == nil
	for i, b := range bs {
		if s.active != nil && blocks.Overlaps(b, s.activeStart, s.activeEnd) {
			if !placed {
				page.Child(s.blockEditor(cx))
				placed = true
			}
			continue
		}
		if !placed && b.Start >= s.activeEnd {
			page.Child(s.blockEditor(cx))
			placed = true
		}
		start := b.Start
		page.Child(el.Div().WFull().OnClick(func() { s.activate(start) }).Child(docs[i].Render(cx)))
	}
	if !placed {
		page.Child(s.blockEditor(cx))
	}
	return el.Div().Grow().WFull().ScrollY().Child(page)
}

// blockEditor renders the active block as source. Its height follows the number
// of lines: it sits inside the document's own scroll, so it must not fill.
func (s *Session) blockEditor(cx *el.Context) el.Element {
	s.active.Height(blockHeight(s.active.Lines()))
	root := el.Div().WFull().Child(s.active.Render(cx))
	if s.focusArmed {
		s.focusArmed, s.focusNext = false, true
	}
	return root
}

// blockHeight is how tall an editor over lines lines has to be to show all of
// them: Keel's editor draws a row at 1.5 times the body text size, plus its own
// top and bottom padding.
func blockHeight(lines int) float32 {
	if lines < 1 {
		lines = 1
	}
	return float32(lines)*theme.TextMd*1.5 + 10
}

// activate opens the block holding offset as source, caret at its end.
func (s *Session) activate(offset int) { s.activateAt(offset, false) }

// activateAt opens the block holding offset as source. atStart puts the caret
// before the block's first character instead of after its last, which is where
// a caret arriving from above belongs.
func (s *Session) activateAt(offset int, atStart bool) {
	v := s.app.View()
	bs := s.documentBlocks(v.Text)
	start, end := 0, len(v.Text)
	switch i := blocks.IndexAt(bs, offset); {
	case i >= 0:
		start, end = bs[i].Start, bs[i].End
	case len(bs) > 0:
		last := bs[len(bs)-1]
		start, end = last.Start, last.End
	}
	kind := "document"
	if i := blocks.IndexAt(bs, start); i >= 0 {
		kind = bs[i].Kind
	}
	s.activeStart, s.activeEnd = start, end
	lead, body, trail := blocks.SplitBlankEdges(v.Text[start:end])
	s.activeLead, s.activeTrail = lead, trail
	s.active = kit.CodeEditor(body).
		Language("markdown").
		Name("block " + kind).
		SoftWrap(true).
		OnChange(s.blockChanged).
		OnBoundary(s.boundary)
	line, col := caretAtEnd(body)
	if atStart {
		line, col = 0, 0
	}
	s.active.SetCursor(line, col)
	s.active.Focus()
	s.focusArmed, s.focusNext = true, false
}

// boundary records an arrow key the caret could not follow: the session switches
// block on the next frame, because this one's element tree is about to be
// discarded and must not be changed from inside a callback.
func (s *Session) boundary(dir int) {
	s.pendingMove, s.pendingAtStart = dir, dir > 0
}

// moveAcrossBlock consumes a recorded boundary key and opens the block above or
// below the active one. At either end of the document it does nothing.
func (s *Session) moveAcrossBlock(v app.View, bs []blocks.Block) {
	dir, atStart := s.pendingMove, s.pendingAtStart
	s.pendingMove = 0
	i := blocks.IndexAt(bs, s.activeStart)
	if i < 0 || i+dir < 0 || i+dir >= len(bs) {
		return
	}
	s.activateAt(bs[i+dir].Start, atStart)
}

// caretAtEnd reports the line and column where an activated block starts
// typing: the end of its last non-empty line, so new text appends instead of
// landing before a ">" or "#" marker.
func caretAtEnd(text string) (line, col int) {
	trimmed := strings.TrimRight(text, "\n")
	line = strings.Count(trimmed, "\n")
	if i := strings.LastIndexByte(trimmed, '\n'); i >= 0 {
		col = utf8.RuneCountInString(trimmed[i+1:])
	} else {
		col = utf8.RuneCountInString(trimmed)
	}
	return line, col
}

// deactivate closes the active block and leaves the document fully rendered.
func (s *Session) deactivate() {
	s.active, s.activeStart, s.activeEnd = nil, 0, 0
	s.activeLead, s.activeTrail = "", ""
}

// blockChanged writes the block's text back into the document. The block's
// start does not move while it is active, so the caret in the editor stays
// where the user put it.
func (s *Session) blockChanged(text string) {
	if s.active == nil {
		return
	}
	text = s.activeLead + text + s.activeTrail
	s.app.Handle(app.Edit(blocks.Replace(s.app.View().Text, s.activeStart, s.activeEnd, text)))
	s.activeEnd = s.activeStart + len(text)
	s.edited++
	s.lastEdit = time.Now()
}

// documentBlocks slices the source, remembering the last result: a frame that
// changed nothing reuses it.
func (s *Session) documentBlocks(text string) []blocks.Block {
	if s.sliceKey != text {
		s.sliced, s.sliceKey = blocks.Slice(text), text
	}
	return s.sliced
}

// blockDocuments renders every block on its own. The document's link-reference
// definitions are prefixed onto each, or a block using [label] would render as
// literal text.
func (s *Session) blockDocuments(text string, bs []blocks.Block) []*markdown.Doc {
	if s.docsKey == text && len(s.docs) == len(bs) {
		return s.docs
	}
	defs := blocks.Definitions(text)
	docs := make([]*markdown.Doc, len(bs))
	for i, b := range bs {
		docs[i] = markdown.New(defs + text[b.Start:b.End])
	}
	s.docs, s.docsKey = docs, text
	return docs
}

func (s *Session) viewLabel() string {
	switch s.mode {
	case modeRendered:
		return "View: rendered"
	case modeSplit:
		return "View: split"
	default:
		return "View: source"
	}
}

// blockHint tells how to get in or out of editing a block. A pending close
// question owns Esc, so the hint goes quiet then.
func (s *Session) blockHint(v app.View) string {
	if s.mode != modeRendered || v.Confirming {
		return ""
	}
	if s.active == nil {
		return "Click a block to edit it"
	}
	return "Esc closes the block"
}

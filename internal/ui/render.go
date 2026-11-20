package ui

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dyike/keel/gio/f32"
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
		body := docs[i]
		if s.span != nil && s.span.blockStart == start {
			body = s.spanDocument(v.Text, b)
		}
		// A paragraph or a heading reports its inline spans, so its own text
		// decides what opens; a code block, a table, a quote or a list has no
		// spans and opens whole. The row remembers where the pointer sits, so
		// the click that follows puts the caret on the character it landed on
		// rather than at the end of the block.
		row := el.Div().WFull().OnPointerMove(func(x, y float32) { s.trackPointer(start, x, y) })
		if b.Kind != "paragraph" && b.Kind != "heading" {
			row.OnClick(func() { s.clickBlock(start) })
		}
		page.Child(row.Child(body.Render(cx)))
	}
	if !placed {
		page.Child(s.blockEditor(cx))
	}
	return el.Div().Grow().WFull().ScrollY().Child(page)
}

// blockEditor renders the active block as source. Its height follows the number
// of lines: it sits inside the document's own scroll, so it must not fill. A
// block taller than the window is capped there instead, so the editor scrolls
// itself to the caret rather than growing past the bottom of the screen.
func (s *Session) blockEditor(cx *el.Context) el.Element {
	_, window := cx.ViewportSize()
	s.active.Height(blockHeight(s.active.Lines(), window))
	root := el.Div().WFull().Child(s.active.Render(cx))
	if s.focusArmed {
		s.focusArmed, s.focusNext = false, true
	}
	return root
}

// blockHeight is how tall an editor over lines lines has to be to show all of
// them: Keel's editor draws a row at 1.5 times the body text size, plus its own
// top and bottom padding.
func blockHeight(lines int, window float32) float32 {
	if lines < 1 {
		lines = 1
	}
	height := float32(lines)*theme.TextMd*1.5 + 10
	// Leave the header and the status bar on screen: the caret has to stay
	// visible while the block is being typed, however long it gets.
	if limit := window - 140; height > limit {
		height = max(limit, 120)
	}
	return height
}

// activate opens the block holding offset as source, caret at its end.
func (s *Session) activate(offset int) { s.activateAt(offset, false) }

// activateAt opens the block holding offset as source. atStart puts the caret
// before the block's first character instead of after its last, which is where
// a caret arriving from above belongs.
func (s *Session) activateAt(offset int, atStart bool) {
	s.closeSpan()
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
	caret := -1
	if atStart {
		caret = start
	}
	s.openBlock(start, end, kind, caret)
}

// openBlock shows [start, end) of the source as the block being edited. The
// range is passed in rather than looked up because a paragraph broken off by
// Enter is an empty range with no parsed block of its own.
func (s *Session) openBlock(start, end int, kind string, caret int) {
	v := s.app.View()
	lead, body, trail := blocks.SplitBlankEdges(v.Text[start:end])
	s.activeStart, s.activeEnd = start, end
	s.activeKind, s.activeBody = kind, body
	s.activeLead, s.activeTrail = lead, trail
	s.active = kit.CodeEditor(body).
		Language("markdown").
		Name("block " + kind).
		SoftWrap(true).
		OnChange(s.blockChanged).
		OnBoundary(s.boundary)
	line, col := caretAtEnd(body)
	if caret >= 0 {
		line, col = caretAt(body, caret-start-len(lead))
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

// caretAt reports the line and column of byte offset off in text, which is
// where a click inside the block puts the caret.
func caretAt(text string, off int) (line, col int) {
	off = min(max(off, 0), len(text))
	line = strings.Count(text[:off], "\n")
	col = utf8.RuneCountInString(text[strings.LastIndexByte(text[:off], '\n')+1 : off])
	return line, col
}

// trackPointer remembers where the pointer sits inside a rendered block, in
// that block's own pixels, so the click that follows can put the caret on the
// character it landed on.
func (s *Session) trackPointer(start int, x, y float32) {
	scale := s.scale
	if scale <= 0 {
		scale = 1
	}
	s.pointer = f32.Point{X: x * scale, Y: y * scale}
	s.pointerBlock, s.pointerSet = start, true
}

// clickBlock opens the block a click landed on, with the caret on the character
// under the pointer. A click whose position maps to no source byte opens the
// block at its end, which is where typing continues.
func (s *Session) clickBlock(start int) {
	at, ok := s.pointerOffset(start)
	if !ok {
		s.activate(start)
		return
	}
	s.activateAtCaret(start, at)
}

// pointerOffset maps the last known pointer position over block start to a byte
// of the source. A rendered block carries the document's link-reference
// definitions in front of it, so the offset moves back past them.
func (s *Session) pointerOffset(start int) (int, bool) {
	if !s.pointerSet || s.pointerBlock != start {
		return 0, false
	}
	v := s.app.View()
	bs := s.documentBlocks(v.Text)
	i := blocks.IndexAt(bs, start)
	if i < 0 || i >= len(s.docs) {
		return 0, false
	}
	doc := s.docs[i]
	ranges := doc.SourceRanges()
	rangeIndex, off, ok := nearestSource(doc, s.pointer)
	if !ok || rangeIndex < 0 || rangeIndex >= len(ranges) {
		return 0, false
	}
	at := start + ranges[rangeIndex].Start + off - len(blocks.Definitions(v.Text))
	if at < start {
		at = start
	}
	// A character is several bytes, and a point lands on one of them, so the
	// byte walks back to the start of its rune: a caret cannot split a
	// character, and the column count would be wrong if it did.
	for at > start && at < len(v.Text) && !utf8.RuneStart(v.Text[at]) {
		at--
	}
	return at, true
}

// sourceProbePx bounds how far above and below a click the source is asked
// where the pointer landed: a click between two drawn lines lands on neither.
const sourceProbePx = 60

// nearestSource asks the document for the source byte under pt, then walks up
// and down from pt until a line answers. A click between two lines then puts
// the caret on the line nearest the pointer instead of nowhere.
func nearestSource(doc *markdown.Doc, pt f32.Point) (int, int, bool) {
	if i, off, ok := doc.SourceAt(pt); ok {
		return i, off, true
	}
	for dy := float32(1); dy <= sourceProbePx; dy++ {
		if i, off, ok := doc.SourceAt(f32.Point{X: pt.X, Y: pt.Y - dy}); ok {
			return i, off, true
		}
		if i, off, ok := doc.SourceAt(f32.Point{X: pt.X, Y: pt.Y + dy}); ok {
			return i, off, true
		}
	}
	return 0, 0, false
}

// activateAtCaret opens the block holding offset with the caret on byte at.
func (s *Session) activateAtCaret(offset, at int) {
	s.closeSpan()
	bs := s.documentBlocks(s.app.View().Text)
	i := blocks.IndexAt(bs, offset)
	if i < 0 {
		s.activate(offset)
		return
	}
	s.openBlock(bs[i].Start, bs[i].End, bs[i].Kind, at)
}

// deactivate closes the active block and leaves the document fully rendered.
func (s *Session) deactivate() {
	s.active, s.activeStart, s.activeEnd = nil, 0, 0
	s.activeLead, s.activeTrail = "", ""
	s.activeKind, s.activeBody = "", ""
}

// blockChanged writes the block's text back into the document. The block's
// start does not move while it is active, so the caret in the editor stays
// where the user put it.
func (s *Session) blockChanged(text string) {
	if s.active == nil || s.suppress {
		return
	}
	previous := s.activeBody
	s.activeBody = text
	if at, ok := newlineInserted(previous, text); ok {
		if s.breaksParagraph() {
			s.breakBlockAt(at)
			return
		}
		if s.activeKind == "list" && at == len(text) {
			if next, ok := nextItem(text); ok {
				s.rewriteBlock(next)
				return
			}
		}
	}
	s.writeBlock(text)
}

// rewriteBlock replaces what the editor holds, so a keystroke that also changes
// the text around the caret — Enter starting a new list item — leaves the caret
// where the user expects it. The callback is muted while it runs.
func (s *Session) rewriteBlock(text string) {
	s.suppress = true
	s.activeBody = text
	s.active.SetValue(text)
	line, col := caretAtEnd(text)
	s.active.SetCursor(line, col)
	s.active.Focus()
	s.suppress = false
	s.writeBlock(text)
}

// writeBlock puts the editor's text back into the source, blank edges included.
func (s *Session) writeBlock(text string) {
	text = s.activeLead + text + s.activeTrail
	s.app.Handle(app.Edit(blocks.Replace(s.app.View().Text, s.activeStart, s.activeEnd, text)))
	s.activeEnd = s.activeStart + len(text)
	s.edited++
	s.lastEdit = time.Now()
}

// breaksParagraph reports whether Enter in this block starts a new block rather
// than adding another line: a paragraph or a heading ends, a list item, a quote,
// a code block and a table keep their own newlines.
func (s *Session) breaksParagraph() bool {
	return s.activeKind == "paragraph" || s.activeKind == "heading" || s.activeKind == "document"
}

// breakBlockAt closes the paragraph at at, splitting the block in two: what is
// before stays, a blank line follows it, and what is after becomes the block
// being edited with the caret at its start.
func (s *Session) breakBlockAt(at int) {
	before, after := s.activeBody[:at-1], s.activeBody[at:]
	replacement := s.activeLead + before + "\n\n" + after + s.activeTrail
	s.app.Handle(app.Edit(blocks.Replace(s.app.View().Text, s.activeStart, s.activeEnd, replacement)))
	s.edited++
	s.lastEdit = time.Now()
	start := s.activeStart + len(s.activeLead) + len(before) + 2
	s.openBlock(start, start+len(after), "paragraph", start)
}

// newlineInserted reports the offset just past a newline that prev and next
// differ by, which is how a single Enter shows up in an editor's text.
func newlineInserted(prev, next string) (int, bool) {
	if len(next) != len(prev)+1 {
		return 0, false
	}
	i := 0
	for i < len(prev) && prev[i] == next[i] {
		i++
	}
	if i >= len(next) || next[i] != '\n' {
		return 0, false
	}
	return i + 1, true
}

// nextItem continues the list the caret just left: Enter at the end of "- two"
// starts "- ", the way the reference product does, instead of a lazy line inside
// the same item. Ordered lists count on. Text that does not end with a list item
// is left to the plain newline.
func nextItem(text string) (string, bool) {
	trimmed := strings.TrimRight(text, "\n")
	line := trimmed
	if i := strings.LastIndexByte(trimmed, '\n'); i >= 0 {
		line = trimmed[i+1:]
	}
	indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
	body := line[len(indent):]
	if len(body) >= 2 && (body[0] == '-' || body[0] == '*' || body[0] == '+') && body[1] == ' ' {
		return text + indent + body[:2], true
	}
	digits := 0
	for digits < len(body) && body[digits] >= '0' && body[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits+1 >= len(body) || body[digits+1] != ' ' {
		return "", false
	}
	if body[digits] != '.' && body[digits] != ')' {
		return "", false
	}
	n, err := strconv.Atoi(body[:digits])
	if err != nil {
		return "", false
	}
	return text + indent + strconv.Itoa(n+1) + body[digits:digits+2], true
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
// blockDocuments renders every block on its own. The document's link-reference
// definitions are prefixed onto each, or a block using [label] would render as
// literal text. Front matter is hidden by default — it is metadata, not prose —
// so a document whose first block is metadata keeps it off screen; here it is
// the block being shown, so it asks for the key table.
func (s *Session) blockDocuments(text string, bs []blocks.Block) []*markdown.Doc {
	if s.docsKey == text && len(s.docs) == len(bs) {
		return s.docs
	}
	defs := blocks.Definitions(text)
	docs := make([]*markdown.Doc, len(bs))
	for i, b := range bs {
		docs[i] = markdown.New(defs + text[b.Start:b.End])
		docs[i].OnSpanClick(func(_, k int) { s.spanClicked(i, k) })
		if b.Kind == "front-matter" {
			docs[i].ShowFrontMatter(true)
		}
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
	if s.span != nil {
		return "Typing replaces the whole span · Enter keeps · Esc reverts"
	}
	if s.active == nil {
		return "Click a block to edit it"
	}
	return "Esc closes the block"
}

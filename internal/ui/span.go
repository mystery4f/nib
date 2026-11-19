package ui

import (
	"time"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/theme"

	"github.com/mystery4f/nib/internal/app"
	"github.com/mystery4f/nib/internal/blocks"
)

// spanEdit is one inline span of a paragraph or heading, opened as a small
// input in the place where it renders. The rest of the block stays rendered, so
// only the word or the emphasis being typed into shows its Markdown.
//
// The draft is not written into the document while it is typed: the block keeps
// parsing from the text it had when the span was opened, which is what keeps the
// span findable and the input alive. Closing the span commits it in one edit.
type spanEdit struct {
	blockStart int    // where the block holding the span starts in the source
	start, end int    // the span's bytes in the source
	index      int    // the block's position in the slice, for a second click
	kind       string // the span kind reported by Keel: text, strong, emph, code, link
	draft      string // the Markdown being typed

	root   *el.RootWidget
	doc    *markdown.Doc // the block rendered with this span swapped for the input
	docKey string
	asks   int
}

// spanNodeKind maps a reported span kind to the Goldmark node its plugin has to
// intercept, plus the emphasis depth that tells *x* from **x**. A kind that has
// no inline view is edited as a whole block instead.
func spanNodeKind(kind string) (node ast.NodeKind, level int, ok bool) {
	switch kind {
	case "text":
		return ast.KindText, 0, true
	case "emph":
		return ast.KindEmphasis, 1, true
	case "strong":
		return ast.KindEmphasis, 2, true
	case "code":
		return ast.KindCodeSpan, 0, true
	case "link":
		return ast.KindLink, 0, true
	}
	return ast.KindDocument, 0, false
}

// spanHolds reports whether the node drawn from [nodeStart, nodeStop) is the one
// the span stands for. A text span is exactly that run of source; a markup span
// contains the node's own text, markers included, and an emphasis only matches
// at its own depth.
func spanHolds(kind string, nodeStart, nodeStop, from, to, level int) bool {
	if kind == "text" {
		return nodeStart == from && nodeStop == to
	}
	if nodeStop <= nodeStart || nodeStart < from || nodeStop > to {
		return false
	}
	switch kind {
	case "emph":
		return level == 1
	case "strong":
		return level == 2
	}
	return true
}

// tooLongToReveal reports a span that is better revealed as its whole block: a
// sentence in a single-line input would hide most of itself. Plain prose is one
// run, so it gets the tighter limit; a short marked phrase stays inline.
func tooLongToReveal(kind, src string) bool {
	limit := 48
	if kind == "text" {
		limit = 24
	}
	return utf8.RuneCountInString(src) > limit
}

// nodeInner reports the source range covered by a node's own text, which is what
// tells one emphasis or link from another in the same block. Markers sit outside
// it, so a **bold** node reports the bytes of bold.
func nodeInner(n ast.Node) (start, stop int, ok bool) {
	t, isText := n.(*ast.Text)
	if isText {
		return t.Segment.Start, t.Segment.Stop, true
	}
	first, last := -1, -1
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		text, isText := c.(*ast.Text)
		if !isText {
			return ast.WalkContinue, nil
		}
		if first < 0 {
			first = text.Segment.Start
		}
		last = text.Segment.Stop
		return ast.WalkContinue, nil
	})
	if first < 0 {
		return 0, 0, false
	}
	return first, last, true
}

// spanClicked opens one inline span of block i for editing. A span that cannot
// spanBytes turns a span's document offsets into offsets inside the block,
// dropping the definitions prefix. A span that lands outside the block is not
// this block's span and is refused rather than clamped: slicing a source at a
// guessed offset is how a file gets corrupted.
func spanBytes(sp markdown.SourceSpan, b blocks.Block, base int) (from, to int, ok bool) {
	from, to = sp.Start-base, sp.End-base
	if from < 0 || to > b.End-b.Start || from > to {
		return 0, 0, false
	}
	return from, to, true
}

// paragraphIndex finds the range the block's own text starts at inside a
// document that has the link definitions prefixed. Definitions draw nothing, so
// they are usually no range at all, but counting from the prefix rather than
// assuming index zero keeps the rule true when they do take one.
func paragraphIndex(doc *markdown.Doc, base int) int {
	for i, r := range doc.SourceRanges() {
		if r.Start > base {
			continue
		}
		if r.End > base {
			return i
		}
	}
	if ranges := doc.SourceRanges(); len(ranges) > 0 {
		return len(ranges) - 1
	}
	return -1
}

// stand on its own — synthetic text, a span crossing a line, a kind with no
// inline view — opens the whole block instead, caret where the span starts.
func (s *Session) spanClicked(i, k int) {
	if i < 0 || i >= len(s.docs) || i >= len(s.sliced) {
		return
	}
	if s.span != nil {
		// A press landed while a span is open. The input captures its own text,
		// but a click on the document underneath still reports a span at that
		// paint position; switching on it would overwrite what the user is
		// typing. An open span is closed with esc or enter, never by a click.
		return
	}
	text := s.app.View().Text
	doc := s.docs[i]
	// The block is rendered with the document's link definitions in front of
	// it, so every offset the document reports counts from that prefix, not
	// from the block. Subtracting it is what turns a span into block bytes.
	base := len(blocks.Definitions(text))
	para := paragraphIndex(doc, base)
	if para < 0 {
		return
	}
	spans := doc.SourceSpans(para)
	if k < 0 || k >= len(spans) {
		return
	}
	sp := spans[k]
	b := s.sliced[i]
	from, to, ok := spanBytes(sp, b, base)
	if !ok {
		return
	}
	_, _, known := spanNodeKind(sp.Kind)
	block := text[b.Start:b.End]
	if !known || !doc.SpanEditable(para, k) || tooLongToReveal(sp.Kind, block[from:to]) {
		// A whole sentence as a single-line input would hide most of itself,
		// so a long run is revealed as its block instead.
		s.closeSpan()
		s.activateAt(b.Start+from, false)
		return
	}
	s.deactivate()
	s.closeSpan()
	s.span = &spanEdit{
		index:      i,
		blockStart: b.Start,
		start:      b.Start + from,
		end:        b.Start + to,
		kind:       sp.Kind,
		draft:      block[from:to],
	}
	// The press that got here also handed the keyboard focus to the document's
	// own selection, which wins the FocusCmd race inside this frame. One more
	// frame lets the input ask again, now that nothing contests it.
	core.Update(func() {})
}

// spanDocument renders the block holding the open span with that span replaced
// by its input. The Markdown it was opened on does not change while typing, so
// the document is built once and the input keeps its caret.
func (s *Session) spanDocument(text string, b blocks.Block) *markdown.Doc {
	key := text[b.Start:b.End]
	if s.span.doc != nil && s.span.docKey == key {
		return s.span.doc
	}
	defs := blocks.Definitions(text)
	src := defs + key
	base, from, to := len(defs), s.span.start-b.Start, s.span.end-b.Start
	node, level, _ := spanNodeKind(s.span.kind)
	index := s.span.index
	doc := markdown.New(src)
	doc.OnSpanClick(func(_, k int) { s.spanClicked(index, k) })
	doc.Plugins(markdown.Plugin{
		Local: true,
		Inlines: map[ast.NodeKind]func(ast.Node, []byte) markdown.InlineObject{
			node: func(n ast.Node, _ []byte) markdown.InlineObject {
				start, stop, ok := nodeInner(n)
				depth := level
				if e, isEmphasis := n.(*ast.Emphasis); isEmphasis {
					depth = e.Level
				}
				if !ok || !spanHolds(s.span.kind, start-base, stop-base, from, to, depth) {
					return markdown.InlineObject{}
				}
				return markdown.InlineObject{Text: s.span.draft, Widget: s.spanWidget()}
			},
		},
	})
	s.span.doc, s.span.docKey = doc, key
	return doc
}

// spanWidget is the input standing in for the span, embedded as its own root so
// it can take focus. It is built once per span: Keel asks the plugin factory for
// a widget during parsing, and the same one has to survive the frames that
// follow or the caret would restart at every redraw.
func (s *Session) spanWidget() core.Widget {
	if s.span.root != nil {
		return s.span.root
	}
	s.span.root = el.Embed(el.ViewFunc(func(cx *el.Context) el.Element {
		sp := s.span
		if sp == nil {
			return el.Div()
		}
		// The click that opened this span also handed focus to the element it
		// landed on — a link keeps it, a bold run does not — so the input asks
		// again on every frame until it holds focus itself.
		// Ask every frame: the press that opened this span handed the keyboard
		// to the document's own selection, and cx.Focused is not to be trusted
		// here — it reports this input as focused before it holds the keyboard.
		// cx.Focus only records the wish, so repeating it is free.
		cx.Focus("nib-span")
		// The input is rebuilt every frame, like any other element; Bind keeps
		// the text and the id keeps the caret.
		return el.Input().
			ID("nib-span").
			Bind(&sp.draft).
			SelectOnFocus(true).
			OnSubmit(func(string) { s.closeSpan() }).
			W(el.Dp(spanWidth(sp.draft))).
			H(el.Dp(theme.TextMd * 1.7)).
			Px(6).
			TextSize(theme.TextMd).
			Mono()
	}))
	return s.span.root
}

// closeSpan commits the span's draft into the document and leaves the block
// fully rendered. An untouched draft writes nothing.
func (s *Session) closeSpan() {
	sp := s.span
	if sp == nil {
		return
	}
	s.span = nil
	text := s.app.View().Text
	if sp.start < 0 || sp.end > len(text) || sp.end < sp.start || text[sp.start:sp.end] == sp.draft {
		return
	}
	s.app.Handle(app.Edit(blocks.Replace(text, sp.start, sp.end, sp.draft)))
	s.edited++
	s.lastEdit = time.Now()
}

// spanWidth is how wide the input over text has to be: wide enough to hold it
// without scrolling, short of the line it would otherwise push out.
func spanWidth(text string) float32 {
	w := 0.0
	for _, r := range text {
		if r > 0x2E80 {
			w += float64(theme.TextMd)
		} else {
			w += float64(theme.TextMd) * 0.58
		}
	}
	return float32(min(max(w+24, 64), 620))
}

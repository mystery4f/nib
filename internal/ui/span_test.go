package ui

import (
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"

	"github.com/dyike/keel/ui/markdown"

	"github.com/mystery4f/nib/internal/blocks"
)

func TestSpanNodeKind(t *testing.T) {
	tests := []struct {
		kind  string
		node  ast.NodeKind
		level int
		ok    bool
	}{
		{"text", ast.KindText, 0, true},
		{"emph", ast.KindEmphasis, 1, true},
		{"strong", ast.KindEmphasis, 2, true},
		{"code", ast.KindCodeSpan, 0, true},
		{"link", ast.KindLink, 0, true},
		{"image", ast.KindDocument, 0, false},
		{"math", ast.KindDocument, 0, false},
		{"footnote-ref", ast.KindDocument, 0, false},
	}
	for _, tt := range tests {
		node, level, ok := spanNodeKind(tt.kind)
		if node != tt.node || level != tt.level || ok != tt.ok {
			t.Errorf("spanNodeKind(%q) = %v, %d, %v; want %v, %d, %v", tt.kind, node, level, ok, tt.node, tt.level, tt.ok)
		}
	}
}

func TestSpanHolds(t *testing.T) {
	tests := []struct {
		name                string
		kind                string
		nodeStart, nodeStop int
		from, to            int
		level               int
		want                bool
	}{
		// A text span stands for exactly the run of source it was reported at.
		{"text exact", "text", 10, 14, 10, 14, 0, true},
		{"text wider", "text", 10, 14, 10, 18, 0, false},
		{"text elsewhere", "text", 20, 24, 10, 14, 0, false},
		// A markup span contains the node's own text; its markers sit outside.
		{"strong inside", "strong", 12, 16, 10, 18, 2, true},
		{"strong wrong depth", "strong", 12, 16, 10, 18, 1, false},
		{"emph inside", "emph", 12, 16, 10, 18, 1, true},
		{"emph wrong depth", "emph", 12, 16, 10, 18, 2, false},
		{"link label inside", "link", 11, 15, 10, 30, 0, true},
		{"link label outside", "link", 31, 35, 10, 30, 0, false},
		{"empty node", "code", 12, 12, 10, 18, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := spanHolds(tt.kind, tt.nodeStart, tt.nodeStop, tt.from, tt.to, tt.level); got != tt.want {
				t.Errorf("spanHolds(%q, %d, %d, %d, %d, %d) = %v; want %v",
					tt.kind, tt.nodeStart, tt.nodeStop, tt.from, tt.to, tt.level, got, tt.want)
			}
		})
	}
}

func TestTooLongToReveal(t *testing.T) {
	// Plain prose is one run: past a few words it is revealed as its block.
	if tooLongToReveal("text", "短的词") {
		t.Error("a short run should stay inline")
	}
	if !tooLongToReveal("text", strings.Repeat("字", 25)) {
		t.Error("25 runes of plain text should open the block instead")
	}
	// Markup spans get more room, since their input shows the Markdown too.
	if tooLongToReveal("strong", strings.Repeat("字", 20)) {
		t.Error("a 20 rune emphasis should stay inline")
	}
	if !tooLongToReveal("strong", strings.Repeat("字", 49)) {
		t.Error("49 runes should open the block instead")
	}
}

func TestSpanWidth(t *testing.T) {
	if got := spanWidth(""); got != 64 {
		t.Errorf("spanWidth(\"\") = %v; want the 64 floor", got)
	}
	narrow, wide := spanWidth("bold"), spanWidth("加粗的四个字")
	if wide <= narrow {
		t.Errorf("wide runes measured %v, narrow %v; want the CJK text wider", wide, narrow)
	}
	if got := spanWidth(strings.Repeat("a", 400)); got != 620 {
		t.Errorf("spanWidth(400 a) = %v; want the 620 cap", got)
	}
}

func TestSpanBytesDropTheDefinitionsPrefix(t *testing.T) {
	src := "text **bold** tail\n\n[ref]: https://example.com\n"
	defs := blocks.Definitions(src)
	blocksOf := blocks.Slice(src)
	if len(blocksOf) == 0 {
		t.Fatalf("no blocks in %q", src)
	}
	b := blocksOf[0]
	doc := markdown.New(defs + src[b.Start:b.End])
	para := paragraphIndex(doc, len(defs))
	spans := doc.SourceSpans(para)
	if len(spans) < 2 {
		t.Fatalf("spans = %d, want at least 2", len(spans))
	}
	strong := spans[1]
	if strong.Kind != "strong" {
		t.Fatalf("span 1 = %q, want strong", strong.Kind)
	}
	from, to, ok := spanBytes(strong, b, len(defs))
	if !ok {
		t.Fatalf("span %+v rejected for block %+v", strong, b)
	}
	if got, want := src[b.Start+from:b.Start+to], "**bold**"; got != want {
		t.Fatalf("block bytes = %q, want %q", got, want)
	}
}

func TestSpanBytesRefuseASpanOutsideTheBlock(t *testing.T) {
	b := blocks.Block{Start: 10, End: 20}
	if _, _, ok := spanBytes(markdown.SourceSpan{Start: 0, End: 5}, b, 8); ok {
		t.Fatal("a span inside the definitions prefix was accepted")
	}
	if _, _, ok := spanBytes(markdown.SourceSpan{Start: 0, End: 25}, b, 0); ok {
		t.Fatal("a span past the block was accepted")
	}
}

func TestShiftSpan(t *testing.T) {
	tests := []struct {
		name               string
		start, end         int
		cs, ce             int
		rep                string
		wantStart, wantEnd int
	}{
		{"before the edit", 0, 5, 10, 14, "**bold**", 0, 5},
		{"after a growing edit", 20, 24, 10, 14, "**bolder**", 26, 30},
		{"after a shrinking edit", 20, 24, 10, 14, "*b*", 19, 23},
		{"an equal-length edit moves nothing", 20, 24, 10, 14, "abcd", 20, 24},
		{"the edited span itself", 10, 14, 10, 14, "**b**", 10, 15},
		{"straddling the end", 12, 18, 10, 14, "**b**", 15, 19},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := shiftSpan(tt.start, tt.end, tt.cs, tt.ce, tt.rep)
			if start != tt.wantStart || end != tt.wantEnd {
				t.Errorf("shiftSpan(%d, %d, %d, %d, %q) = %d, %d; want %d, %d",
					tt.start, tt.end, tt.cs, tt.ce, tt.rep, start, end, tt.wantStart, tt.wantEnd)
			}
		})
	}
}

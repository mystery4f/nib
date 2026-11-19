// Package blocks turns a markdown source into the top-level blocks render mode
// edits one at a time.
//
// The boundaries are Keel's: markdown.Doc.SourceRanges reports them, already
// tiling the source, and nothing here re-derives or corrects one. A boundary
// that looks wrong is a Keel defect, not something to patch locally.
package blocks

import (
	"strings"

	"github.com/dyike/keel/ui/markdown"
)

// Block is one top-level block and the source bytes it came from.
type Block struct {
	Kind       string
	Start, End int
}

// Slice reports the blocks of src in source order. The ranges are contiguous
// and both edges sit on line boundaries.
func Slice(src string) []Block {
	ranges := markdown.New(src).SourceRanges()
	out := make([]Block, 0, len(ranges))
	for _, r := range ranges {
		out = append(out, Block{Kind: r.Kind, Start: r.Start, End: r.End})
	}
	return out
}

// Replace returns src with src[start:end] replaced by text. Out-of-range offsets
// are clamped, so a stale range never panics.
func Replace(src string, start, end int, text string) string {
	start, end = clamp(src, start), clamp(src, end)
	if end < start {
		end = start
	}
	var b strings.Builder
	b.Grow(len(src) - (end - start) + len(text))
	b.WriteString(src[:start])
	b.WriteString(text)
	b.WriteString(src[end:])
	return b.String()
}

// Overlaps reports whether the half-open range [start,end) shares a byte with b.
func Overlaps(b Block, start, end int) bool {
	return start < end && b.Start < end && b.End > start
}

// IndexAt reports the block holding the byte at pos, or -1. Text between two
// blocks (blank lines, link-reference definitions) belongs to the block above
// it, so only the very end of the document can fall outside a block.
func IndexAt(bs []Block, pos int) int {
	for i, b := range bs {
		if pos >= b.Start && pos < b.End {
			return i
		}
	}
	return -1
}

// Definitions returns every link-reference definition line in src that is not
// inside a fenced code block, ready to prefix onto a block rendered on its own:
// a paragraph using [label] renders as a link only when the definition that
// labels it is in the same document.
func Definitions(src string) string {
	var defs strings.Builder
	fence := ""
	for _, line := range strings.Split(src, "\n") {
		body := strings.TrimLeft(line, " ")
		indented := len(line)-len(body) <= 3
		if indented {
			if f := fenceOf(body); f != "" {
				switch {
				case fence == "":
					fence = f
				case f[0] == fence[0] && len(f) >= len(fence) && strings.TrimSpace(body[len(f):]) == "":
					fence = ""
				}
				continue
			}
		}
		if fence != "" || !indented {
			continue
		}
		if !isDefinition(body) {
			continue
		}
		defs.WriteString(line)
		defs.WriteByte('\n')
	}
	if defs.Len() == 0 {
		return ""
	}
	defs.WriteByte('\n')
	return defs.String()
}

// fenceOf reports the fence marker starting line, or "".
func fenceOf(line string) string {
	if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
		c := line[0]
		n := 0
		for n < len(line) && line[n] == c {
			n++
		}
		return line[:n]
	}
	return ""
}

// isDefinition reports a link reference definition: "[label]:" with an optional
// up-to-three-space indent, which TrimLeft already removed. The line must parse
// the way CommonMark reads one — a destination with no unescaped blank, then
// nothing but an optional quoted title — so a line that only starts like a
// definition ("[ref]: https://example.com trailing words") stays a paragraph
// instead of resurfacing as text at the top of every block.
func isDefinition(body string) bool {
	if !strings.HasPrefix(body, "[") {
		return false
	}
	closing := strings.IndexByte(body, ']')
	if closing <= 1 {
		return false
	}
	rest := strings.TrimLeft(body[closing+1:], " ")
	if !strings.HasPrefix(rest, ":") {
		return false
	}
	rest = strings.TrimLeft(rest[1:], " \t")
	var ok bool
	if rest, ok = cutDestination(rest); !ok {
		return false
	}
	rest = strings.TrimLeft(rest, " \t")
	if rest == "" {
		return true
	}
	if rest, ok = cutTitle(rest); !ok {
		return false
	}
	return strings.TrimSpace(rest) == ""
}

// cutDestination consumes a link destination and returns what follows it:
// "<...>" or a run with no unescaped blank and balanced parentheses.
func cutDestination(s string) (string, bool) {
	if strings.HasPrefix(s, "<") {
		for i := 1; i < len(s); i++ {
			switch s[i] {
			case '\\':
				i++
			case '>':
				return s[i+1:], true
			}
		}
		return "", false
	}
	if s == "" {
		return "", false
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return "", false
			}
			depth--
		case ' ', '\t':
			return s[i:], depth == 0
		}
	}
	return "", depth == 0
}

// cutTitle consumes a link title: "..." or '...' or (...), on one line.
func cutTitle(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	closing := s[0]
	switch closing {
	case '"', '\'':
	case '(':
		closing = ')'
	default:
		return "", false
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case closing:
			return s[i+1:], true
		}
	}
	return "", false
}

func clamp(src string, pos int) int {
	if pos < 0 {
		return 0
	}
	if pos > len(src) {
		return len(src)
	}
	return pos
}

// SplitBlankEdges cuts the blank lines a block slice carries at its edges, so
// the editor holds only the block's own lines.
func SplitBlankEdges(text string) (lead, body, trail string) {
	start := 0
	for start < len(text) {
		i := strings.IndexByte(text[start:], '\n')
		if i < 0 || strings.TrimSpace(text[start:start+i]) != "" {
			break
		}
		start += i + 1
	}
	end := len(text)
	for end > start {
		j := strings.LastIndexByte(text[:end], '\n')
		if j < start || strings.TrimSpace(text[j+1:end]) != "" {
			break
		}
		end = j
	}
	return text[:start], text[start:end], text[end:]
}

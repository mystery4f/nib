package blocks_test

import (
	"strings"
	"testing"

	"github.com/mystery4f/nib/internal/blocks"
)

const prompt = "# Title\n\npara one\n\n> quote\n>\n> more\n\n```go\nfmt.Println(1)\n\nfmt.Println(2)\n```\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\ntail\n"

func TestSliceTilesTheSource(t *testing.T) {
	got := blocks.Slice(prompt)
	if len(got) == 0 {
		t.Fatal("Slice returned no blocks")
	}
	prev := 0
	for i, b := range got {
		if b.Start != prev {
			t.Fatalf("block %d (%s) starts at %d, want %d", i, b.Kind, b.Start, prev)
		}
		if b.End <= b.Start {
			t.Fatalf("block %d (%s) is empty: %d..%d", i, b.Kind, b.Start, b.End)
		}
		prev = b.End
	}
	if want := len(strings.TrimRight(prompt, "\n")); prev != want {
		t.Fatalf("blocks end at %d, document body ends at %d", prev, want)
	}
}

func TestSliceKeepsFencesInTheCodeBlock(t *testing.T) {
	for i, b := range blocks.Slice(prompt) {
		if b.Kind != "code" {
			continue
		}
		seg := prompt[b.Start:b.End]
		if !strings.HasPrefix(seg, "```go") || !strings.HasSuffix(strings.TrimRight(seg, "\n"), "```") {
			t.Fatalf("block %d code is %q, want the whole fence", i, seg)
		}
		return
	}
	t.Fatal("no code block found")
}

func TestReplaceComposes(t *testing.T) {
	bs := blocks.Slice("one\n\ntwo\n\nthree\n")
	mid := bs[1]
	got := blocks.Replace("one\n\ntwo\n\nthree\n", mid.Start, mid.End, "TWO\n\n")
	if want := "one\n\nTWO\n\nthree\n"; got != want {
		t.Fatalf("Replace = %q, want %q", got, want)
	}
}

func TestReplaceClampsStaleOffsets(t *testing.T) {
	if got := blocks.Replace("abc", 1, 99, "X"); got != "aX" {
		t.Fatalf("Replace with a long end = %q, want %q", got, "aX")
	}
	if got := blocks.Replace("abc", -5, 2, "X"); got != "Xc" {
		t.Fatalf("Replace with a negative start = %q, want %q", got, "Xc")
	}
}

func TestIndexAtFindsTheBlockUnderAnOffset(t *testing.T) {
	bs := blocks.Slice("one\n\ntwo\n")
	if got := blocks.IndexAt(bs, 1); got != 0 {
		t.Fatalf("IndexAt(1) = %d, want 0", got)
	}
	if got := blocks.IndexAt(bs, 5); got != 1 {
		t.Fatalf("IndexAt(5) = %d, want 1", got)
	}
	if got := blocks.IndexAt(bs, len("one\n\ntwo\n")); got != -1 {
		t.Fatalf("IndexAt(end) = %d, want -1", got)
	}
}

func TestOverlaps(t *testing.T) {
	b := blocks.Block{Start: 10, End: 20}
	if !blocks.Overlaps(b, 12, 18) || !blocks.Overlaps(b, 5, 12) || !blocks.Overlaps(b, 18, 30) {
		t.Fatal("Overlaps missed a shared byte")
	}
	if blocks.Overlaps(b, 20, 30) || blocks.Overlaps(b, 0, 10) || blocks.Overlaps(b, 15, 15) {
		t.Fatal("Overlaps reported a shared byte for a disjoint range")
	}
}

func TestDefinitionsCollectOnlyLinkDefinitions(t *testing.T) {
	src := "[a]: https://example.com\n" +
		"text [a] here\n" +
		"  [b]: https://b.example\n" +
		"```\n" +
		"[fenced]: https://nope.example\n" +
		"```\n" +
		"[not a def]\n" +
		"[c]:\n"
	got := blocks.Definitions(src)
	want := "[a]: https://example.com\n  [b]: https://b.example\n\n"
	if got != want {
		t.Fatalf("Definitions = %q, want %q", got, want)
	}
}

func TestDefinitionsRejectAMalformedLine(t *testing.T) {
	// A line that only starts like a definition used to be collected and then
	// rendered as text at the top of every block.
	src := "[ref]: https://example.com trailing words\n" +
		"[b]: /ok \"title\"\n" +
		"[c]: <https://angle.example/x>\n"
	got := blocks.Definitions(src)
	want := "[b]: /ok \"title\"\n[c]: <https://angle.example/x>\n\n"
	if got != want {
		t.Fatalf("Definitions = %q, want %q", got, want)
	}
}
func TestDefinitionsEmptyWithoutAny(t *testing.T) {
	if got := blocks.Definitions("# title\n\nbody\n"); got != "" {
		t.Fatalf("Definitions = %q, want empty", got)
	}
}

func TestSplitBlankEdgesCutsOnlyTheEdges(t *testing.T) {
	for _, tc := range []struct {
		name              string
		text              string
		lead, body, trail string
	}{
		{"no edges", "plain", "", "plain", ""},
		{"blank line after", "| a | b |\n| 1 | 2 |\n\n", "", "| a | b |\n| 1 | 2 |", "\n\n"},
		{"blank line before", "\n| a | b |\n| 1 | 2 |", "\n", "| a | b |\n| 1 | 2 |", ""},
		{"blank lines both sides", "\n\ntext\n\n\n", "\n\n", "text", "\n\n\n"},
		{"all blank", "  \n\n", "  \n\n", "", ""},
		{"inner blank line kept", "a\n\nb\n", "", "a\n\nb", "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lead, body, trail := blocks.SplitBlankEdges(tc.text)
			if lead != tc.lead || body != tc.body || trail != tc.trail {
				t.Fatalf("SplitBlankEdges(%q) = %q, %q, %q; want %q, %q, %q",
					tc.text, lead, body, trail, tc.lead, tc.body, tc.trail)
			}
			if lead+body+trail != tc.text {
				t.Fatalf("edges lost text: %q", lead+body+trail)
			}
		})
	}
}

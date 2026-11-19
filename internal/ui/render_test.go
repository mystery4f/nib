package ui

import "testing"

func TestNextItem(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
		ok   bool
	}{
		{"bullet", "- two\n", "- two\n- ", true},
		{"asterisk keeps its marker", "* one\n", "* one\n* ", true},
		{"plus keeps its marker", "+ one\n", "+ one\n+ ", true},
		{"indented bullet keeps the indent", "  - two\n", "  - two\n  - ", true},
		{"second item in one block", "- one\n- two\n", "- one\n- two\n- ", true},
		{"ordered counts on", "1. one\n", "1. one\n2. ", true},
		{"ordered keeps the paren style", "3) three\n", "3) three\n4) ", true},
		{"indented ordered", "   9. nine\n", "   9. nine\n   10. ", true},
		{"continuation line is not an item", "- one\n  more\n", "", false},
		{"paragraph", "plain\n", "", false},
		{"dash without space is not a marker", "-dash\n", "", false},
		{"empty text", "\n", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := nextItem(tc.text)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (text %q)", ok, tc.ok, tc.text)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewlineInserted(t *testing.T) {
	tests := []struct {
		name     string
		prev     string
		next     string
		wantAt   int
		wantSure bool
	}{
		{"enter in the middle", "ab", "a\nb", 2, true},
		{"enter at the end", "ab", "ab\n", 3, true},
		{"enter at the start", "ab", "\nab", 1, true},
		{"typing a letter", "ab", "abc", 0, false},
		{"pasting a newline and more", "ab", "a\nbc", 0, false},
		{"deleting", "abc", "ab", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			at, ok := newlineInserted(tc.prev, tc.next)
			if ok != tc.wantSure || (ok && at != tc.wantAt) {
				t.Fatalf("newlineInserted(%q, %q) = %d, %v; want %d, %v", tc.prev, tc.next, at, ok, tc.wantAt, tc.wantSure)
			}
		})
	}
}

# ADR 0004 — Render-mode editing is block-granular, not inline

## Context

ADR 0001 deferred "edit in the rendered document" and Nib v1 shipped a source
pane beside a preview. v2 brings the rendered document forward. The question this
ADR settles is how deep the editing goes: character-level inside rendered text
(what Typora does), or block-level (the block under the caret becomes source).

Keel's markdown renderer publishes no mapping from rendered content back to
Markdown source, and has none internally:

- `TextRange` is documented as a UTF-8 byte range **in `RenderedText`, not in the
  source** (`ui/markdown/ranges.go:19`), and `RenderedText` only exists after a
  frame has painted (`ranges.go:42-49`).
- The internal hit test maps a click to a rendered-text offset only
  (`ui/markdown/selection.go:273`); `syncRanges` (`ranges.go:102`) shifts existing
  highlights by common prefix and suffix rather than mapping positions.
- The parsed block struct carries no source offset at all: its `start` field is a
  list's starting ordinal, not a byte position (`ui/markdown/parse.go:39-54`).

So an inline caret would need the rendered glyph run to be mapped back to source
bytes per character. That mapping does not exist, and building it means owning
the parse, the line breaking, the selection model and the IME plumbing — a text
editor engine, not an editor application. The upstream-facing ask is small
(`Doc.Blocks()` with source ranges; a hit test that returns a block and a byte
offset), but a v2 cannot be built on an API that does not exist yet.

## Decision

Editing happens at block granularity. The document stays one source string;
blocks are contiguous byte ranges reported by Keel's `Doc.SourceRanges()` (added
upstream in commit `f42dfea` on the `nib-source-ranges` branch, wired in through a
`replace` in `go.mod`); the active block renders as a `kit.CodeEditor` over its
own range. Clicking a block activates it, `esc` returns it to rendered form, and
the caret lands at the end of the block so typing appends.

`SourceRanges()` returns the ranges from the parse itself, not from a painted
frame, so a document can be sliced the moment it is loaded. Two defects surfaced
while checking the contract and were fixed upstream rather than worked around
here: a fence line was handed to the block above its code block, and a display
math block was split at a blank line.

## Consequences

- Tables, code blocks and lists are edited as source when active — which is the
  only realistic path anyway: `Doc.table` is a read-only render
  (`ui/markdown/render.go:182`) with no editing branch.
- Inline formatting is not editable in place. The document says so, so the
  limitation is a known edge rather than a surprise.
- Blocks keep document-level constructs intact because goldmark decides the
  boundaries: a loose list or a quote spanning blank lines is one block, not
  several.
- Per-block rendering needs the document's link-reference definitions prefixed
  onto each block, and footnote references degrade to literal text in render
  mode.
- The block editor is an ordinary child element inside the document's scroll
  column: `el.Embed` + `LayoutIn` (`ui/el/root.go:215`, `:251`) matters only for
  a view that scrolls itself, and `ScrollY` is on the outer document.
- If a source-offset hit test ever appears upstream, the block model here does
  not change — only how a click picks its offset.

## The caret crosses block edges

Typora reveals the whole paragraph under the caret, not one span of it, so the
block granularity matches the reference product; what it did not have was a caret
that stops dead at the block's first and last line. Keel's
`CodeEditorView.OnBoundary` (`83d1186`) reports those two keys, and the session
opens the neighbouring block on the next frame — above, with the caret at its
end, below, with the caret at its start.

Span-level reveal (only the bolded or linked run turning into source) stays out.
It needs a source→span mapping Keel does not have, plus a round-trip check that
re-renders a candidate span with the document's definitions before showing it,
because synthesized text (footnote markers, `$$` wrappers, rendered image alt)
has no source of its own. Recorded here so the option is not rediscovered from
scratch: a measured day or two, not an afternoon.

## A newline in a paragraph opens the next block

Revealing a whole block made one thing worse than the reference product: the
block under the caret is also the block that grows, so typing `#` three times
then Enter, or writing a long paragraph, turns that whole block into a source
editor and pushes everything else off the screen. Typora never shows a growing
source region — pressing Enter ends the paragraph and the next one starts empty.

So a newline inside a paragraph or a heading now splits the block at the caret:
the text before is written back and stays rendered, the text after becomes the
new active block with the caret at its start. The insertion is `"\n\n"` because
two blocks need a blank line between them to parse as two blocks; without it the
saved file would be one paragraph with a soft break, and the block boundaries
would come back merged on the next slice.

The trigger is the text, not the key: an edit whose length grew by exactly one
and whose new character is `\n` is a newline, which needs no key interception and
keeps working when the text arrives from a paste of one character. Lists, quotes,
fenced code and tables keep the newline as content — a new list item, a new code
line — because there a newline is part of the block, and splitting would make it
impossible to type a second item at all.

The copy of the block that the editor holds is now only a body plus the blank
edges it arrived with; the edges are re-attached on every write, so a split or an
edit never eats the blank lines that separate blocks. And because a code block
can still legitimately grow taller than the window, the editor view is capped at
the window height and scrolls inside itself instead of pushing the document
scrollbar — the caret stays visible either way.

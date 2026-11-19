# Nib v2 — editing inside the rendered document

## What changes against v1

v1 shipped the source pane beside a rendered preview, and ADR 0001 deferred
"edit in the rendered document". This version does that work, at block
granularity: the document is shown rendered, and the block you are in shows its
own Markdown source in place, with everything else staying rendered. The split
view stays as one of the modes.

## Modes

Three modes, cycled by `f9` and by the header button:

- **Render** (default on open): one pane, the document rendered, the active
  block editable in place.
- **Split**: source editor left, rendered preview right — today's layout.
- **Source**: the source editor alone.

## Editing model

The document is always one string — the source. Editing never changes that: the
active block's `CodeEditor` is a window onto a byte range of it.

- Clicking a rendered block makes it active; a block is active until another is
  clicked or `esc` is pressed inside it.
- The active block replaces its rendered form with a `kit.CodeEditor` holding
  that block's source, focused, the caret at the end of its last non-empty line:
  typing appends instead of landing in front of a `#` or `>` marker. On open the
  last block is active for the same reason — the document is ready to type at.
- Typing edits the block's source; on every change the block's byte range is
  rewritten in the document, the blocks are re-sliced, and the active block is
  found again by the caret's byte offset. Because slicing is offset-based, a
  block that grows or shrinks (or splits in two) keeps the caret.
- `esc` inside a block returns it to rendered form. `ctrl+s`, `ctrl+enter` and
  `ctrl+q` keep their v1 meaning and work from any mode.

## Blocks

A block is a contiguous half-open byte range of the source, and the blocks cover
the whole document with no gaps: block *n* runs from its own start to the next
block's start, the last one to the end of the file. Everything between two
blocks belongs to the earlier one, so a saved file is byte-identical to what the
user would have typed in the source pane.

Boundaries come from Keel itself: `Doc.SourceRanges()` — added in Keel commit
`f42dfea` on the `nib-source-ranges` branch, wired in through a `replace` in
`go.mod` — reports every top-level block as `{Kind, Start, End}` with the ranges
already tiling the source. They arrive in source order, each starting where the
one before ended, both edges on line boundaries, the first at 0 and the last at
the end of the file with trailing newlines trimmed. A fenced code block and a
display-math block start at their marker line, so a block's source can be
written back verbatim. Keel parses with its own goldmark configuration
(`extension.GFM`, `extension.Footnote`, its math parsers), so a loose list, a
quote spanning blank lines, an HTML block with blank lines, a fenced block
containing blank lines and a YAML front matter each stay one range, and text
that draws nothing (a link-reference definition, the blank lines between two
blocks) belongs to the block above it.

Nib takes the ranges as given: it does not re-derive boundaries and does not
patch suspicious ones locally. A range that looks wrong is a Keel defect,
reported and fixed upstream — a local correction would silently disagree
with what Keel draws, and the next Keel change would break it. Two defects
were found this way and fixed in `f42dfea`: a fence line leaking to the block
above it, and a display-math block split at a blank line.

## Rendering a block

Each inactive block is rendered by `markdown.New(definitions + blockSource)`,
where `definitions` is every link-reference definition line found in the
document outside code fences. Without that prefix a block that uses
`[label]` shorthand would render as literal text, because the definition lives
in another block. The prefix shifts no offset we use: render-mode highlights and
block ranges are not mixed.

The scan follows CommonMark's own order — `[label]:`, then a destination
(`<...>`, or a run with no unescaped blank and balanced parentheses), then an
optional quoted or parenthesised title, then the end of the line. Anything looser
collects a line like `[ref]: https://example.com trailing words`, which CommonMark
does not read as a definition, and then renders that line as visible text at the
top of every other block. The malformed line stays an ordinary paragraph block
instead, which is what the source says.

Two consequences of per-block rendering, accepted and documented:

- **Footnotes degrade.** `[^1]` references render as literal text in render
  mode, because footnote numbering is document-level. The source is always one
  click away, and the split view shows the real rendering.
- **A block is the largest editable unit.** Inline formatting is not editable in
  place: the caret lives in a source editor, not in rendered glyphs. See ADR
  0004 for why that is a boundary of the framework, not a shortcut.

## Testing

- `blocks` (new package) is pure and table-driven: `Slice` tiles the source, a
  fence stays inside its code block, `Replace` composes and clamps stale offsets,
  `IndexAt` finds the block under an offset, `Overlaps` is half-open, and
  `Definitions` keeps real definition lines (including a title and an angle
  destination), skips fenced ones and rejects a malformed line.
  The app and writer tests stay off disk behind an in-memory filesystem, and
  `Stale` covers the preview refresh policy.
- The `app` state machine does not change, so its tests stay as they are.
- One automation scenario drives the built binary through the Keel automation
  socket: open a multi-block document, click a block, type, press `esc`, save,
  and assert the file bytes and the exit code; then repeat with `ctrl+q` and
  Discard to assert the file is untouched and the exit code is 1.

## Out of scope for v2

Inline (character-level) editing inside rendered text, scroll position memory per
mode, table cell editing, drag to move blocks, and multi-block selection.

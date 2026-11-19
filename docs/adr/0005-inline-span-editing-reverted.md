# ADR 0005 — Inline span editing was attempted and reverted

## Context

ADR 0004 left one door open: block-level reveal is coarse, and Typora-like
editing eventually wants character-level reveal — only the run under the caret
turning into source. Keel added the pieces for it: `Doc.SourceSpans`,
`Doc.SpanEditable` and `Doc.OnSpanClick`, plus the `Plugin.Inlines` hook that
puts a host widget in the middle of a rendered paragraph.

Nib wired those together: a click on an emphasized run, a code span or a link
replaced it with a single-line `el.Input` embedded in the paragraph, `enter`
committed the draft back into the document, `esc` dropped it.

## Decision

The span editor is removed from Nib (commit `f67d097` reverts it) and Keel
reverts the span API. Block-level reveal stays the only editing surface.

## Why

Two independent costs showed up in real use, and neither is cheap to remove:

- **Responsiveness.** Every keystroke in the embedded input rebuilds the
  paragraph's widget tree inside a document that is itself re-rendered, and the
  per-frame focus re-request keeps the whole window repainting. Typing in a span
  feels like the window is dragging behind the keyboard.
- **Hit testing and focus.** A press inside the embedded input still reaches the
  document drawn underneath it, which reports whatever byte sits at that paint
  position; the document also takes keyboard focus on press. Every fix moved the
  problem rather than removing it (a guard that ignores clicks while a span is
  open, a `cx.Focus` re-request each frame, a rebuilt root per span). A wrong
  report silently rewrites the neighbouring span, which is data loss, not a
  blemish.

The block editor has none of this: one `kit.CodeEditor` per active block, focus
granted once, no widget embedded in a re-parsed document.

## Consequences

- Nib's editing surface is: click a block, edit its source in place, `esc` back
  to rendering. No character-level reveal.
- Keel keeps `SourceRanges`, `CodeEditor.OnBoundary` and the range fixes; the
  span API, its tests and its documentation sections are dropped.
- Revisiting this needs a different approach, not a patch — for example
  rendering the whole block into one editable surface that owns the caret
  itself, instead of embedding inputs into the rendered tree.

# Source editor with a projected preview, not a block-level WYSIWYG core

Keel's `ui/markdown` renders at production quality — GFM tables, code cards, native math, streaming optimisations — but exposes no editing surface: `Doc` has `SetSource`/`Append`/cursor-less rendering methods only, its `TextRange` addresses rendered text rather than source offsets, and `SetSource` clears the selection. `kit.CodeEditor`, by contrast, is a finished source editor with multi-cursor, folding, search and a markdown lexer available through the opt-in highlight package. So v1 pairs the two: source on the left, rendering of that source on the right. Real WYSIWYG (typing inside the rendered document) would mean building block editing, rich-text inline editing, table-cell editing and a source↔render offset map — an editor core, not a mini tool.

## Considered Options

- Block-level WYSIWYG built on Keel's `el` and `markdown.Plugins`: closest to the requested experience, but every piece of the gap is missing and none of it is incremental work.
- Single-pane "focus line shows source, other blocks render": approximates Typora, needs the same missing block editing plus caret routing.
- Switching to a different framework with a rich-text editor component: changes the constraint the user set (Keel), and its agent-driven automation is why Keel was chosen.

## Consequences

The words "WYSIWYG" and "preview" must not be used interchangeably in this repo: the preview is a projection that cannot be typed into. Any future request for inline editing is an architecture change, and ADR-0001 is the record that it was considered and declined for v1.

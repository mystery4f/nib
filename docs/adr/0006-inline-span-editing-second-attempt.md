# ADR 0006 — Inline span editing, second attempt

## Context

ADR 0005 removed inline span editing after it felt laggy and mis-reported
clicks. The user asked for it again. Before rewriting anything, both failure
causes were re-examined against the Keel worktree rather than assumed:

- The hit-through was real, but only half of it: a press inside an embedded
  inline object does reach the document's selection handler, because Gio
  dispatches to the ancestor area too. Keel `bc552c1` now breaks the selection
  path for presses that land on an inline object piece, and stays silent for
  `OnSpanClick` inside one.
- The lag was not the framework's per-key work. Keel re-renders only the block
  holding the inline object (~0.1 ms per key at 50 blocks); what Nib added on
  top — a focus re-request on every frame, forcing a redraw of the whole
  window each frame — was the expensive part.

## Decision

Rebuild the span editor on the restored Keel API (`745ee4c`) with three changes
from the first attempt:

- **Focus is asked for once**, in the frame the input is first built. The press
  that opened the span went to the rendered text (the input did not exist
  yet), so the input appears one frame later and takes the keyboard then; with
  the selection handler out of the way, nothing takes it back.
- **A click on another span while one is open commits the open span and opens
  the clicked one**, instead of ignoring the click. The clicked span's byte
  range is resolved against the text as it was rendered, then shifted across
  the commit's replacement (`shiftSpan`), then re-located in the new block
  slice; a range the commit swallowed opens nothing rather than something
  wrong.
- The draft still never touches the document while typing; `enter` commits,
  `esc` reverts, and any session action (save, view switch, block activation)
  commits first.

## Consequences

- The input holds the keyboard for as long as it is open; clicking inside it
  does nothing (and reports no span), clicking another span switches with the
  first edit committed.
- Keel's branch needs `745ee4c` and `bc552c1` (the fork branch `nib-source-ranges`
  carries both).
- ADR 0005 stays as the record of the first attempt's failure; this ADR
  supersedes its "not a patch" conclusion: the fix was one framework commit
  plus removing Nib's own per-frame focus churn, not a new editing surface.

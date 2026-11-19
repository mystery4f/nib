# ADR 0003 — The window close button discards dirty work instead of asking

## Context

Keel gives a window two ways to end: the in-app quit path the application
controls, and the system close button. `window.Options` has `OnClose`, but the
hook only reports that the window is going away — it cannot veto the close, and
`kit.TitleBar`'s close button calls `core.CurrentWindow().Close()` directly.
There is no before-close hook, no `SetTitle`, and no message-box API, so a
confirmation cannot be raised from the close path.

The accept/discard contract with pi needs exactly three outcomes: written (0),
discarded (1), could not start (2). An unhandled close would leave the process
exit code at Go's default 0, which pi would read as "adopt the file" — the worst
possible answer for a document the user never saved.

## Decision

`ctrl+q` is the asker: on a dirty document it raises the in-window confirmation
bar. The close button is the discarder: `OnClose` finishes the session with exit
code 1 when the document is dirty, exit code 0 when it is clean, and logs the
dirty case so it is diagnosable afterwards.

## Consequences

- A stray close loses the edits, by design and non-interactively. The bar cannot
  be shown, so pretending otherwise would mean a stuck window.
- The exit code stays truthful in every path, which is what pi reads.
- If Keel gains a vetoable close hook, ADR 0002's single exit contract lets the
  close path be routed through `Quit` without touching the state machine.

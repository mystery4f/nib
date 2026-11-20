# ADR 0007 — Rewrite on Rust, Tauri and Milkdown

## Status

Accepted. Supersedes ADR 0001 through 0006 and both specs under
`docs/specs/`, which describe the Keel build that this decision deletes.

## Context

The Keel build reached every promise it made — exit codes, atomic save, block
granular editing, inline span editing — but two defects sat under all of them
and neither could be closed from the application side:

- A caret inside text could not be placed without changing the block model.
  Every click had to be mapped through `Doc.SourceAt`, and anything the
  framework drew as an inline object — a list bullet, a footnote marker, a
  table cell — refused the mapping, so clicks there fell back to the end of
  the block.
- Rendered inline widgets were replayed through `op.Record`, and Gio's input
  router does not walk recorded macros. An input box drawn inside a rendered
  paragraph therefore never received the pointer events that move its caret,
  so editing a bold word always meant replacing the whole word.

Both are framework-internal. Working around them meant maintaining a fork of
the GUI toolkit, which is a second project with its own test matrix.

## Decision

Build the window from a browser engine instead of a custom immediate-mode
toolkit:

- **Tauri 2** for the shell: one Rust binary, WebView2 on Windows, native
  window, `app.exit(code)` for the exit contract.
- **React + Vite** for the page.
- **Milkdown Crepe** as the editor: a rich-text markdown editor whose
  document model is the block tree, so a caret inside a bold phrase is an
  ordinary ProseMirror selection rather than a framework extension.

The process contract does not change: one path in, exit `0` accepted, `1`
discarded, `2` could not start, atomic save. What changes is that the editor
now owns the caret, so Nib no longer maps click coordinates to source bytes.

## What the rewrite costs

- Milkdown normalises markdown on load: `-` list markers become `*`, table
  cells get padded. A save must therefore be gated on "the document is dirty",
  not on "the serialised text differs", or a clean file gets rewritten the
  moment it is opened. The Rust side holds that gate (`save_document` and
  `save_and_exit` both return early when `dirty` is false).
- The first save of an edited file writes Milkdown's canonical markdown, not
  a minimal diff against the original. That is a deliberate trade: the
  alternative is a byte-preserving markdown editor, which is what the Keel
  build was trying to be.
- The editor is now a web page, so `nib.exe` needs the WebView2 runtime
  (present on Windows 10/11 by default).

## Consequences for the automation story

The Keel build was driven end-to-end through `keel-mcp`, which could click and
type into the native widget tree. Tauri exposes none of that; the equivalent
path is WebView2's remote debugging port:

```sh
WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222 ./nib.exe doc.md
```

`Runtime.evaluate` reads the DOM and clicks the confirmation strip,
`Input.dispatchKeyEvent` / `Input.insertText` type and press shortcuts. The
three exit paths were verified this way, not by hand.

Note that a screenshot of a Tauri window is blank: the WebView2 surface is
composited by the GPU and neither UI automation nor `PrintWindow` captures it.
To see the page as the user sees it, point a headless browser at the dev server
(`http://localhost:1420`) during `tauri dev`.

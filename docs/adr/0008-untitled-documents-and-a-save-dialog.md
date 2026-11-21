# 0008 — Untitled documents and a save dialog

## Status

Accepted.

## Context

`docs/specs/nib-v1.md` fixed the invocation surface at exactly one positional
argument: no path, no document. That was written for a pure `$EDITOR`, where pi
always hands over a file and nothing else exists.

The 0.1.0 installer broke that assumption. Launching Nib from the Start menu or
a shortcut — the way anyone tries an installed editor — printed
`usage: nib <file.md>` and exited `2`. Release builds carry
`windows_subsystem = "windows"`, so there was no console either: the user saw a
window flash and nothing else. A markdown editor that cannot be opened on its
own is not installable software.

Two shapes were available:

- keep the single-argument contract and ship a stub that does nothing without a
  path;
- let a missing path mean an untitled document, and let the first save ask the
  OS where the file belongs.

## Decision

No argument is an untitled document. The window opens with the title
`nib — untitled`, the editor is empty, and `ctrl+s` (`ctrl+enter`, and
`Save & close` in the close confirmation) opens the native save dialog through
`tauri-plugin-dialog`; the chosen path is adopted by the shell and the title is
updated in place.

More than one argument is still a usage error and still exits `2`. Everything
about the exit contract is unchanged, and the "a clean document is never
rewritten" rule survives intact: the shell rewrites a file only when the page
reports an edit, or when the write is going to a freshly chosen path.

## Consequences

- The installed app works when opened the way a user opens an app.
- `nib.exe` now needs two more things: the dialog plugin, and one permission
  (`dialog:default`) in `src-tauri/capabilities/default.json`.
- `save_document` and `save_and_exit` take an optional path. When one arrives,
  it is adopted before the write and the title follows; when none arrives the
  document writes to the file it already owns and stays untouched if clean.
- An untitled document that is closed while clean exits `0` without writing
  anything, which is what pi expects from "nothing to save".
- The store-on-disk half is unchanged: temp file, sync, rename, exit.

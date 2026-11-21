# Nib

[![ci](https://github.com/mystery4f/nib/actions/workflows/ci.yml/badge.svg)](https://github.com/mystery4f/nib/actions/workflows/ci.yml)

A small markdown editor for one job: being pi's `$EDITOR`. It opens the one file
it is given, edits it rendered — WYSIWYG, the way a word processor does — and on
exit tells the caller, through its exit code, whether the edit was saved or
thrown away. A second view shows the markdown source beside a live rendering for
when the exact bytes matter.

No new-file flow, no tabs, no open dialog, no file browser. One path in, one exit
code out.

## Stack

- **Rust** (`src-tauri/`) — argv, preflight, atomic save, the exit codes, the
  close-confirmation hook.
- **Tauri 2** — the window; WebView2 on Windows.
- **React + Vite** (`src/`) — the page.
- **Milkdown Crepe** — the editor. It owns the caret; Nib does not map clicks to
  source offsets.

`docs/adr/0007-rust-tauri-milkdown-rewrite.md` records why the earlier Keel
build was dropped.

## Build

```sh
npm install
npm run build                    # tsc --noEmit && vite build -> dist/
cargo build --release --manifest-path src-tauri/Cargo.toml
cp src-tauri/target/release/nib.exe nib.exe
```

### Installer and releases

`npm run tauri build` produces the Windows NSIS installer under
`src-tauri/target/release/bundle/nsis/`. Pushing a tag `v*` runs
`.github/workflows/release.yml`, which builds that installer on `windows-latest`
and attaches it to the matching GitHub release. `.github/workflows/ci.yml` runs
the build, prettier, rustfmt, clippy and `cargo test` on every push to `main`.

`cargo build --release` on its own is enough: `src-tauri/Cargo.toml` enables
Tauri's `custom-protocol` feature, so the binary serves the bundled `dist/`
instead of looking for a dev server. A release binary built without that feature
loads `devUrl` and shows an empty window.

For the live-reload loop:

```sh
npx tauri dev                    # vite on :1420 + a debug nib.exe
npx tauri dev -- "C:\path\doc.md"   # the path goes after the double dash
```

## Use with pi

```sh
export EDITOR=/path/to/nib.exe     # or set externalEditor in pi's settings
```

`ctrl+g` in pi's prompt then opens Nib on the prompt's markdown, and pi adopts
the file only when Nib exits `0`.
Started with a path, Nib opens that file. Started with no argument at all it
opens a blank document, and the first save asks the OS for a name — so it also
works from a shortcut or from the Start menu.

## Keys

| Key          | Action                                              |
| ------------ | --------------------------------------------------- |
| `ctrl+s`     | save, stay open                                     |
| `ctrl+enter` | save and close                                      |
| `ctrl+q`     | close (asks first when there are unsaved changes)   |
| `esc`        | dismiss the confirmation strip                      |
| `f9`         | swap between the rendered editor and the split view |

Editing keys follow Typora where Milkdown has no binding of its own: `ctrl+k`
link, `ctrl+t` table, `ctrl+shift+k` code block, `ctrl+shift+q` quote,
`ctrl+shift+`` ` `` inline code, `ctrl+1`..`ctrl+6` heading level,
`ctrl+0` paragraph. `ctrl+b`, `ctrl+i` and `ctrl+e` are the editor's own.

Everything else — bold, lists, tables, code blocks, links — is the editor's own
toolbar and shortcuts.

## Views

- **Edit** — the rendered editor. The caret, the selection and the inline
  formatting all live in Milkdown; what is on screen is what gets saved.
- **Split** — markdown source on the left, a read-only rendering on the right.
  The preview waits for a pause in typing (250 ms) instead of re-rendering on
  every keystroke. `ctrl+s`, `ctrl+enter` and `ctrl+q` work the same in both
  views, and switching views carries the text across.

Pasted or uploaded images are written to `%TEMP%\nib\images\` and the document
keeps the plain file path; only the view turns it into an `asset://` URL.

## Exit codes

| Code | Meaning                                               |
| ---- | ----------------------------------------------------- |
| `0`  | accepted: written, or unmodified and nothing to write |
| `1`  | discarded or cancelled                                |
| `2`  | could not start: bad arguments, unreadable file       |

An unmodified document is never written: Milkdown normalises markdown on load
(`-` becomes `*`, table cells get padded), so a save is gated on the document
being dirty rather than on the serialised text differing.

## Where things are

- `src-tauri/src/main.rs` — argv, preflight, `AppState`, atomic save, the eight
  commands the page calls, the close hook, the single `app.exit`
- `src/main.tsx` — the two views, dirty tracking, key bindings, image paste, the
  confirmation strip
- `src/styles.css` — the chrome around the editor
- `scripts/e2e.mjs` — the end-to-end run against the real binary
- `docs/adr/` — decisions, newest last; `0007` supersedes the Keel-era ones
- `GLOSSARY.md` — the words this project uses

## Tests

`cargo test` covers the parts that do not need a window: argument handling, BOM
stripping, atomic save (and its failure path), image naming. The exit contract
and the views are exercised end-to-end against the real binary through WebView2's
remote debugging port:

```sh
node scripts/e2e.mjs --file .tmp/sample.md --phase clean    # ctrl+enter, nothing edited
node scripts/e2e.mjs --file .tmp/sample.md --phase edit     # ctrl+enter after a typed edit
node scripts/e2e.mjs --file .tmp/sample.md --phase discard  # ctrl+q -> Discard
node scripts/e2e.mjs --file .tmp/sample.md --phase split    # f9, edit in the source pane
node scripts/e2e.mjs --file .tmp/sample.md --phase image    # paste a png, save, temp dir entry
```

Pass `--binary <path>` to point the same checks at an installed build
(`"$LOCALAPPDATA/nib/nib.exe"` after running the installer) instead of the one in
the repository root.

Each run copies the fixture to `<file>.e2e`, drives the page over CDP
(`Runtime.evaluate`, `Input.dispatchKeyEvent`, `Input.insertText`), and asserts
the process exit code plus the file bytes.

`npm run format:check` and `npm run check:rust` (rustfmt + clippy `-D warnings`)
are the other two gates.

A screenshot of a Tauri window comes out blank — WebView2 paints through the
GPU and no window capture sees it. To look at the page, open
`http://localhost:1420` in a headless browser while `tauri dev` runs.

# Nib

A small markdown editor for one job: being pi's `$EDITOR`. It opens the one file
it is given, edits it rendered — WYSIWYG, the way a word processor does — and on
exit tells the caller, through its exit code, whether the edit was saved or
thrown away.

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

## Keys

| Key | Action |
| --- | --- |
| `ctrl+s` | save, stay open |
| `ctrl+enter` | save and close |
| `ctrl+q` | close (asks first when there are unsaved changes) |
| `esc` | dismiss the confirmation strip |

Everything else — bold, lists, tables, code blocks, links — is the editor's own
toolbar and shortcuts.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | accepted: written, or unmodified and nothing to write |
| `1` | discarded or cancelled |
| `2` | could not start: bad arguments, unreadable file |

An unmodified document is never written: Milkdown normalises markdown on load
(`-` becomes `*`, table cells get padded), so a save is gated on the document
being dirty rather than on the serialised text differing.

## Where things are

- `src-tauri/src/main.rs` — argv, preflight, `AppState`, atomic save, the six
  commands the page calls, the close hook, the single `app.exit`
- `src/main.tsx` — mount, dirty tracking, key bindings, confirmation strip
- `src/styles.css` — the chrome around the editor
- `docs/adr/` — decisions, newest last; `0007` supersedes the Keel-era ones
- `GLOSSARY.md` — the words this project uses

## Tests

There is no unit-test suite yet; the exit contract is exercised end-to-end
against the real binary through WebView2's remote debugging port:

```sh
WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222 ./nib.exe doc.md
```

then drive the page over CDP (`Runtime.evaluate`, `Input.dispatchKeyEvent`,
`Input.insertText`) and assert the process exit code and the file bytes. The
three paths verified that way: clean `ctrl+enter` (exit 0, file untouched),
edited `ctrl+enter` (exit 0, file holds the edit), edited `ctrl+q` → Discard
(exit 1, file byte-identical).

A screenshot of a Tauri window comes out blank — WebView2 paints through the
GPU and no window capture sees it. To look at the page, open
`http://localhost:1420` in a headless browser while `tauri dev` runs.

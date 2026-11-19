# Nib

A small markdown editor for one job: being pi's `$EDITOR`. It opens the file you
give it, shows the document rendered, and on close tells pi — through its exit
code — whether the edit was saved or thrown away.

Text is edited where it draws: the block under the caret turns back into its
Markdown source and everything around it stays rendered. The source/preview
split of v1 is still one key away.

No new-file flow, no tabs, no open dialog. One path in, one exit code out.

## Build

```sh
# the build pi should run: GUI subsystem, so no console beside the editor
go build -ldflags "-H=windowsgui" -o nib.exe .

# a console build, for watching panics and exit codes while developing
go build -o nib.exe .
```

`keel build` produces the same thing and additionally stamps an icon and a
version resource; it needs the CLI's own dependencies in the module graph, so it
is worth running only once the app has an icon to show.

`go.mod` replaces `github.com/dyike/keel` with a local Keel worktree
(`../keel-learning/keel`, branch `nib-source-ranges`). Block-granular editing
needs `Doc.SourceRanges()`, which is not released upstream yet; the replace is
the pinned dependency until it is.

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
| `esc` | close the block being edited (in the rendered view) |
| `↑` / `↓` | cross into the block above or below when the caret cannot move |
| `f9` | cycle the view: rendered → split → source |

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | accepted: written, or unmodified and nothing to write |
| `1` | discarded or cancelled |
| `2` | could not start: bad arguments, unreadable file, unwritable parent |

The window's close button cannot ask for confirmation (Keel has no before-close
hook), so closing a dirty document discards it with code `1`. Use `ctrl+q` when
you want to be asked — see `docs/adr/0003-close-button-discards-dirty-work.md`.

## Where things are

- `main.go` — argv, preflight, the single `os.Exit`
- `internal/app` — the state machine (`App`, `Command`, `View`), no UI
- `internal/document` — load, dirty tracking, atomic save
- `internal/preview` — the refresh policy (idle debounce, forced on save)
- `internal/blocks` — Keel's source ranges, and the byte-range edits over them
- `internal/ui` — Keel projection of the app's view: the three views and the
  block editor
- `internal/niblog` — the log file, since the GUI build has no console
- `docs/specs/nib-v1.md`, `docs/specs/nib-v2-render-mode.md` — what each version
  promises
- `docs/adr/` — decisions behind it
- `GLOSSARY.md` — the words this project uses

## Tests

```sh
go test ./...
```

The exit contract is also exercised end-to-end through Keel's automation server:
launch the built binary on a fixture, drive keys, assert the exit code and the
file bytes.

# Nib

A small markdown editor for one job: being pi's `$EDITOR`. It opens the file you
give it, shows the source beside the rendered document, and on close tells pi —
through its exit code — whether the edit was saved or thrown away.

No new-file flow, no tabs, no open dialog. One path in, one exit code out.

## Build

```sh
go build -o nib.exe .
```

On Windows, `keel build` adds the GUI subsystem flags, so no console window
appears next to the editor (the build `keel` ships does this for you):

```sh
go run github.com/dyike/keel/cmd/keel build
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
| `f9` | toggle the source/preview split |

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
- `internal/ui` — Keel projection of the app's view
- `internal/niblog` — the log file, since the GUI build has no console
- `docs/specs/nib-v1.md` — what v1 promises
- `docs/adr/` — decisions behind it
- `GLOSSARY.md` — the words this project uses

## Tests

```sh
go test ./...
```

The exit contract is also exercised end-to-end through Keel's automation server:
launch the built binary on a fixture, drive keys, assert the exit code and the
file bytes.

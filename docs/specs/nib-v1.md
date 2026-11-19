# Nib v1 — a blocking markdown editor for `$EDITOR`

Status: ready-for-agent. Derived from the design interview of 2026-11-19. Language of this repo: English (personal/GitHub project type).

## Problem Statement

When I hit `ctrl+g` in pi, the agent hands my prompt to whatever `$EDITOR` is configured, as a temporary `.md` file, and waits for the process to exit. Today that means a terminal editor: no live preview, no formatting feedback, and markdown structure errors are only visible after the agent has already consumed the prompt.

I want a small desktop editor that opens exactly that file, shows me the source and the rendered result at the same time, and returns control to pi with an unambiguous "accept" or "discard" signal. I do not want a general-purpose markdown application: no new-file flow, no document management, no tabs.

The framework is Keel (Go desktop UI). Two facts from its source set the shape: its `ui/markdown` package is a finished renderer but **not** an editor core (no insert/delete/cursor API, and `SetSource` clears the selection), while `kit.CodeEditor` is a mature source editor. The rendered view is therefore a projection, never an input surface.

## Solution

A single Go binary, invoked as `nib <file.md>`. It opens one window with the markdown source on the left and the rendered document on the right, refreshable on idle, switchable to a single full-width pane. `ctrl+s` saves in place; `ctrl+enter` saves and returns success to the caller; closing the window or `ctrl+q` on a modified document asks "save / discard / go back" before anything is written.

The process is the editor session: it starts when the window opens and it exits only through one function that either writes the file and reports success, or reports failure without touching it.

## User Stories

1. As a pi user, I want `ctrl+g` to open my prompt in a windowed markdown editor, so that I can edit multi-line prompts with formatting.
2. As a pi user, I want the rendered result beside the source, so that I can see immediately whether my list, table or code fence is broken.
3. As a pi user, I want a preview that updates while I type but never steals my selection, so that I can keep selecting and replacing text.
4. As a pi user, I want `ctrl+enter` to close the window and hand the content back, so that accepting an edit is one keystroke.
5. As a pi user, I want `ctrl+s` to save without closing, so that I can save a long draft and keep editing.
6. As a pi user, I want closing a modified window to ask before anything is written, so that I cannot submit half-written text by accident.
7. As a pi user, I want discarding to leave the original prompt untouched and to signal failure, so that pi keeps the prompt it already had.
8. As a pi user, I want to know whether the document is modified, so that I do not close it thinking it is saved.
9. As a pi user, I want the file name visible in the window title, so that I can tell which prompt I am editing.
10. As a pi user, I want the whole document to render with Keel's full capability (GFM tables, code blocks, HTML as rich text, math), so that the preview matches what the agent-facing markdown really means.
11. As a pi user, I want to toggle between split and single-pane view, so that a narrow prompt does not cost me half the window.
12. As a pi user, I want markdown syntax highlighting in the source pane, so that structure is readable without mentally parsing it.
13. As a pi user, I want to search within the source, so that I can find a phrase in a long prompt.
14. As a pi user, I want the editor to keep the document's existing line endings and characters other than what I typed, so that an edit does not rewrite the whole prompt.
15. As a pi user, I want the write to be atomic, so that a crash or a full disk never leaves a truncated prompt for pi to read.
16. As a pi user, when I open a file with `nib file.md` outside pi and save it, I want the same behaviour as inside pi, so that the tool is usable on its own.
17. As a pi user, when something prevents the editor from starting (missing argument, unreadable file, unwritable directory), I want a log line explaining it and a distinct non-zero exit code, so that I can tell "I cancelled" from "it never ran".
18. As an agent developer, I want to drive the real window from automation (keys, screenshot, close) and then assert the exit code and the file bytes, so that the whole accept/discard path is testable without a human.
19. As an agent developer, I want the app's whole state machine behind one seam, so that command sequences and their exit codes are unit-testable without a window.
20. As an agent developer, I want a log file, so that a `windowsgui` build that fails before the window appears still leaves evidence.
21. As a maintainer, I want the exit path to be a single function, so that adding a feature later cannot introduce a second way to write the file.

## Implementation Decisions

**Domain shape — one seam.** The application is one domain object (`App`) built from the target path and an injected filesystem. It exposes three things only: `Handle(Command)`, `View()` returning an immutable read-only view, and `ExitCode() (code int, done bool)`. Commands are intents: `Save`, `SaveAndQuit`, `Quit`, `DiscardConfirmed`, `ReturnToEditing`, `Edit(text)`. Keel rendering, keyboard handling and window lifetime are a projection of `View()`; no UI code holds state of its own.

**Exit contract.** pi treats exit code 0 as "adopt the file" and anything else as "prompt unchanged". Nib therefore fixes three codes: `0` accepted (file written, or unmodified and nothing to write), `1` discarded or cancelled, `2` could not start (bad arguments, unreadable file, unwritable parent). The single exit function serialises the content, writes a sibling temporary file, syncs it, renames it over the target, then calls `os.Exit(code)` — `window.Quit()` is never called, because its `os.Exit(0)` is asynchronous and would race a non-zero exit.

**Window lifetime is the session.** Keel's `window.Main()` does not return; the process ends when the last window closes. Nib leans on that: closing the window is not a side effect to be undone but the session's terminal event, routed through the same state machine as `ctrl+q`. Keel has no before-close hook and its title bar closes the window directly, so the system close button cannot raise the confirmation bar: a close on a dirty document discards it (exit 1), and the event is logged so it is diagnosable. `ctrl+q` is the way to ask a dirty document whether to save.

**Modification guard.** `Ctrl+S` saves and stays. `Ctrl+Enter` saves and quits. A quit request on a dirty document raises an in-window confirmation bar (Keel exposes no message-box API): save-and-quit, discard-and-quit, or return to editing. A quit request on a clean document exits 0 without writing. `Esc` belongs to the source editor (Keel's `CodeEditor` uses it for its own interactions) and is only bound inside the confirmation bar as "return to editing"; the quit request key is `ctrl+q`, because the window close button cannot be intercepted (see window lifetime).

**Preview is a projection with an explicit refresh policy.** The preview is re-rendered from source only when the editor has been idle for a short debounce, and on save — never on each keystroke, because `SetSource` clears the selection. The policy is a named pure decision (should this refresh happen, given the edit version, the rendered version and the idle time) so it is testable without a clock. No scroll synchronisation in v1; the two panes scroll independently. Source-to-preview click mapping is deferred.

**Rendering scope is full.** Keel's default markdown capability is kept as-is, including HTML rendered as rich text and native math. Trimming was rejected: the source pane is always visible, so a strong renderer cannot mislead, and trimming would mean writing and maintaining filter logic for no gain.

**Editing surface.** `kit.CodeEditor` with language `markdown`, syntax highlighting enabled through Keel's opt-in highlight package (chroma's markdown lexer is present in it), soft wrap on, search available, line numbers on. Jumping to a line is done through the cursor API, which also scrolls.

**File handling.** The file is read once at start; BOM is dropped, line endings are preserved as found. The editor's text is what gets written — no auto-appended trailing newline, no re-wrapping, no reformatting. Writes go through a sibling temporary file plus rename so a reader never sees a partial file.

**Invocation surface.** Exactly one positional argument: the path. No new-file flow, no open dialog, no drag and drop, no tabs, no single-instance handoff (Keel has no IPC), no lock file. A second concurrent invocation is allowed to exist and may overwrite the first — accepted risk for an `$EDITOR `. More than one argument, or none, is an exit-2 error.

**Failure visibility.** The binary is built with Keel's Windows GUI flags, so there is no console and panics are silent. Nib therefore appends to a log file under the user's local application data directory, and the log path is shown in-window so it is discoverable. Preflight failures are logged before any window is created.
**Keyboard and status surface.** Status bar shows line and character counts, the last save outcome, and the shortcut hints. The header row shows the file name and a modified marker; the window title carries the file name too, but Keel sets it once at creation and offers no rename API, so the marker cannot live there.

**Dependency.** The only third-party dependency is the Keel module itself (plus its transitive chroma). No other library: no config file, no CLI framework, no logging package.

## Testing Decisions

A good test here exercises behaviour through `App` — a command sequence in, an exit code and file bytes out — and never inspects internal fields, the render tree, or the debounce timer's implementation. The ideal count of seams is one, and it is the one we test.

- **`App` state machine.** Table-driven cases over command sequences: save-then-quit, quit-clean, quit-dirty-then-discard, quit-dirty-then-return-then-save, quit-dirty-then-save-and-quit, edit-without-command. Each asserts `ExitCode()` and the resulting file bytes, with the expectations written in terms of the exported exit-code constants rather than literals.
- **Document write path.** Cases for: unmodified document written at all / not at all, content containing no trailing newline, content with CRLF preserved, missing file, unwritable directory, and the atomic-replace ordering (no partial file observable). Uses the injected filesystem against a temporary directory.
- **Preview refresh policy.** A pure function with its own table: same version, new version below the debounce, new version after the debounce, save-forced refresh.
- **End-to-end via automation.** One scripted scenario through Keel's automation server: launch a build against a fixture file, press a few keys, take a screenshot for the record, close the window, then assert the process exit code and the fixture's bytes. This is the only test that touches the real window; it exists because the exit contract is exactly what unit tests cannot prove.
- **Not tested by unit tests:** Keel rendering output, window layout, keyboard wiring, and log formatting. These are covered by the automation scenario or by eye.

There is no prior art: the repository starts empty.

## Out of Scope

- Click-a-rendered-block-to-jump-to-source (recorded as the v1.1 candidate).
- Block-level WYSIWYG: inline rich-text editing, block dragging, table cell editing. A future change here is an editor-core rewrite, not an increment.
- Preview/source scroll synchronisation.
- Single-instance reuse, tabs, sessions, recent files, file dialogs, new-file creation, drag and drop.
- Mermaid, PlantUML, Graphviz rendering; image export; print.
- User-configurable themes, fonts and keybindings.
- Log rotation and structured log levels.
- Crash reporting, telemetry, auto-update.
- macOS and Linux verification (the code avoids platform-specific assumptions, but only Windows is validated).

## Further Notes

- The `$EDITOR` command string is split on spaces by the caller, so the installed binary must live at a path containing no spaces.
- pi strips one trailing newline from the file it reads back, and drops a BOM; writing exactly what the user wrote is therefore safe, and appending a newline "for politeness" is not needed.
- Keel's automation element tree does not include `kit.CodeEditor`, so automation cannot read or click the editor body; keyboard input reaches it through focus, and the file bytes after exit are the authoritative check.
- The interview record: full-renderer scope, split-pane preview, explicit save/quit bindings, single-exit write path, and no locking were all chosen deliberately — see the ADRs in `docs/adr/`.

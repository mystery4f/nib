# Nib

A blocking markdown editor used as `$EDITOR`: it opens one file handed to it by a calling process, and that process judges the edit by Nib's exit code.

## Language

**Session**:
One run of the binary, from the accepted argument to the process exit. The window's lifetime is the session; there is never more than one document per session.
_Avoid_: instance, document window, tab

**Accept**:
The outcome where the file on disk now holds the content the session produced, reported as exit code 0. Writing and accepting are not the same event: an unmodified session accepts without writing.
_Avoid_: save (that is the user's keystroke), commit, submit

**Discard**:
The outcome where the file is deliberately left byte-identical to its state at session start, reported as a non-zero exit code. Also the state a quit request waits in until resolved.
_Avoid_: cancel, abort, rollback

**Could not start**:
A preflight failure — wrong arguments, unreadable file, unwritable directory — that ends the session before any window exists, reported distinctly from Discard.

**Dirty**:
The document's text differs from what is on disk. It is a fact about the document, not about whether the user pressed save.
_Avoid_: modified, unsaved, changed

**Command**:
A user or window event expressed as an intent the domain object handles ("save", "save and quit", "quit"). Handlers name a command; they never mutate state directly.
_Avoid_: action, event, message

**Preview**:
The rendered markdown, always a projection of the source and never an input surface. It may lag the source by the refresh policy; it never becomes the source of truth.
_Avoid_: rich text view, WYSIWYG pane, output

**Refresh policy**:
The rule that decides whether a pending edit is due to be rendered: idle past the debounce, or forced by save. Deliberately separate from rendering itself.
_Avoid_: debounce, throttle, autosave

**Exit path**:
The one function through which a session ends: it either writes and accepts, or leaves the file alone and reports a code. No other code calls the process exit.
_Avoid_: shutdown, teardown, quit handler

**Preflight**:
The checks made before the window opens — argument count, file readability, directory writability — whose failures are logged rather than shown.
_Avoid_: validation, startup checks

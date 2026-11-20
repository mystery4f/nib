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
The outcome where the file is deliberately left byte-identical to its state at session start, reported as exit code 1. Also the state a quit request waits in until resolved.
_Avoid_: cancel, abort, rollback

**Could not start**:
A preflight failure — wrong arguments or an unreadable file — that ends the session before any window exists, reported as exit code 2, distinctly from Discard.
_Avoid_: validation error, startup failure

**Dirty**:
The document's text differs from what was last written. It is a fact about the document, not about whether the user pressed save, and it is the flag that decides whether a close has to ask and whether a save touches the disk at all.
_Avoid_: modified, unsaved, changed

**Shell**:
The Rust half: it owns argv, the file, the dirty flag, the atomic write and the exit code. It renders nothing.
_Avoid_: backend, host, core

**Page**:
The React half: it owns the editor and the key bindings, and asks the shell to save or to exit. It never touches the file.
_Avoid_: frontend, UI layer, renderer

**Command**:
One of the six operations the page may invoke on the shell — read the document, write it, report dirtiness, save and exit, discard and exit, accept and exit. Each ends a session or leaves the file untouched; none returns a document to the page mid-session.
_Avoid_: call, request, RPC

**Confirmation strip**:
The bar the page shows when a quit request arrives on a dirty document, offering save-and-close, discard, or keep editing. It is shown by the page, not by the window manager, because the shell only knows that a close was requested.
_Avoid_: dialog, modal, prompt

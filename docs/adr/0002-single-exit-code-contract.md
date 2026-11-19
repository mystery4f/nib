# One exit function, `os.Exit(code)`, and never `window.Quit()`

The caller (pi) adopts the file only on exit code 0 and treats everything else as "prompt unchanged", so the exit code is the entire user-visible contract. Keel offers no way to choose that code: `window.Main()` never returns, and `Quit()` performs cleanup in a goroutine and then calls `os.Exit(0)` asynchronously — a race if a non-zero exit is issued nearby. The session therefore ends in exactly one place: serialise, write a sibling temporary file, sync, rename over the target, then `os.Exit(code)` with `0` accepted, `1` discarded, `2` could not start. `window.Quit()` is not called anywhere in the codebase.

## Consequences

`OnQuit` clean-up is skipped by design; nothing in the app owns a resource that needs it. Because the process may die inside a signal-free exit, the write must be complete and synced *before* the call — the ordering inside that single function is load-bearing, and tests assert the file bytes rather than trusting the code path. A future graceful-shutdown feature cannot be added by switching to `Quit()` without re-deriving the exit code contract, so this ADR exists to make that cost visible.

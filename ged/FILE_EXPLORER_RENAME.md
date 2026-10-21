# Explorer rename transaction SDK

`FileExplorer.RenamePath` validates/reserves on the UI thread, dispatches one
filesystem worker, and delivers exactly one completion back to the UI. Its
`RenameBusy()` reservation survives root/view retirement until completion.
Filesystem mutation is no-replace; existing targets must not be overwritten.

`SetRenameCoordinator` is the legacy compatible UI prepare/complete hook. Use
`SetRenamePreparation` when background readers/processes must release the source
first. Its result is:

```go
type RenamePreparation struct {
    Wait     func(context.Context) error // background worker; may be nil
    Complete func(error)                // UI thread; nil means committed
}
```

The prepare callback must not block. Capture immutable readiness handles there;
do not read widgets/maps from `Wait`. The explorer supplies a five-second
cooperative deadline. Wait errors (including a deadline expired just as Wait
returns nil) become `SILK-FS009` preserving the source path and original cause.
**No filesystem mutation occurs on wait failure.** Native rename errors retain
existing file-operation codes. Complete must release every reservation and apply
success/rollback to the latest editor buffer state, never an old text snapshot.
The public done callback follows host completion after RenameBusy is cleared.

SilkIDE's actual implementation is `cmd/silkide/file_rename.go`: it reserves both
path identities against save/open/close, suspends SDK editor Git for the source
subtree, retires LSP callbacks and awaits actual Git child reaping on the worker.
A closed tab's still-running Git process is included. Refresh requests issued
while waiting remain bounded/held. Success rebinds dirty editors, undo histories,
metadata and providers; failure restores old-path providers without saving or
discarding edits. The Git gate releases only after UI reconciliation.

A blocking native filesystem call cannot be forcibly canceled. The event loop
must stay alive until completion. Designer/autosave rename migration is still
explicitly rejected rather than partially applied. Aliases, running build/debug
jobs, other Git subsystems and unrelated process locks are separate concerns.

Regression tests cover a held worker, nested/retired Git jobs, unrelated work,
10,000 coalesced refreshes, timeout without mutation, root retirement, exact-once
completion and dirty-buffer/undo behavior. Native Win10 still requires a fresh
manifest/hash, actual Git on PATH and UI save/readback/exit checks.

# Editor Git markers

`CodeEditor.SetFilePath` and `RefreshGitStatus` now schedule work; they never run
Git or wait on a worker on the UI thread. Markers describe the **saved working
file versus the index**. They are not unsaved-buffer diffs or staged-vs-HEAD diffs.

## SDK lifecycle

```go
editor.SigGitStatusChanged(func() {
    if err := editor.GitStatusError(); err != nil {
        // Log diagnostic.CodeOf(err), cause and recovery hint.
    }
    markers := editor.GitLineStatuses() // owned copy, 1-based line numbers
    _ = markers
})
editor.SetFilePath(path)
// Read GitStatusLoading() on the UI thread; completion arrives through gui.Post.
// After a save or external file change:
editor.RefreshGitStatus()
// Before permanently removing the editor or stopping its event loop:
editor.CancelGitStatus()
// After the GUI loop has returned, on final process shutdown (not in a UI handler):
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()
if err := gui.ShutdownEditorGit(ctx); err != nil {
    // Record cleanup timeout; cancellation alone does not certify child exit.
}
```

All editor APIs/callbacks remain UI-thread-only. Cancellation does not wait for
I/O or emit a failure; a later refresh creates a fresh subscription. Changing
identity immediately clears old markers and errors. Successful empty results
clear old markers too. Failures clear markers and retain `GitStatusError`; a
copied structured diagnostic prevents callers from modifying editor-owned state.
`SigGitStatusChanged` signals only accepted final results, not every queued edit.

One process-wide editor-gutter scheduler permits at most **two active Git jobs**, one latest
pending request per editor, and one queued GUI delivery for all editors. Closing
an editor removes queued work/results; a canceled running operation occupies its
worker until it really exits. Repeated refresh/reopen cannot create unlimited
subprocesses or workers. Idle workers exit; there is no timer or polling loop.
The queue scales with live editors, not refresh count.

`ShutdownEditorGit` permanently stops the process-wide service, rejects new work,
cancels queued/running jobs, suppresses queued deliveries and joins the actual
workers. It is idempotent; a deadline error means cleanup is not yet certified.
Use `CancelGitStatus` for an individual tab, not this terminal application API.
The IDE waits up to three seconds after its event loop returns.

## Synchronous API

`GitDiffContext(ctx, path)` is a synchronous **off-UI** SDK primitive returning
markers plus an error. The existing `GitDiff(path)` remains a synchronous wrapper
that suppresses errors for compatibility; do not call it from UI handlers.

- Resolve the file's directory and run Git there, not in the IDE process cwd.
- Literal pathspec and `--` protect filenames with glob/option syntax.
- Ignore inherited repository-selection variables such as `GIT_DIR`; fix the
  locale for identifying the ordinary non-repository result.
- Disable pager/external diff/textconv and optional index locks.
- Two seconds for the complete request, 2 MiB stdout, 64 KiB stderr, and at most
  100,000 hunk-derived markers; malformed/overflowing ranges fail closed.
- `exec.Cmd.Run` joins pipe readers and reaps the direct child. Cancellation and
  capture overflow terminate that child; `WaitDelay` bounds inherited pipe waits.
  Windows background commands use `CREATE_NO_WINDOW` without changing GUI apps.
- Ordinary non-repository/untracked/unchanged files have an empty successful
  result. Missing Git (`SILK-GIT001`), execution/timeout/malformed output
  (`SILK-GIT002`) and output/marker limits (`SILK-GIT003`) preserve cause and hint.
  Limits never return partial markers as though the diff were complete.

The limit bounds the direct child and captured output, not arbitrary OS stalls
inside process startup or the lifetime of custom Git/filter grandchildren.
No arbitrary process-tree enumeration or unrelated process termination is used.

## IDE integration and remaining scope

SilkIDE schedules refresh after successful save and retires work on tab close and
application shutdown. SDK `ged.EditorTabs` also retires work when closing a tab.
An automatic error is logged once per distinct failure per editor (no toast storm).
The command palette **Refresh Editor Git Markers** explicitly reports success or
an error banner. Stale manual requests after rename do not claim completion for
the former path.

Other Git features (blame, Changes panel, repository-wide operations), preferences
flushes, live unsaved-buffer diffing and filesystem watchers have separate
lifecycles. This change is not a claim that all IDE I/O or all background work is
asynchronous or that Windows/macOS desktop acceptance has already passed.

## Filesystem transactions on Windows

Canceling a query is not enough before renaming a directory: a live Git process
can keep that directory open as its CWD. `SuspendEditorGitPath(path)` reserves a
**lexical path subtree**, cancels matching scans and holds newer matching requests.
It includes jobs still reaping after their tab was closed or rebound; suspending
only the currently visible editor would miss those jobs. Unrelated paths retain
normal scheduling. Each live editor still has only one latest pending request;
held requests occupy no worker and there is no polling goroutine.

Acquire the reservation on the UI thread, call its `Wait(ctx)` on a background
worker before the filesystem mutation, then migrate editor identities on the UI
thread and `Release()` on either success or failure. Wait joins actual scans/direct
children, not cancellation notifications. A wait timeout must abort mutation, not
be treated as successful cancellation. `Release` is idempotent and nested path
reservations remain effective until their last release. Shutdown also retires held
requests. Already-queued GUI results honor reservations even when an earlier
callback reserves a path during delivery of the same batch.

SilkIDE uses `ged.RenamePreparation` for this sequence. The explorer gives its
cooperative wait a five-second deadline and reports `SILK-FS009` with source path,
original cause and recovery hint if readiness fails; it does not rename or save
unsaved buffers on that failure. Existing `SetRenameCoordinator` callbacks remain
supported without a wait phase. See `ged/FILE_EXPLORER_RENAME.md`.

This is not a lock against unrelated processes, filesystem aliases/symlinks,
custom Git grandchildren, build/debug processes or OS-level I/O hangs. Those still
need their own lifecycle coordination and native tests; no sleep/retry loop hides
a sharing violation.

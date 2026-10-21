# Settings persistence and UI scheduling

`Sync()` remains synchronous and returns only after the actual snapshot write,
file sync, close, and replacement have finished. Disk I/O and encoding no longer
hold the settings mutex. Reads and mutations can continue while an already-loaded
store is saving; an older snapshot never clears newer dirty edits.

For UI callbacks, first load the store during initialization, then use:

```go
if err := prefs.SetString("editor/theme", "dark"); err != nil {
    // report error
}
if err := prefs.RequestSync(); err != nil {
    // report load-protection error; background failures appear in Status()
}
// At shutdown, after stopping all producers, outside the UI dispatch loop:
if err := prefs.Flush(context.Background()); err != nil {
    // report diagnostic code, path, cause and hint; this was NOT a successful save
}
```

- At most one on-demand writer per `Settings`. Repeated requests during a save
  coalesce into the latest snapshot. No polling, permanent worker, or goroutine
  per refresh. A completed/failed flight exits.
- `Flush(ctx)` joins that flight and coalesced requests. A timeout only ends the
  caller's wait; it does **not** abort filesystem I/O. Do not exit/declare success
  on timeout. After stopping producers, use an unbounded final Flush when true
  completion is mandatory. Continually issuing new requests can prolong a flush.
- SetValue/Remove/Clear without a save request can leave newer changes dirty even
  after an older save succeeds. `Status()` reports the last actual save error;
  a timed-out wait is returned to that waiter without falsifying writer status.
- First Value/SetValue/Sync still performs lazy synchronous loading. RequestSync
  does not load an untouched store. This change does not eliminate startup I/O,
  snapshot-clone cost, synchronous Sync call-site latency, or external disk stalls.
- Group stacks are shared cursors: individual methods are synchronized, but a
  BeginGroup/SetValue/EndGroup sequence is not a multi-call transaction.

## File safety

A same-directory temporary file is encoded, synced and closed before replacement.
Encoding/flush/close/rename failure retains dirty state and never truncates the
previous file. POSIX uses rename. Windows uses FileRenameInfoEx with replace-existing and POSIX
semantics; SDK settings/TDoc file readers use shared-delete handles. A native
probe proved that MoveFileEx fails with an open target even when its reader
shares deletion. Unsupported extended-rename kernels/filesystems fall back to
legacy FileRenameInfo, with weaker open-reader semantics. Neither path deletes
first, ignores readonly flags, or retries sharing/permission failures blindly. Local filesystem rename
semantics apply; this is not a universal network-filesystem or power-loss guarantee.
Existing permission bits are retained; new files use 0600 and directories 0700.
Windows ACL inheritance follows the temporary file's directory; custom per-file
ACLs and multiple writers/instances/processes are not reconciled.

Special paths and symlinks are rejected. Detected load failures are sticky: even
Clear does not authorize overwriting an unreadable/corrupt original. Back it up,
repair it, and create a fresh Settings instance. The complete gzip stream is
validated before parsing, including its trailer/checksum. Plain TDoc grammar
remains tolerant; this is not strict schema validation. Filesystem races with
external path replacement/other processes are not a transactional CAS protocol.

## Diagnostics

- `SILK-SET001`: read/decode failure; original file protected.
- `SILK-SET002`: write/sync/close/replacement failure; dirty settings retained.
- `SILK-SET003`: flush wait timed out/canceled; real writer may still be active.

All include the path, original cause (`errors.Is`/`errors.As`) and recovery hint.
`core.TDoc.Save/Save1` now return buffered flush, gzip finalization and short-write
errors; SaveFile also returns close errors. Standalone TDoc.SaveFile still writes
in place and is not an atomic settings-style replacement API.
Generic TDoc readers now propagate transport and gzip errors, decode the writer's
space/tab key escapes, and report `SILK-DOC001/002/003`; see
[`core/TDOC_IO.md`](../core/TDOC_IO.md). Settings-specific load protection still
wraps failures with `SILK-SET001` and retains the underlying cause.

## Integration boundary

SilkIDE's preference setters now enqueue a coalescing host save worker instead
of blocking interactive callbacks in Sync. That worker joins the SDK writer;
at most one of each is active, with no idle polling. One queued UI notification
reads current status, de-duplicates errors, and includes code/path/cause/hint.
After final session capture and core.EventLoop return, finishPreferenceSaves
stops requests, retires callbacks and joins the actual final save. Shutdown wait
cancellation is not I/O cancellation. The production exit join has no timeout;
a permanently stalled filesystem can delay process exit after the window closes.
Exit failures are logged with diagnostic details, not reported as saved.

Appearance changes are applied immediately and say saving in background, not
saved before completion. Initial lazy loading, recent-file stat calls and other
IDE persistence paths are still separate latency concerns. Package tests do not
prove whole-IDE frame-rate targets or every native UI operation.

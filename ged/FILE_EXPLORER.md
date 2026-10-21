# FileExplorer SDK

All widget APIs and callbacks run on the UI thread. `SetRootDir` and `Refresh`
now **schedule** scans instead of synchronously walking a project. Existing
rows remain usable during refresh; a new root clears the old rows immediately.

```go
files := ged.NewFileExplorer()
files.SigFileOpen(func(path string) { /* open the requested absolute path */ })
files.SigScanError(func(err error) {
    log.Printf("scan %s: %v", diagnostic.CodeOf(err), err)
})
files.SetRootDir(projectDir)
// files.Loading() is true until a matching-generation result reaches the UI.
// Filesystem errors are retained by LastError() and the header warning button.
```

- `SetFilter(query)` matches a case-insensitive relative-path substring;
  `SetFilter("")` clears it. Filtering traverses lazily hidden subdirectories
  subject to the scan budget, displays matching rows and their ancestors, and
  does not search file contents. The filter input is capped at 256 runes.
- `RevealPath(path)` requests ancestor expansion and selection of an in-root
  path. Its bool reports scope acceptance, **not** existence or scan completion.
  Hidden/excluded paths remain subject to visibility policy; call
  `SetShowHidden(true)` or adjust exclusions first when appropriate.
- `SelectedPath`, `RootDir`, `Filter`, `ShowHidden`, `Loading`, `LastError` are
  UI-thread queries, not goroutine-safe synchronization APIs.
- `SetExcludedDirs(names)` replaces the default exclusions. Despite the legacy
  name it excludes matching entry basenames (files too). `docs` and `icon` are
  visible by default; `.git`, `node_modules`, `vendor` and other generated/tool
  directories are excluded. Dot entries require `SetShowHidden(true)`.
- `SetEntryLimit(n)` bounds inspected entries (default 50,000, maximum 1,000,000).
  Depth is capped at 64. Reaching a cap produces a partial list plus a typed
  `SILK-FS005` error, not a claim that all matches were found.
- Call `CancelScan()` when permanently removing the widget. It retires pending
  deliveries; a subsequent `SetRootDir` can reuse the widget. Idle workers exit.

## Input and rendering

Arrow keys navigate/collapse/expand; Home/End/PageUp/PageDown select visible
rows; Enter opens a file; F5 refreshes; F2 requests rename; Escape clears the
filter. Cmd/Ctrl+F or the header search icon opens the filter dialog. Explorer
F2/F5 override host shortcuts only while this widget is focused. Save, build,
modified F-keys and other global shortcuts keep their normal host ownership.
SilkIDE also exposes Show Files, Filter Project Files, Refresh Project Files,
Toggle Hidden Files and Reveal Active File in Explorer in the command palette.

Painting only visits viewport rows, respects theme/font changes, clips the body
below a fixed header and elides measured text before the reserved Git badge.
Git status is optional, NUL-delimited, timeout/output bounded, and mapped back
to the caller's root path (including project-root aliases).

## Filesystem safety and remaining limits

Scans use `os.Root`, never intentionally descend into child symlinks, and cancel
between filesystem calls/entries. A kernel filesystem call already blocked on
an unavailable volume cannot be preempted; it stays off the UI thread and there
is at most one scan worker and one pending request per explorer. Completed UI
snapshots are also coalesced, so a busy UI cannot accumulate one callback per
refresh.

Create uses exclusive creation. Rename uses atomic no-replace primitives on
Windows, macOS and Linux; unsupported filesystems/platforms fail instead of
falling back to a potentially overwriting rename. Names are normalized by
trimming outer whitespace and checked for cross-platform reserved characters,
control characters, trailing dots and Windows device names. Errors preserve
causes and include `SILK-FS002/003/004/005` codes.

## Async rename and host document coordination

`RenamePath(path, newName, done)` is UI-thread-only, validates a portable name
and an in-root non-root source, then runs the atomic no-replace rename on one
worker. F2 and the context menu use this same path. Immediate errors reject the
operation; accepted operations call `done(error)` once on the UI thread.
`RenameBusy()` stays true through filesystem completion and host reconciliation;
a second rename (or UI create/delete) fails with `SILK-FS008` while it is busy.
The default explorer also migrates selected/expanded paths for directory moves.

Hosts may install `SetRenameCoordinator(prepare)`. `prepare(oldPath, newPath)`
reserves affected documents before disk mutation and returns `(complete, error)`.
An error vetoes the rename without touching disk. `complete(error)` reconciles
host state on the UI thread, on BOTH success and failure. It is delivered even
if the explorer changed root or `CancelScan()` retired its scans: a real disk
mutation cannot be silently abandoned. Hosts must keep their UI event loop
alive until completion, release reservations on failure, and retain dirty
buffers. Do not run filesystem work in a coordinator. The underlying OS rename
cannot be interrupted; a timeout must not pretend the operation was canceled.

SilkIDE now reserves the source/destination subtree, keeps typing and undo
available, protects save/open/close/quit during the operation, and rebinds all
open code editors after a successful move. Editor callbacks resolve current
identity instead of capturing the original filename. Old LSP/macro providers
retire before dispatch; new Go URIs wait for old `didClose` and reopen with the
latest unsaved text. Tabs, macro analysis, references, problem paths, bookmarks,
MRU and session paths migrate; affected old coverage entries are invalidated.

**Still under development:** create/delete remain synchronous and lack editor
coordination. Active designer documents (including their parent directories)
and active debug/macro-expansion sessions currently veto renames rather than
migrating autosave/reloader/debugger state. Preference persistence in the IDE
commit callback still uses the existing synchronous settings flush. Build/test
result invalidation and symlink aliases need further coordination. Case-only
rename on case-insensitive filesystems may report a destination conflict; no
unsafe overwriting fallback is used. Native desktop verification must match
the exact source snapshot, not a preceding iteration's green tests.

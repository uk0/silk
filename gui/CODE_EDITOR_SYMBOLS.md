# Editor symbol snapshots

`CodeEditor` shares its tolerant Go/SilkGo declaration parser between navigation,
breadcrumbs, test gutter markers and the outline. Plain-text files only expose
explicitly supplied provider symbols. All editor APIs below are UI-thread-only.

- `ParseSymbols() []CodeSymbol`: synchronous, exact current-revision snapshot.
  Intended for explicit commands, not a paint/poll loop. The returned slice is owned
  by the caller. Existing SDK callers retain their synchronous semantics.
- `SymbolsSnapshot() ([]CodeSymbol, bool)`: nonblocking UI snapshot for large files.
  `ready == false` means indexing is pending; it does **not** mean no declarations.
  Do not retain or execute stale line-numbered actions while waiting.
- `SymbolsRevision() uint64`: changes on text/provider updates and acceptance of a
  background parse, including a synchronous command completing a pending request.
  Cache UI projections by this revision rather than repeatedly copying snapshots.
- `SubscribeSymbolsChanged(func()) func()`: subscribe on the UI thread and keep
  the returned unsubscribe. Invalidate the consumer's own paint region in the
  callback; a revision change or editor repaint alone does not redraw an
  independently cached sibling panel. Events cover text/provider/dialect changes
  and accepted background results (or synchronous takeover). Multiple observers
  are supported; detach on editor switch/disposal. Callbacks must be brief and
  must not synchronously edit the source. `CodeOutlinePanel.SetEditor` manages
  this subscription automatically; call `SetEditor(nil)` when disposing it.
- `CancelSymbolIndex()`: call before permanently removing an editor or stopping its
  event loop, alongside `CancelCompletion()` and `CancelGitStatus()`. It releases
  pending worker snapshots/delivery references and suppresses queued results. It
  does not unsubscribe symbol observers; detach those separately as above. It
  does not wait on the UI thread; a later query can start fresh.

Files below 32 KiB keep the synchronous fast path. Larger files use a 90 ms
cancelable debounce, one worker and one latest request per editor, at most two
concurrent parsers process-wide, and a coalesced UI delivery slot. Idle workers
exit; no repaint timer polls for completion. Background work receives immutable
text/provider snapshots, never an editor or its mutable line array. Results are
accepted only for the matching worker, text revision, dialect and provider
generation. Old results cannot restore a removed test button or overwrite new
provider symbols after an edit, undo, language switch or tab close.

The standard Go parser cannot be interrupted mid-parse. An obsolete running parse
keeps its concurrency permit until it returns, then discards the result. This is
not a general hard CPU/memory limit or an asynchronous replacement for explicit
navigation. Large-file text rebuilding, formatting/saving and host LSP work have
separate costs. See `EDITOR_INPUT_QA_20261024.md` for measured scope and native QA.

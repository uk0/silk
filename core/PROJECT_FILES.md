# Bounded project file enumeration

`ScanProjectFiles(ctx, root, ProjectScanOptions)` is a synchronous SDK primitive
for background workers. Results include absolute `Path`, host-relative `Relative`,
`Root`, visited-entry count and `Incomplete`. Hidden names are omitted by default;
callers can supply excluded directory basenames and case-insensitive extensions.
The IDE excludes generated/vendor/binary trees (including `local`) and binary
assets, while retaining `.silkgo` and `.silkui`.

Defaults are 50,000 visited entries and 64 directory levels; explicit positive
limits override them. Enumeration uses `os.Root` and 256-entry batches, refuses
child symlink traversal and omits special files. The chosen root may itself be a
symlink. The returned paths are names, not immutable handles: later editor open
must still deal with files that are removed/replaced after enumeration.

Errors retain partial results and set `Incomplete`: `SILK-FS005` carries access/
read causes; `SILK-FS007` indicates a count/depth limit, with a reminder that query
refinement cannot find files outside the collected index. Cancellation preserves
`errors.Is(err, context.Canceled/DeadlineExceeded)`. Filesystem syscalls cannot be
forcibly interrupted, so **do not run this API on a UI thread**.

## Quick Open scheduling

SilkIDE opens the dialog immediately with an indexing state. One application-wide
worker scans once per dialog and ranks off-thread. Typing replaces the pending
query (75 ms debounce) without restarting the scan. Close/root replacement
cancels the owner, drops callbacks and keeps at most one stalled scan plus one
latest session; repeated open/close cannot multiply blocked filesystem workers.
Reopening scans again rather than silently relying on a stale cross-dialog cache.

At most one GUI delivery is queued. Results are generation checked; changing the
query immediately invalidates old actionable rows. The list shows at most 500
matches and discloses the count. Scan errors/limits remain visible even when a
query has no matches, with code/cause/recovery hint in the status tooltip and Log.
Enter in the query field accepts the current selected row, not always row zero.
A slash-separated query also works on Windows without changing the opened path.

This is not a filesystem watcher, asynchronous rename/delete coordinator or a
million-file index. Explorer mutation/editor identity coordination remains open.

# Cancelable Git read APIs

The synchronous compatibility APIs `GitStatusPorcelain` and `GitShortLog` now
use the bounded read implementations. UI hosts should call these context-aware
forms from a worker, never from input/paint callbacks:

- `GitRepositoryRootContext(ctx, dir)` resolves the actual working-tree root.
- `GitStatusPorcelainContext(ctx, dir)` returns lossless root-relative paths.
- `GitShortLogContext(ctx, dir, count)` returns up to 1–10,000 recent commits.
  A valid unborn branch returns an empty list, not a tool-failure result.

Status uses porcelain v1 `-z`, preserving Unicode, spaces, quotes, tabs, literal
`->`, and newline-containing names on filesystems that allow them. Rename records
carry destination followed by source; they are not split on textual arrows.
See [Git's porcelain format](https://git-scm.com/docs/git-status#_porcelain_format_version_1).
Join entries with the resolved repository root, **not** an arbitrary workspace
subdirectory. Status/path decoding does not promise every mutation/pathspec API
has been audited.

Each read inherits cancellation and has a 10-second command timeout. Captured
stdout is limited to 8 MiB, stderr to 64 KiB, and status to 100,000 entries.
Cancellation/overflow returns an error rather than a partial snapshot that could
look clean. `exec.Cmd.WaitDelay` bounds inherited pipe handling after exit; the
direct subprocess is waited/reaped. This is not a general process-tree kill API.

Use `errors.Is` with `ErrGitUnavailable`, `ErrGitNotRepository`, `ErrGitReadLimit`,
`ErrGitStatusMalformed`, and the context errors. Invalid history limits are
errors. Read commands disable optional index locks and terminal prompting, use a
stable error locale, and remove inherited repository-selection environment
variables. No machine PATH changes, repository initialization, downloads,
global configuration changes, or write commands are performed by these APIs.

## Panel feedback and host scheduling

`ged.GitChangesPanel` and `ged.GitLogPanel` expose `SetLoadState`, `LoadState`, and
`SigRefreshRequested`. `PanelLoadState` distinguishes ready/loading/failed/empty,
with bounded title/detail/action text. SetEntries/SetFiles/SetCommits/Clear retire
feedback. All calls and callbacks belong on the UI thread. Feedback uses normal
child widgets, not independent popup windows or polling/spinner timers. A blocked
view does not render stale rows or accept row/commit actions; hosts still own
error data clearing, cancellation and repository identity checks.

SilkIDE coalesces automatic requests into one running query plus one latest
pending query per pane, and waits for the old query to finish before starting a
replacement. Missing Git/non-repository state suppresses automatic save retries;
Retry, palette refresh, or a new workspace can recover it. A manual retry queued
during an existing failure is retained. Results are fenced by workspace,
request sequence and panel identity; closing the IDE cancels and joins readers.
Git remains an optional external tool, separate from the bundled gopls server.

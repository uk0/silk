# Process Runtime source navigation verification

Baseline: `98045d9`. This connects the existing embedded runtime to the
authored source; it does not add runtime edits, automatic acquisition, or a
schema change.

- The bounded decode worker indexes point declarations, devices, and pipes in
  the exact snapshot. Point names live in a separate namespace from object IDs.
  A binding or pipe endpoint reference is not a declaration.
- **Go to source** uses the selection in the visible Points/Diagram view and
  records normal Back/Forward history. Without a selection it focuses the
  source file without moving the caret. Obsolete buffers, replaced tabs,
  competing intents, and keys without an exact token never receive coordinates.
- Diagram inspection is opt-in and read-only. It follows the rendered zoom,
  pan, visibility, rotation, and pipe path. Its outline does not replace the
  pipe's quality/diagnostic color or start edits/drags.

## Frozen regression results

Source archive: 2,893 paths, SHA-256
`956fb87a2febcdc6c8fbfa6a798f1b885020b83bfc6ccb93280cbe2bdc102a1f`.

| Platform | Full / race (each) | Strict bundled gopls |
|---|---:|---:|
| macOS arm64 | 9,710 named passes, 29 skips, 0 failures | 4 passes, no skips |
| Linux arm64 / Xvfb | 9,649 named passes, 37 skips, 0 failures | 4 passes, no skips |
| Windows 10 amd64 | Full passed with the test-only deadline fix; focused race passed on four changed packages; full race pending | Pending |

The first Windows full run stopped on `filesearch.TestSearchContextErrorsCancellationAndSymlink`
with a successful search from a still-live one-nanosecond timeout context
(`filesearch/context_test.go:141`). That test predates runtime navigation and
the frozen production code is unmodified. Its fixture now uses a deadline
already in the past. The clean Windows replay applies only that test-file
overlay, with an independent manifest and isolated results. The original
targeted test passed 40 isolated repetitions (the original suite failure
was intermittent); the corrected version passed 100 repetitions before the
fresh full matrix began. The corrected full run exited zero; the race and
strict gopls portions are separate gates.

Focused coverage: Unicode/CRLF and inherited key positions, namespace
separation, read-only rotated/hidden/cached geometry, the actual runtime
button against a SQLite-backed session, reentrant tab choice, stale revisions,
cross-tab history, unchanged dirty buffer, and independent disk readback.

## Native macOS

Used the exact built SilkIDE binary in an isolated profile. Opened the
Unicode/CRLF fixture at line 10, column 14; F5 started the runtime.
The live point `BI11` (73, Good) jumped to line 9, column 16; clicking the
tank and pressing Right left its geometry unchanged and jumped to `tankB`
(line 30, column 14). Selecting the pipe jumped to `flowRoute` (line 42,
column 14); the live green stripe remained visible inside the selection.
Back and Forward restored the device and pipe coordinates respectively.
Stop displayed **Stopped; no acquisition**; Cmd+Q closed the process
(exit status was not captured).

The saved session independently contained zero-based `line=41,column=13`.
The source SHA-256 was unchanged. SQLite's read-only integrity check passed,
both typed definitions existed, and the latest/history tables had zero
records: startup samples are in-memory and the navigation did not write them.

## Native Windows 10

The real amd64 IDE in interactive Session 1 used the same frozen source and
fixture. F5 started the runtime, and selecting the second point (`DI11`, true,
Good) jumped to line 14, column 16. Diagram selection jumped to `tankB`
(line 30, column 14) and then `flowRoute` (line 42, column 14). Back/Forward,
Stop and Alt+F4 were exercised on the same window. The held process handle
reported exit 0, no owned gopls process remained, the persisted caret was
41:13 (zero-based), and source bytes matched. The stopped SQLite store
passed integrity readback and held the two definitions but no sample writes.

[Original Windows capture: selected live pipe and its declaration](docs/images/hmi-runtime-source-windows.png).

Raw evidence and exact source manifest: `bin/qa-artifacts/process-runtime-nav.YnaSmw/`.
Alarm lifecycle, hot reload and broader IDE parity remain separate.

# SilkIDE alarm dock verification

The **Live alarms** view is available when the running snapshot defines
rules. It shares the Points and Diagram acquisition session. Alarms and
history retain typed rule IDs; **Go to source** resolves an ID only in the
matching editor revision. History rows are read-only. Stop detaches the
session, and changes to the source invalidate the old rule targets.

## Native macOS arm64

Built SilkIDE from the frozen candidate, launched a clean profile and opened
`examples/process2d/alarms.silkhmi` from a temporary project. Pressed F5,
selected **Live alarms**, and jumped from the active `valve-trip` row to its
JSON declaration at line 57, column 14. After acknowledgment, switched to
Points and back to Live alarms; history still contained exactly the raised
and acknowledged events. Used Back, selected the historical acknowledgment,
and jumped to the same source position. Acknowledge was disabled in History.

Stop showed **Stopped; no acquisition**, cleared the rows and disabled
acknowledgment and history. Quit closed the isolated process. The source
SHA-256 remained unchanged; session readback found the persisted caret at
zero-based 56:13. The read-only SQLite integrity check passed; BI11 was an
integer, DI11 was a boolean, and the latest/history tables had zero rows.

## Regression evidence

Candidate: 2,904 paths, archive SHA-256
`034caf45ae79c3e3343d0c987f9c7f57c97df9eeb6fb1a00b8b41182e55c6786`.
The macOS full and race suites each passed 9,733 named tests (29 skips,
zero failures), with all archive members verified before and afterward.
The focused race suite passed for `hmi`, `hmiapp`, `gui`, `cmd/silkide`
and `filesearch`. Four strict bundled-gopls checks and the CGO-disabled
alarm/source-index tests passed. All staged Go and demo fixture bytes
match the tested candidate. Changes after the freeze are documentation
only, aside from an unchanged legacy text file that differs between
the working tree and index only in its line endings.

The committed baseline `d78deab` also passed the Windows 10 amd64 full and
race suites (9,530 named passes and 43 skips each) and all four strict bundled
gopls checks. The original Windows runner had zero source-hash mismatches
and no extra Go files. The Windows capture in the README is from the separate
[runtime navigation candidate](HMI_RUNTIME_NAV_QA_20261117.md), not the alarm
dock.

Manifests, test logs and native readback:
`bin/qa-artifacts/process-alarm-dock.igzTRy/`.

## Follow-up: history selection during acquisition

The follow-up preserves selection by event sequence, not by rule ID or row
index. Events with the same rule ID cannot replace the selection. Eviction or
switching modes clears it. The regression covers newest-first and sorted
history, a reused rule ID, eviction, and the disabled acknowledgment action.

The macOS native IDE was built from the follow-up archive. It polled a
loopback-only HTTP fixture. After selecting the raised `valve-trip` row,
changing the fixture BI11 value from 9007199254740992 to 9007199254740993
inserted `pressure-hi` above it. The highlight followed `valve-trip` without
another click. **Go to source** still jumped to its rule at 57:14. Returning
to **Live rules** left both active rules unselected and acknowledgment
disabled. Stop and Quit ended polling. SQLite readback retained both exact
integer samples and the persisted editor caret was 56:13 (zero-based). The
source bytes were unchanged.

The follow-up macOS arm64 full/race suites (9,734 named passes and 29 skips
each) and strict bundled-gopls checks passed. Linux arm64/Xvfb full/race
also passed (9,676 named passes and 34 skips each), together with the four
bundled-gopls checks. Both platforms verified all 2,905 source hashes after
the runs. Windows also uses this source archive, SHA-256
`42e428d4b8098817bda978c52b83482346a15f949dd229d56fa81ec422a16d5d`.
On Windows 10 amd64, all five focused alarm and UI-queue race tests passed
25 repetitions each. The full and race suites each had 9,531 named passes
and 43 skips, and all four strict bundled-gopls checks passed. The terminal
runner reported zero source-hash mismatches and no extra Go files.

Follow-up results and native readback:
`bin/qa-artifacts/process-alarm-release.ZarCZb/follow-up/`.

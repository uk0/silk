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

Full Linux and Windows verification for this integrated candidate is not
claimed. The Windows capture in the README is from the separate
[runtime navigation candidate](HMI_RUNTIME_NAV_QA_20261117.md).

Manifests, test logs and native readback:
`bin/qa-artifacts/process-alarm-dock.igzTRy/`.

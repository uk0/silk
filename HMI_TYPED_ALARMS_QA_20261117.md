# Typed HMI alarms: implementation and verification

This increment adds authored rules, a bounded in-memory lifecycle, and an
inline alarm monitor in the standalone workbench. It does not turn display
conditions into equipment control or persist alarm history in SQLite. The
newer [IDE alarm dock verification](HMI_ALARM_DOCK_QA_20261117.md) covers the
embedded monitor separately.

## Behavior exercised

- Thresholds are type-checked, numeric trip/reset boundaries are disjoint,
  and integer domains do not pass through float64. Bad/uncertain data cancels
  pending dwell without declaring a return.
- Each occurrence has a runtime-scoped token. Recurrence/rebinding/sorting
  cannot transfer acknowledgment from the displayed occurrence to another.
  The monitor subscribes to lifecycle/quality changes; unchanged states and
  normal analog updates do not trigger another UI redraw.
- The rule editor validates the complete draft before a single Undo
  transaction. Malformed rows do not partially commit. Save/load and actual
  generated Go compilation include the authored rule.

## Native macOS replay

Tested a Unicode fixture with one latched boolean rule and a high integer
threshold of 9007199254740993. The integer value immediately below it stayed
normal. Opening **Live alarms** before Run did not create a database.
Explicit Run raised the boolean rule. One manual fixture write set DI11 to
false, then its rule returned while remaining latched. Selecting it and
pressing **Acknowledge alarm** produced the four events: raised, returned,
acknowledged, cleared. Stop and normal Quit were exercised. The project file
was byte-identical and independent SQLite readback found exactly the one
manual DI11 write, not another sample from acknowledging the alarm.

The first replay exposed state text painting over the adjacent quality
column, and an unneeded wrap in the toolbar. The repaired candidate uses
clipped cell ink and a wider state column; the raster regression proves
overflow with clipping off and no changed pixels in the adjacent column with
it on. The second native replay on a fresh store verified the layout and
the same lifecycle. Sorting the live list cleared the old selection and
disabled Acknowledge until the alarm was explicitly reselected.

## Evidence and limits

Candidate V2 SHA-256:
`04a22e4907174a242eccabb5bcea9b95f79207355ca19218f8503ed275bcc983`.
Artifacts, exact manifests, test outputs, and native readback:
`bin/qa-artifacts/process-alarms.akN8Ek/`.

The final candidate passed focused race coverage for rule validation, exact
hysteresis, dwell/quality, stale tokens, transition history, authoring Undo,
coalesced binding delivery, sorting, cell clipping and generated compilation.
On macOS arm64, the frozen full and race suites each passed 9,731 named tests
(29 skips, zero failures); all 2,902 source hashes were checked before and
after. Four strict bundled-gopls checks and the CGO-disabled alarm tests also
passed. The fresh BuildKit run is not yet a Linux pass, and no Windows alarm
matrix is claimed. Cross-platform acceptance is separate from macOS native
verification.

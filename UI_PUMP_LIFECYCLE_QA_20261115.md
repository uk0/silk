# Message-pump lifecycle and SDK cleanup — 2026-11-15

Increment after `bd4a357`. The broader product plan remains active; this is not
Qt Creator parity, an all-platform visual acceptance or a whole-IDE performance
certification.

## What runtime evidence established

The prior native startup/shutdown traces included a heartbeat warning more than
five seconds **after** the owned window had received Close. The old watchdog was
started by the first heartbeat and kept polling for the lifetime of the process.
There was no ownership boundary for an actual running message pump.

An isolated consumer now deliberately separates hidden setup, active pumping,
6.5 seconds of blocked UI work, recovery and 6.5 seconds after MainLoop returns.
The same consumer is built against the `bd4a357` baseline and this candidate.
Phase-tagged log observations, native process exit and source identity are
recorded; an empty/no-warning log alone cannot pass the test.

| Native backend | Baseline false post-pump warnings | Candidate false post-pump warnings | Active block / recovery |
|---|---|---|---|
| Windows 10 amd64, Session 1, real mouse click | 1 | 0 | 1 / 1 for both builds |
| macOS arm64, native GLFW/Cocoa, SDK-driven | 1 | 0 | 1 / 1 for both builds |
| Debian 12 arm64, Xvfb/software, SDK-driven | 1 | 0 | 1 / 1 for both builds |

All six native processes exited normally with code 0 and no data-race report.
The Windows controller checked PID-owned HWNDs and foreground admission; it
closed the owned window with WM_CLOSE, not an unverified global Alt+F4.
The first Windows observation attempt mis-bound Select-String arguments and
failed before clicking. The same live baseline process was admitted/resumed
after correcting that harness error; failed evidence is retained separately.

The Windows probe binary hashes are:

```text
baseline   e9e3de8f3d952b404e67b21493cff046f68c80834766d6b4a55d1665f41b46ac
candidate  17e6554c4a1d8acd3c5021d46a2ddb6fc1e832ffb7a0be7aa67bb84d00a2ca46
```

## Implemented ownership

- Actual MainLoop, Window.ShowModal and DoDragDrop pumps own balanced leases.
  Nested pumps share one worker. Last-owner exit cancels it; later pumps get
  fresh run/deadline state. Construction or cleanup heartbeats do not rearm it.
- The GLFW main pump cancels before explicit native/GPU teardown; defers cover
  errors and panic unwind. Windows cancels before deferred dispatcher/GPU cleanup.
- Heartbeats use process-relative monotonic nanoseconds, not Unix wall time.
- The five-second blocked-work threshold, one report per episode, recovery and
  native-dialog wait suppression remain. The positive control explicitly
  prevents accepting a disabled or indiscriminately suppressed detector.
- Cancellation does not block the UI on an arbitrary user log sink. An already
  running callback is cooperative; termination acknowledgement is tested after
  releasing it. Ready ticks after cancellation cannot start another report.
- The processlibrary CLI cleans up on all normal, operation-error, usage-error
  and help returns before main calls os.Exit. Repeated child processes prove
  restart/no-overwrite behavior and absence of stale `.runlock` files.

## A Windows-only test assumption corrected before publication

The first candidate passed ordinary Windows tests but its full race suite failed
`TestUIWatchdogPumpOwnsOneWorkerAndRearmsAfterLastLease`. A bounded overlay replay
recorded **different** run addresses, a cancelled old run and an active new run,
but equal adjacent clock readings (e.g. both `686619001`). Monotonic time need
not advance on every read. Strict `new_tick > old_tick` was not a valid portable
ownership assertion.

The final test invalidates the retired timestamp and checks independent identity,
uncancelled state and a freshly seeded timestamp within monotonic before/after
bounds. It does not sleep to force a pass or change production timing/thresholds.
The corrected case passed 50 consecutive race runs on Windows. The full final
three-platform ordinary/race matrix is rerun rather than relabelling the failed
candidate as passing.

All **937 production source files** are byte-identical between the native A/B
build source and the final archive; only `gui/uiwatchdog_test.go` changed among
compiled source files. The native binaries and real bundled-gopls probes thus
exercise the final production bytes; source-equivalence evidence is retained.

## Source and automated verification

Frozen source archive SHA-256:
`a123cb3aaedee16aafbb718e28b35081aa6e4e96c2e863a236cc9dc97a07ad9d`.
All Go/C/Objective-C/header/assembly bytes are reconciled with the final staged
tree before publication. Temporary native consumers remain separate under
`bin/qa-artifacts/ui-watchdog-20261115/`, not framework source packages.
Final broad-suite evidence is separately retained under
`bin/qa-artifacts/ui-watchdog-final-20261115/`.

```sh
go test -json -count=1 -p 2 -timeout=20m ./...
go test -race -json -count=1 -p 2 -timeout=20m ./...
go test -race -count=5 ./gui ./examples/processlibrary \
  -run 'TestUIWatch|TestUIHeart|TestModalLoop|TestServiceUIFrame|TestDeviceLibraryCLI'
CGO_ENABLED=0 go test -count=1 ./hmi ./point ./examples/processlibrary
SILK_DEBUG=1 go test -tags silk_pure_go -race -count=1 ./gui \
  -run 'TestUIWatch|TestUIHeart|TestModalLoop|TestServiceUIFrame'
```

Named subtests count; skips are not passes. Results are completed only when the
corresponding final-candidate runner is terminal and its exit status is verified.

| Platform | Full ordinary suite | Full race suite |
|---|---|---|
| macOS arm64 | 8,662 pass / 28 skip, exit 0 | 8,662 pass / 28 skip, exit 0 |
| Debian 12 arm64, Xvfb/software | 8,601 pass / 36 skip, exit 0 | 8,601 pass / 36 skip, exit 0 |
| Windows 10 amd64 | 8,442 pass / 51 skip, exit 0 | 8,442 pass / 51 skip, exit 0 |

Real bundled-gopls probes passed on all three platforms: three cases each,
nine passes, no skips (unsaved Unicode completion/formatting, renamed definitions,
and type/definition/back/implementation navigation).

The debug-enabled Windows nested-library workflow also passed: owned mapping
modal, complete insertion, single-step Undo/Redo with byte-exact file saves,
selection capture, correct catalog suffix and reopen, normal process exit 0,
no data race or stray heartbeat warning. A separate post-suite fresh-process
catalog load/save replay passed with byte-exact four-template readback and normal
exit 0. This exercises the same production code as the final archive.

The concurrent debug-enabled library run took about 16.4 seconds from the
controller's Close step through verified exit; the quiet post-suite replay took
about 2.08 seconds. These are different-load observations, not an established
before/after speedup. Actual slow cleanup/startup still warrants profiling.

## Retained allocation-counter anomaly

One final-source macOS full-race run failed the existing
`TestLineSpliceBatchAllocationCount` budget (349 reported allocations versus 4).
The splice implementation was not changed by this increment. The same frozen
source then passed 20 isolated repetitions, ten contextual `TestLine*` runs,
and a complete full-race replay (8,662 pass / 28 skip). Ten contextual baseline
runs also passed. A separate race-instrumented benchmark reports 4 allocations
per operation; it is not a release-build or whole-editor latency benchmark.

The original failed run is retained at `mac-race.jsonl`; the successful full
replay is `mac-race-replay.jsonl`. No threshold was raised and no case was skipped.
The intermittent full-process allocation count is not claimed to be diagnosed
or permanently fixed; process-wide allocation interference remains an inference,
not an established root cause.

## Explicit limits / follow-up

- The Mac/Linux controls create native windows and run real backend pumps, but
  their actions are SDK-driven. Mac authoring, visual layout and native keyboard/
  mouse acceptance remain separate; the earlier ScreenCaptureKit -3811 capture
  problem has not been declared resolved.
- This establishes and fixes one post-pump diagnostic defect. It does not prove
  all older Windows startup warnings have the same cause, remove every shutdown
  delay, diagnose the earlier partial first-frame capture, or establish an FPS /
  CPU / input-latency improvement. Actual remaining delays still need profiling.
- No dependency, signed binary release or version tag is changed by this work.

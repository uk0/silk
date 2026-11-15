# Product iteration plan — active

Goal: continue completing SilkIDE, its designer and SDK toward a coherent
Qt Creator-style Go desktop workflow. This is a continuing product goal, not a
claim of feature parity or a certified industrial control system.

Baseline: `6b9381b` (industrial 2D foundation). Preserve unrelated user files.
Each iteration must prove authored state, preview/runtime behavior, persistence,
generated code, cancellation/thread ownership and relevant platform behavior.
Do not substitute a small passing test for the whole product-readiness gate.

## Current increment: measured Windows editor performance

- [x] Capture the real non-race release build, OS process/child CPU, native
      keyboard input, saved-file readback, rendering and normal shutdown.
- [x] Reject a passing isolated cache prototype that did not improve the real
      IDE; trace the live fractional dock coordinates before extending it.
- [x] Preserve exact device pixels, alpha and fractional edge clipping while
      updating only changed minimap sample bands; bound native resource use.
- [x] Finish final frozen-source three-platform ordinary/race regression and
      identical Windows native replay; publish actual numbers and screenshot.
- [x] Recheck macOS capture: Retina SDK mouse-driven hide/show, theme, minimap
      drag, independent unchanged-source readback and normal close now pass.
- [ ] Continue large-file allocations, outline/completion/gopls CPU and native
      macOS keyboard/IME and full designer QA. One drawing optimization is not
      full Qt parity. The new native mouse test is not a keyboard acceptance.

## Validated increment: pump lifecycle and SDK resource ownership (`9b142b7`)

- [x] Reproduce a post-loop false stall with an explicit native baseline and
      final-candidate control, rather than just hiding a log message.
- [x] Scope one diagnostic worker to main/modal/drag pumps; balance nested,
      panic and concurrent unwind, cancel after last exit, and rearm cleanly.
- [x] Use monotonic heartbeat timing; preserve native-dialog waits and actual
      blocked-work/recovery reporting with a 6.5-second positive control.
- [x] Verify native Windows mouse-driven controls and SDK-driven native
      macOS/Linux controls. These do not replace missing macOS visual/input QA.
- [x] Fix CLI example cleanup on success, operation/argument failure and help;
      test repeated real child processes and independently inspect run markers.
- [x] Finish final frozen-source platform/race/LSP and nested library replay;
      document exact scope and publish only the tested source.
- [ ] Continue investigating actual Windows startup/shutdown latency and the
      earlier first-frame capture discrepancy. The diagnostic fix is not proof
      that every delay or graphics/performance issue has been resolved.

## Validated increment: reusable device authoring (`bd4a357`)

- [x] Capture immutable subgraphs with internal pipes/bends and referenced
      point definitions; never clone sources, live samples or retention policy.
- [x] Place independent prefixed instances atomically; explicitly share only
      compatible point definitions and preserve the host acquisition owner.
- [x] Add inert searchable device library, point mapping, selection capture,
      responsive owned dialogs and a single insertion Undo transaction.
- [x] Add bounded catalog files, owned asynchronous GUI I/O, explicit overwrite
      consent, revision checks and nested-dialog/worker retirement.
- [x] Verify `.silkui` persistence, preview and independently compiled concrete,
      scene-declarative and silkgen consumer constructors. Fix the missing
      concrete catalog-property setter exposed by that regression.
- [x] Finish frozen-source three-platform ordinary/race regression, native
      Windows placement/capture/save/reopen readback and README screenshots.
- Publication uses the tested increment's Git commit plus matching remote SHA;
  README screenshots, changelog and QA evidence accompany that commit.
- [ ] Continue Windows startup/shutdown latency tracing. A new concurrent-load
      repetition recorded a 5.882-second heartbeat delay; another bounded probe
      warned after Close. The final post-suite file/UI replay passed, but that
      does not prove every intermittent latency issue fixed. Pump-owned
      diagnostic lifecycle has since been corrected and natively verified;
      actual startup/shutdown timing and allocation-counter noise remain open.

This increment produces ordinary editable nodes, not persistent linked prefab
instances. Catalogs require explicit file persistence in the workbench/IDE;
`.silkhmi` stores placed content, not the separate catalog. Device authoring
does not complete the remaining runtime, alarms, routing or platform QA gaps.

## In progress: direct 2D authoring

- [x] Implement pure-Go, validated, non-mutating group edit SDK operations.
- [x] Implement multi-selection, directional marquee, group move, resize, undo,
      keyboard commands, alignment/distribution/layering and editable waypoints.
- [x] Keep authored geometry separate from live visibility/position bindings.
- [x] Preserve exact text/body input and expose real action availability.
- [x] Complete full ordinary/race suites on macOS arm64, Linux arm64/Xvfb and
      Windows amd64; changed-package and no-CGO SDK regression also passed.
- [x] Verify Windows native authoring/menu/history/save readback and normal exit;
      add real screenshots to README and document the editing SDK.
- [x] Measure preview allocations (logic-only, not rendered FPS).
- [x] Finish isolated Windows debug/performance/export repetition: no race or
      heartbeat warning; authored-file reload and actual Go export succeeded.
- [x] Compile the actual native Go export with race instrumentation; run that
      generated application, save/read back authored coordinates, close normally.
- [ ] Finish native macOS authoring. The older ScreenCaptureKit -3811 capture
      failure no longer reproduced in the November 15 isolated SDK editor test;
      full IDE/HMI authoring and keyboard/IME readback are still not accepted.
- [ ] Investigate the one Windows heartbeat delay observed during concurrent
      compiler/test activity. Isolated repetition did not reproduce it; do not
      infer a root cause or declare all performance targets achieved.

## Next product gaps (not completed by the current authoring work)

- In progress: `.silkhmi` JSON source mode, Tools/palette visual editing,
  revision-guarded async decoding, one host undo transaction and ordinary Save
  integration are implemented and tested after `bd3019d`.
  Node-to-source-caret selection and integrated runtime panels remain open.
  The first candidate passed functional native editing/byte-exact host history,
  but its debug-enabled screenshot exposed inherited placeholder painting.
  It was retired. Explicit themed composite painting and light/dark raster
  checks now pass (including debug and pure-Go renderer checks); the frozen
  final candidate passed fresh three-platform full suites and Windows native
  regression recorded at
  `bin/qa-artifacts/hmi-ide-final-20261114/`.
  Final Windows debug-enabled native editing/history/ownership and a separate
  non-debug session-restore/cancel run have passed, including normal exits.
  Strict real bundled gopls cases passed on all three platforms (three each).
  One initial capture under compiler/test activity showed partial equipment;
  a fresh quiet run's initial/+1s/+4s client-area pixels were identical and complete.
  Distinguishing capture/compositor timing from application first-frame damage
  remains open; do not claim it diagnosed or permanently fixed.
- Obstacle-aware pipe routing with explicit port directions, bounded work,
  deterministic fallback diagnostics and geometry-revision caches.
- Equipment templates, point-prefix mapping and the first practical device
  library are implemented in the current increment. Persistent linked-prefab
  updates and a fuller inspector workflow remain future work.
- Typed live-point monitoring and explicit operator/driver writes; preserve
  history/quality and reject competing writers or stale UI completions.
- Alarm lifecycle SDK: hysteresis, delay, quality gating, latching/acknowledgment
  and history, independent of simple display color rules.
- Extend full-quality bindings and safe lifecycle ownership across legacy
  widgets, while preserving existing compatibility contracts.
- Continue editor/LSP navigation/refactoring polish, event-driven rendering,
  DPI/font/icon precision and consistent menus/buttons across platforms.

## Acceptance strategy

1. Exercise input through the real decisive branch; compare pre/post authored
   documents and independently reopened storage, not only an intent/status log.
2. Verify multi-step undo/redo, Escape/capture loss/resize/close cancellation,
   rejected edits, offline/malformed data and recovery.
3. Compile actual generated Go and validate designer save/load/preview.
4. Run changed-package race tests, broad regression and platform/native UI
   checks. Record skips, architecture, source identity and failed candidates.
5. Keep QA exports out of source packages; never pollute a test tree with an
   additional generated `main`. Keep tested code and final staged code aligned.
6. Maintain README/changelog and explicit limits. Commit/push/release only with
   the applicable user authorization and after the required gates actually pass.

The goal remains active until product requirements and their full verification
are met. No individual checklist section defines the full goal as complete.

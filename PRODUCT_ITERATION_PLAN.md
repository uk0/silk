# Product iteration plan — active

Goal: continue completing SilkIDE, its designer and SDK toward a coherent
Qt Creator-style Go desktop workflow. This is a continuing product goal, not a
claim of feature parity or a certified industrial control system.

Baseline: `6b9381b` (industrial 2D foundation). Preserve unrelated user files.
Each iteration must prove authored state, preview/runtime behavior, persistence,
generated code, cancellation/thread ownership and relevant platform behavior.
Do not substitute a small passing test for the whole product-readiness gate.

## Validated increment: source-backed industrial runtime inside SilkIDE

- [x] Add a reusable inert runtime view with shared background acquisition/store
      ownership, responsive diagram/point views, explicit Run/Stop and off-loop Wait.
- [x] Run the exact current `.silkhmi` buffer in an owned IDE dock; isolate its
      local point store by document identity and preserve source/Undo/disk state.
- [x] Wire menu/palette/F5, source navigation, source-change/close/rename/quit
      retirement, stale asynchronous-load guards and hidden-pane stop behavior.
- [x] Prove typed HTTP-to-view, manual writes/history and cancellation from a clean
      baseline; run SDK/IDE/layout races and final full platform/native gates.
- [x] Update real screenshots, README/SDK/changelog; verify final three-platform
      ordinary/race, 12 strict gopls probes, Windows native/SQLite replay and all
      2,004 publication build inputs before commit/push. Keep macOS-native gaps open.

## Next increment: startup I/O and remaining industrial workflow

- [ ] Trace and remove UI-thread file-open stalls during workspace restoration;
      prove cancellation, retained source identity and late-result guards before
      changing startup behavior. The current native macOS observation is not a pass.
- [ ] Complete real macOS-native startup, keyboard/IME and source-runtime checks
      without changing personal preferences or bypassing OS permissions.
- [ ] Continue point-source configuration, explicit store/type migration, alarms,
      device routing and SDK icon gaps with separate persistence/native evidence.
      Do not label these or complete Qt Creator parity as already delivered.

## Validated increment: industrial live-point monitoring and explicit writes (`087c8a2`)

- [x] Add atomic tag sample/revision snapshots, runtime-local optimistic write
      tokens, typed/manual writes, HTTP-writer protection and durable-before-live
      publication without blocking memory reads behind store I/O.
- [x] Add bounded recent/range history capabilities without changing the legacy
      SQLite StoredSample type; validate restored/history samples and reader bounds.
- [x] Add an inert, designer-placeable ProcessPointMonitor with coalesced live
      updates, precise integer sorting, explicit owned write/history dialogs,
      cancellation and close/rebind guards. No per-widget acquisition/poll loop.
- [x] Integrate the standalone workbench and event-driven RuntimeChanged lifecycle;
      hide inactive authoring tools in live mode to preserve the diagram viewport.
- [x] Exercise SDK, GUI/dialog lifecycle, palette/codegen and layout regressions
      under race detection. Keep final full/native acceptance separate.
- [x] Complete broad regressions, real Windows UI/SQLite readback and frozen-source
      full three-platform ordinary/race + strict bundled-gopls gates; update README
      screenshots, SDK/changelog and commit/push only the verified implementation.
- [x] Reproduce and fix panel hide/show rebinding a stopping/unavailable runtime;
      independently refreeze and rerun all platform gates, not the earlier source.
- [ ] Continue alarms, device routing and remaining
      Qt Creator/editor/macOS-native and default SDK icon gaps. This is not a
      certified control system.

## Validated increment: exact incremental local completion candidates (`275e1c8`)

- [x] Re-profile the published body-projection baseline with a separate, minimal
      allocation-capture overlay. Local lexical scanning is about 54% of sampled
      allocation; independently parse the saved native input and retain the patch.
- [x] Add worker-private lexical chunks, stable first-occurrence dedup ordering,
      owned identifier bytes and bounded sidecar fallback; preserve all candidates.
- [x] Retire sidecar/cache payloads on shrink, language change and close without
      blocking the UI. Keep existing 90 ms debounce, two-worker permits and
      generation/revision/caret delivery guards, not a new timer or polling loop.
- [x] Check complete-scanner parity, invalid UTF-8 seams, budgets, cancellation,
      owned results and real editor Undo/Redo/provider behavior with race tests.
- [x] Complete final-source differential fuzz, seven matched Windows native
      controls and independent saved-source parsing. Body allocations fall
      53% / IDE CPU time 7–10%; keep gopls and latency limits explicit.
- [x] Pass frozen-source macOS/Linux full ordinary/race and strict bundled-gopls
      gates; verify all 2,680 source files and current 1,986 build inputs.
- [x] Complete full three-platform ordinary/race and 12 strict real-gopls
      probes; independently reconcile Windows post-run identity and extra-Go
      checks. All runners exit zero; publish only the exact tested build inputs.
- [x] Finish final read-only native gallery, restored-fixture readback and
      documentation/index audit. All publication gates are ready; keep
      unrelated user artifacts outside this commit/push batch.
- [ ] Continue language-server CPU, native macOS keyboard/IME and the full
      designer/runtime roadmap; this increment is not the whole product.

## Validated increment: exact bounded function-body symbol projections (`8f49636`)

- [x] Reprove body interiors with the existing Go lexer; preserve physical
      lines and every exposed symbol field, with full AST fallback otherwise.
- [x] Bound UI lexical work and metadata copying; share unchanged immutable
      spans rather than regress EOF-comment typing with whole-array copies.
- [x] Prime metadata through both SDK synchronous and background paths; cover
      Undo/Redo, provider/language invalidation, close, ready draw/navigation.
- [x] Add repeated-edit differential fuzzing alongside full-parser comparison.
- [x] Reconcile seven native Windows controls, independent saved-source AST
      readbacks and normal exits. Body input in both run orders cuts IDE CPU
      31–37% / cumulative allocation 68%; comment-only and gopls CPU do not gain.
- [x] Verify native macOS SDK mouse Edit/Undo/Save and normal shutdown. Keep
      later ScreenCaptureKit failure and untested keyboard/IME branches explicit.
- [x] Finish frozen-source ordinary/race suites on all three platforms and
      12 strict real-gopls probes. Reconcile all 2,676 manifest files and 1,981
      staged build inputs; refresh native README image, SDK docs and changelog.
- [ ] Continue completion-index/server CPU, native macOS keyboard/IME and the
      broader designer/runtime roadmap. This is not an incremental Go compiler.

## Validated increment: negotiated incremental LSP synchronization (`d11ffd5`)

- [x] Trace the real full-text transport and verify LSP capability, ordering,
      UTF-16 and CRLF requirements before changing the protocol.
- [x] Add additive SDK delta APIs, safe full fallback, last-successful wire
      baselines and differential/ownership/order regression.
- [x] Reject the slower first diff prototype; profile and optimize unchanged
      prefix/position work without weakening text equivalence checks.
- [x] Complete frozen-source ordinary/race and strict real-gopls regression on
      all three platforms, plus matched uninstrumented Windows native replay.
- [x] Reconcile 1,978 publication build inputs, refresh the native screenshot,
      SDK docs/changelog and exact QA evidence; all release gates are ready.
- [ ] Continue ordinary-body symbol analysis, completion-index/server CPU,
      macOS keyboard/IME and designer/runtime roadmap gaps. An isolated body
      projection prototype is not part of this transport publication.

## Validated increment: incremental analysis and SDK snapshots (`8f802d6`)

- [x] Capture a sampled-allocation profile of the actual Windows typing flow;
      distinguish AST, outline rebuilding, completion and JSON allocations.
- [x] Add immutable exact comment projections and content-versioned SDK
      snapshots without disabling completion features or replacing Go parsing.
- [x] Reuse equal outline trees while forbidding stale pending draw/navigation;
      test ownership, editor/provider identity and nonblocking worker retirement.
- [x] Complete uninstrumented native release replay and frozen-source full
      three-platform ordinary/race regression plus nine strict bundled-gopls
      probes. Keep the publication index byte-identical to tested build inputs.
- [ ] Continue local-completion indexing, LSP full-text transport/server CPU,
      macOS keyboard/IME and full designer authoring. Do not call a partial
      analysis optimization a complete compiler or a complete product.

The earlier symbol-snapshot allocation profile attributed about 42% to local
completion scans and 25% to JSON serialization. Negotiated incremental LSP
sync and the current body projections subsequently change that distribution;
re-profile the final native candidate before quoting those shares as current.
The local completion worker already caches one revision, but text changes
invalidate its entire lexical identifier scan. Investigate exact incremental
identifier occurrence/dedup ordering without sharing mutable UI state, changing
candidate semantics or retaining unbounded source snapshots. Keep differential
and lifecycle/race checks, strict real-gopls probes and matched native replay;
do not merely lengthen debounce or suppress language features to lower CPU.

## Validated increment: measured Windows editor performance (`cd5da66`)

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

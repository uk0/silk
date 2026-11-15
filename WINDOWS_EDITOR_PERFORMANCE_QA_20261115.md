# Windows editor rendering performance — 2026-11-15

## Reproduction and acceptance

Use Windows 10 amd64 Session 1, native keyboard input and the actual release
build flags, **without race instrumentation**. Start a fresh process for each
capture, restore the same isolated settings and canonical CRLF Go fixture
(5,000 one-line functions), use the same bundled gopls and 1280×940 window.
Input 161 ASCII characters at 125 ms intervals, save, independently read the
file, verify the expected content/hash and close the PID-owned window normally.
Do not run builds or test suites on the Windows host during a capture.

The bounded profiler starts in `main`, not during package initialization.
Record process CPU deltas from Windows separately from pprof, plus paint count,
paint wall time, memory, direct-child gopls CPU and normal exit. `SendWait`
duration measures the automation call, **not key-to-photon latency**. This is a
large-file reproduction, not a claim of universal performance or Qt parity.

Baseline: `9b142b73622beb525459c950875b22e77e3007f2`.
Raw evidence, immutable source manifests and replay scripts are kept separately
under `bin/qa-artifacts/windows-performance-20261115/` (ignored QA artifacts).

## Observed baseline and rejected first candidate

- A 45-second idle capture recorded only eight paints. Between the first/last
  process samples (41.19 s apart), the IDE used 0.546875 CPU seconds: about
  **1.33% of one core**. Bundled gopls indexing was still active separately.
- During the controlled typing interval, the IDE used 18.328125 CPU seconds in
  28.6627533 elapsed seconds: **63.94% of one core**. Direct-child gopls used
  about 33.80 additional CPU seconds. These are not host-wide CPU percentages.
- The full capture recorded 323 paints / 12.6858176 s paint wall time and
  1,486 MiB cumulative Go allocations (not retained memory).
- Active editor stacks repeatedly submit the entire minimap's translucent
  rectangles to Cairo, including unchanged rows. Do not confuse these active
  drawing stacks with blocked native message or pipe waits.
- The first cache prototype passed exact-pixel and full macOS/Linux tests, but
  **did not improve the real Windows replay**: 18.703125 CPU seconds in
  29.4208601 elapsed seconds; 315 paints / 13.2193846 s paint wall time.
  Its source archive is retained as
  `04c1f557d178627043e235112a53cf708ee01d5ce46c4909ea92083a554ed019`.
  Passing isolated cache benchmarks was not treated as product acceptance.

## Final native replay

The diagnostic overlay observed the real minimap at logical
`(725,22,60,490.044921875)`, with device translation
`(242,124.955078125)`. Thus a prototype restricted to integer outer bounds
could never activate. The final renderer caches only opaque interior device
pixels. Guarded scratch rendering retains the original fractional clip and
masked-fill rounding; fractional outer pixels remain vector draws. Native
path/clip knowledge rejects curved/disjoint masks rather than assuming their
bounding box is the clip. Custom/vector/pure-Go painters stay uncached.

Final source archive SHA-256:
`32f41575c5577c6809b71343cc300585f9659d8d1645a185c7a43819b57d7de2`.
Windows release binary SHA-256:
`cb71ab9b1b80a468c31ae70f3b1eef1c29e49a35e50825c29e88d1782dcc6ce3`.

All four controlled typing runs independently saved the identical SHA-256:
`ec798c639c8abfc910c01d8578e5bdb8ea54020d3d1a1345fcd13e548e8d526b`.
They each typed 161 characters and exited normally with code 0. All captures
are 45 seconds; paint time is CPU-side paint wall time, **not displayed FPS**.

| Measurement | Baseline A1 | Baseline A2 | Final B1 | Final B2 |
|---|---:|---:|---:|---:|
| Typing interval, s | 28.66 | 29.26 | 29.14 | 28.98 |
| IDE CPU during typing, s | 18.33 | 19.72 | 18.06 | 18.41 |
| IDE CPU, % of one core | 63.94 | 67.40 | 61.99 | 63.52 |
| Paint count | 323 | 279 | 446 | 446 |
| Total paint wall time, s | 12.69 | 13.52 | 8.51 | 8.15 |
| Mean paint wall time, ms | 39.27 | 48.47 | 19.08 | 18.27 |
| Cumulative Go allocation, MiB | 1,486 | 1,236 | 2,018 | 1,991 |

This is a repeatable **drawing** improvement, not an overall CPU solution.
Paint count and cumulative allocations increased; remaining symbol/completion
work and asynchronous invalidation require a separate measured iteration.
Do not claim a total-allocation reduction, faster typing, end-to-end latency
target, or a universal speedup from the minimap benchmark.

The final idle control again recorded eight paints over 45 seconds, without a
new polling/render loop. Native captures close only the exact PID-owned HWND;
gopls shutdown is observed rather than inferred from a closed-looking window.

README screenshot: `docs/images/silkide-editor-minimap-windows.png`, captured
from the final release binary after save/readback and return to the file top.
Screenshot SHA-256:
`8f6cdab2c96490495bf24ba4fb37b14a9a4e7f63903600c77ca29ce38e5a4e0d`.

## Frozen-source regression

Both full suites use `-count=1 -p 2 -timeout=20m ./...`; race adds `-race`.
Named subtests count; a skip is not a pass. All runner exit statuses are 0.

| Platform | Full ordinary | Full race |
|---|---|---|
| macOS arm64 | 8,708 pass / 28 skip | 8,708 pass / 28 skip |
| Debian 12 arm64, Xvfb/software | 8,647 pass / 36 skip | 8,647 pass / 36 skip |
| Windows 10 amd64 | 8,498 pass / 41 skip | 8,498 pass / 41 skip |

Debug-enabled pure-Go raster fallback race tests, the no-CGO point/HMI/library
SDK regression and the isolated minimap microbenchmark also exited 0. The
pure-Go backend deliberately does not enable the native cache; native-only
cache ownership/capability checks skip there, while pixel comparisons run.

The Windows runner validates all 2,658 archived files before/after testing;
the independent Linux working copy also matches the manifest. All **1,966
compiled-source/build-input files** are reconciled with the final publication
tree; documentation and new screenshots are completed after source freeze.

## Native macOS scope

The earlier ScreenCaptureKit -3811 failure did **not** recur in this run.
A separately compiled consumer of the frozen SDK was driven through CUA on a
Retina display: hide/show the minimap, switch dark/light, drag its viewport,
click Save and close the actual window. Both saved states independently match
the original 195,033-byte LF fixture, SHA-256
`ec111c8989f056f157dd4f08135f6f4e2079930e2706717907ec0b04597112af`.
The owned process printed `NORMAL_EXIT` and exited 0.

This is a native **SDK mouse/rendering/readback** acceptance, not full SilkIDE,
designer, LSP or keyboard/IME acceptance. Keyboard automation attempts did not
produce the expected persisted text and are not counted as passes; their cause
has not been established. A light-theme breadcrumb/chrome consistency check
also remains part of the UI-polish backlog. Do not infer a framework input bug
or successful input from a tool call alone.

Replay uses screenshot-relative physical pixels (the 2× image is 2200×1464):
Save `(270,1320)`, minimap toggle `(800,1320)`, theme `(1328,1320)`, minimap drag
`(2060,200)` → `(2060,1150)`, Close `(1856,1320)`. SDK `MapToGlobal` reports
screen logical points and is **not** the coordinate space of these CUA clicks.
Keep the launch process alive, use an isolated `SILK_DATA_DIR`/explicit result
path, and rebind the app after a process restart. Earlier calibration/input
attempts are retained separately from the passing mouse replay.

Native image evidence:

```text
mac-native/native-light-drag.png
  1de800e5236ef064420c18864126767974bbece9efc524802bea98afdf2669cf
docs/images/silk-editor-minimap-macos.png
  31999a9395e7790786a74160643dd5602a2d4c2826dabd6e42de4802714f5c70
```

## Instrumentation limitations / retained failed harnesses

- Windows pprof reported roughly 130 cumulative sample seconds in a 45-second
  capture, dominated by blocked native message/pipe stacks. This conflicts
  with the independently sampled OS process CPU. Do **not** present the
  apparent pprof percentage as actual CPU utilization or optimize blocking
  waits on that basis. Use it only with corroborating runtime evidence.
- An initial input-language request cycled the active layout and produced
  Chinese IME composition rather than the intended ASCII text. Preserve that
  workload separately; it is not a passing ASCII replay or evidence of lost
  editor input. The corrected harness requests the explicit English layout
  and checks the owning UI thread's active layout before typing.
- Save formats the fixture's final `main` declaration. The canonical fixture
  is the independently read-back formatted version, not the initial raw
  generated bytes. No application state is inferred from a screenshot alone.

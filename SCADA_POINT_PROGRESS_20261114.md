# Industrial 2D workbench verification — 2026-11-14

**Verdict: PASS for the implemented 2D scope.** All commands below completed;
this is not a claim of Qt Creator parity, certified SCADA or real PLC validation.

## Delivered

- Portable inert `.silkhmi` documents and ProcessDiagram / ProcessEditor factory,
  designer drop/save/load/preview/generated-Go integration and `cmd/silkhmi`.
- Nine schematic node kinds; snapped drag, connected/rotating ports, orthogonal
  dogleg routes and table-authored waypoints, zoom/pan and bounded Undo/Redo.
- Staged point/source/header/mapping/conversion/color/property/pipe tables.
  Reject invalid candidates without replacing the document; preserve advanced
  pipelines. Load resets cross-document undo history.
- DI/AI/BI categories independent of scalar types; optional SQLite latest/history,
  stored-point restoration, SDK publishing, shared TTL and bounded retention.
- Explicit owned runtime lifecycle, shared bounded HTTP acquisition, body/header
  snapshots, JSON Pointer mapping, conversion/comparison and quality-aware UI.
- Fix first-header overpainting and table scroll-layer pointer interception.
  Bars/inline editors retain their hits; nested dialogs now genuinely edit.
  Keep pipeline/gradient caches and eliminate JSON parsing on pointer moves.

## Final frozen-source matrix

Go 1.26.10, CGO enabled, `go test -json -count=1 -p 1 -timeout=30m ./...`,
then the same command with `-race` (the final Windows race run used `-p 2`).
Linux used Debian 12 arm64 with Xvfb in
`silk-qa-gl-dispatch-warm:20261028`, not a physical Linux desktop/GPU test.

| Platform | Full suite | Full race suite | Exit codes |
|---|---:|---:|---:|
| macOS arm64 | 8623 pass / 28 skip | 8623 pass / 28 skip | 0 / 0 |
| Linux arm64 | 8562 pass / 36 skip | 8562 pass / 36 skip | 0 / 0 |
| Windows amd64 | 8403 pass / 51 skip | 8403 pass / 51 skip | 0 / 0 |

Counts are Go test/subtest completion events, not unique test functions.
Skips are explicit, not passes; their full names are preserved in the local QA
summary. Default-suite real-gopls skips were supplemented by **three strict real
bundled-gopls probes per platform**, all passing without skips: unsaved Unicode
completion/formatting and progress, rename/definition after rename, and
unsaved type/definition/implementation/back navigation. The pinned bundle
(v0.24.0 / Go 1.27.3) passed the existing version/target/digest contract. This
checks integrity/compatibility, not publisher signing. Remaining fixture,
permission and platform skips are not hardware validation.

Additional passing gates: focused point/HMI/designer race tests; `go vet` for
point/hmi/hmiapp; CGO-disabled point/hmi tests (SQLite correctly unavailable);
GUI dependency isolation from SQL/SQLite; generated standalone workbench builds
and designer-generated builds. Native UI exported Go was also built on both
macOS and Windows.

## Native UI and persistence

- **macOS arm64 and Windows 10 amd64 Session 1: PASS**, using race-enabled
  workbench binaries, not cross-compiled UI claims.
- Actual cell editing and invalid type Apply rejection, Cancel preservation,
  drag/Undo/Redo, Save/Load readback, generated Go and compact layouts.
  macOS additionally clicked two actual ports and verified the saved third pipe.
- Loopback POST with exact request body/custom header; green 72.5, yellow 92.8,
  red 120.0, HTTP 503, malformed numeric field with a valid DI sibling, recovery.
  Both point buses and independently reopened SQLite histories were checked.
- Stop unlocked authoring only after cancellation/join; request counts stayed
  unchanged afterward. Closing during a pending 30-second request exited
  normally: macOS 1.09s, Windows 1.26s, exit 0.
  Race runtime exit delay is included; these are not release-build latency claims.
- README images are actual final-candidate captures, not UI mockups.

Windows race-build short samples: idle 0.109 CPU seconds /
10.01 wall seconds; HTTP/runtime 0.672 CPU seconds /
10.03 wall seconds. CPU seconds are per-process CPU time,
not whole-machine percentage. No FPS, long-run load, hard-real-time or high-rate
historian performance is inferred from this ten-second sample. No per-widget
polling/animation timer was added; unchanged observations do not enqueue redraws.

## Provenance and reproducibility

- Base: `25261df00850ac0c362a6d0a74395c2a637b7606`; frozen files: 2619.
- Source archive SHA-256: `9be3a0505fb7c7e16e9a95fc03b049298bdcdb0649bb79af1af614971c8ff27f`.
- Manifest SHA-256: `51771b9f2a4a560e291e63b3fdcefef56e14a54ae7cb6119e6866da48e6346f8`.
- Local evidence: `bin/qa-artifacts/hmi-workbench-verified-20261114/` contains
  JSONL/exit files, source pre/post checks, native screenshots/state/request logs,
  independent SQLite readback, bundled-LSP probe logs and the staged-tree audit.
- Runtime code/assets remain byte-identical to this tested snapshot. Only final
  documentation/screenshots changed afterward. The pre-existing
  `core/callback.go.txt` CRLF-to-LF Git normalization is recorded separately;
  no new Go source has that exception.
- Earlier candidates and harness failures remain separate. Real desktop testing
  exposed the table-input bug and led to the final source snapshot. The Windows
  harness was corrected for console/window ownership, UTF-8 input and an empty
  PowerShell property-count check. A GUI-exported main temporarily placed in
  the Windows SDK QA root caused a `main redeclared` race-build failure; it was
  moved to an ignored evidence directory, the root race test passed in isolation,
  and the entire clean Windows race suite was rerun. A wrong-architecture Linux LSP fixture was
  rejected and replaced with the verified arm64 bundle. These are not counted as
  successful original runs or hidden product-test passes.

Re-run ordinary/race suites with the commands above; use
`make test-ide-lsp` for the strict packaged-LSP contract and
`go run ./examples/process2d` for a local synthetic state/HTTP demo. The visual
workflow and SDK entrypoints are in [HMI workbench](docs/HMI-WORKBENCH.md).

## Explicit limits

Basic schematic symbols, not a certified P&ID catalog. No obstacle-avoidance
routing, draggable bend handles, PLC write/control integration, alarm
hysteresis/delay/ack, 3D additions or physical Linux/hardware-GPU certification
in this change. Multi-point delivery is not an atomic PLC scan. Bounded retention
is not a hard disk quota; direct external SQL writes are not a live subscription
feed. Use Runtime.Publish / the shared Tag bus for live driver updates.

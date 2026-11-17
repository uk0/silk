# Bounded, shared process-pipe geometry — 2026-11-17

Verified development-source increment after `410c272`; not complete
Qt Creator parity, industrial certification or a binary release.

## Reproduced problem and correction

The old public route from `(120,130)` to `(400,130)` crosses a third device at
`(220,70)-(300,190)`. The preserved negative exits 1:
`bin/qa-artifacts/pipe-routing-20261117/negative.log`.

- `Link.Routing` explicitly selects `avoid`; empty/`orthogonal` retains existing
  version-1 doglegs. New interactive connections default to avoidance. Pipes
  edits remain staged; old six-column SDK edits preserve an existing mode.
- `PipeRouter` owns/indexes conservative obstacle bounds, outward cardinal port
  directions and mandatory waypoint order. Port positions are not snapped.
  Rotated clients use nearest-cardinal normals and full rotated bounding boxes,
  including labels. This is not full vector-angle or optimal P&ID routing.
  Only an endpoint's own entry/exit stub may traverse its owner envelope.
- Rectangles round outward. A first randomized test caught a one-ULP boundary
  penetration caused by reassociating `x + width + clearance`; its failed log is
  retained separately, and independent segment assertions now pass.
- Per-request search states and cumulative geometry work are bounded. Defaults
  are 2,048 visited states / 200,000 indexed-collision work units; explicit options
  also have hard caps. Interval indexes skip distant devices. GUI budget priority
  follows authored pipe order, independent of which observer asks first.
- Blocked/budget results return a visibly diagnosed legacy fallback, not claimed
  avoidance. Invalid input produces no drawable coordinates. Amber pipes and a
  fixed-viewport banner distinguish failure from normal live flow colors.
- Painting, picking and waypoint insertion share geometry. Transient drag,
  position/visibility/rotation bindings, cancellation and history invalidate it;
  colors, selection, camera and unchanged grid-snap moves do not rebuild it.
  Public route/diagnostic reads are UI-thread-only defensive copies. Computed
  paths never enter authored JSON, history, generated Go or point persistence.

## Preflight evidence

- Independent segment checks cover every successful route in 200 seeded random
  scenes. Ten thousand indexed queries match a flat open-rectangle oracle,
  including exact boundaries and skipped endpoint owners.
- Cover all endpoint-direction pairs, waypoint order, blocked exits/walls,
  invalid/nonfinite inputs, deterministic order, cumulative and per-search limits,
  independent inputs/results, schema/clone/device capture and placement.
- A 512-obstacle fixture routes 100 requests using 63,900 shared work units.
  A 512-node / 1,024-link GUI snapshot reaches but never exceeds its shared cap,
  reports fallbacks and gives identical results for forward/reverse first queries.
- GUI tests exercise picking the actual detour, rejecting the invisible old path,
  live quality/geometry, rotation, drag/cancel/waypoint history, staged routing
  edits and fixed-viewport clipped warnings in light/dark raster tests.
- Designer drop/save/load/preview/generated constructors and the inert routing
  example pass. Changed-package race suites (`hmi`, `gui`, `ged`, `hmiapp` and
  `examples/processrouting`) pass; no-CGO HMI and GUI SQL-dependency isolation pass.

Three quiet macOS arm64 preflight repetitions report cached private pipe-geometry
queries at **0 B/op, 0 allocs/op** (about 9.2–9.4 ns/op on this M2 Max). This is a
map/cache lookup, **not complete drawing, native input, startup or FPS**. Cold
3-obstacle index-plus-route construction allocates 13,232 B / 156 allocations;
512-obstacle construction about 216 KB / 855 allocations. Cold geometry is not
zero-allocation. Benchmarked Go sources match the frozen candidate.

## Frozen platform gates

Archive SHA-256:
`3cb7461226737765fdd562baec171b89fb5034162822e381b9718dd5d7e869a0`.
Manifest: 2,883 paths, including 2,106 Go / 2,112 Go-and-module paths. Full suites
run in isolated trees, never a generated-main-contaminated SDK root.

| Platform | Full named pass / skip | Race named pass / skip | Strict actual bundled gopls |
| --- | ---: | ---: | ---: |
| macOS arm64 | 9,698 / 29 | 9,698 / 29 | 4 pass, 0 skip |
| Linux arm64 / Xvfb | 9,637 / 37 | 9,637 / 37 | 4 pass, 0 skip |
| Windows 10 amd64 | 9,495 / 43 | 9,495 / 43 | 4 pass, 0 skip |

All three platform gates exit 0. All 181 renderer-subset cases pass without skips; scoped
vet passes. Full vet still exits 1 with 501 existing diagnostics, verified as
unchanged messages on unchanged source lines after relocation, not a clean vet.
Independent Linux and Windows exports match every manifest path without extra Go files.
Linux Xvfb is not physical Linux/GPU certification.

## Native and publication boundary

Windows same-candidate full/race/strict checks have finished successfully.
A read-only QA native host
has been compiled on macOS; that proves compilation, not a native UI pass. The
Windows host launched only after its full platform job succeeded. Its copied
example-project function is hash-correlated with the frozen source; `/state` only
reads on `gui.Post`. Native edits, menus, writes, Save/Generate and close require
actual desktop input. No observer-side action substitutes for a user operation.

Current macOS native authoring has not been accepted for this candidate. The
previous increment's ScreenCaptureKit failures are retained separately and do
not prove this candidate's native behavior; permissions/preferences are untouched.

Native authoring has already exercised preview/cancel, committed movement,
Undo/Redo, independent moved-file save, picking the actual detour, Arrange's
waypoint action, blocked preview/cancel and a committed amber fallback followed
by Undo. One harness wait incorrectly treated an absent JSON bends field as an
empty PowerShell array: `@($null).Count` is 1. Its failure is retained. Read-only
inspection and a corrected checkpoint confirm the expected no-bend state on the
same owned app without repeating the completed input actions. SDK source is
unchanged; the corrected replay script is retained alongside the original.

A scheduled PowerShell task also took foreground before the first write. Its
pre-input guard rejected the attempt, and inspection proves no write marker or
value change occurred. Focusing the unique, already visible owned dialog fixed
the harness without reopening it. The successful native write is independently
verified as exactly one `DI_SHOW=false`, quality-good SQLite latest/history row.

Windows native Run uses the current unsaved y=120 drawing, not the previously
saved y=300 file. The real conditional write hides the reactor and changes the
shared route from a detour to a straight line. Authored initial visibility remains
true. Stop restores the authored obstacle; Redo restores the valid mandatory bend.
Save and Generate preserve that configuration, not live values or cached paths.
The original workbench closes normally with held-handle exit 0 and empty stderr.

The byte-identical actual native export passes a separate macOS race consumer
test for saved/embedded document identity and actual public diagram routing. This
does not run its main's macOS UI. On Windows the actual export is compiled with
race instrumentation and executed as a separate native application, not a QA
helper. Its real Save produces the exact original project SHA-256, with no point
store created before Run; its held-handle exit is 0 and stderr is empty. Three
original README images show that exported app, the amber failure and live binding.

Publication still requires the exact source/index/media audit and independent
remote readback. Per the cleanup request, disposable source/build trees can be
removed after publication. Preserve source manifests/archive, scripts, logs,
original captures, native documents/database and a compact verification bundle
under `bin/qa-evidence/`; cleanup is not loss of the tested-source identity.
Raw artifacts live under `bin/qa-artifacts/pipe-routing-final1-20261117/`; negative
and preflight evidence is separate under `bin/qa-artifacts/pipe-routing-20261117/`.

```sh
go test -race -count=1 ./hmi ./gui ./ged ./hmiapp ./examples/processrouting
CGO_ENABLED=0 go test -count=1 ./hmi
go run ./examples/processrouting
```

This does not solve electrical connectivity at crossings, pipe bundles, persistent
linked-prefab updates, PLC command protocols, alarm lifecycle, all-widget popup/
DPI/IME behavior or complete Qt Creator functionality. The product goal stays active.

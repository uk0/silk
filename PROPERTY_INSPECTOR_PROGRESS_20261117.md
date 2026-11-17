# Coherent property inspector — verified development source

Baseline `2a68b14a290a2b61c38c1af5ac68a0d8e00a3d02`.
The whole SilkIDE/designer/SDK product goal remains active. No new release/tag,
full Qt Creator parity or whole-product completion is implied. Final frozen,
platform and native gates pass; publication requires the exact index/media audit
and independent matching remote commit readback.

## Implemented

- Exact categories for built-in Chinese/English IDs, while preserving persisted
  IDs and unknown extension heuristics. HTTP/body/response/conversion/device rows
  join Data Binding; titles, state and grid placement stop falling into General.
- English/Chinese category search plus canonical/legacy aliases, custom labels,
  hidden-row and multi-selection event exclusions. Clearing a filter restores
  the stored collapse; whitespace follows the same rule in layout and chevrons.
- One real-weight glyph pass, bounded derived-font cache, theme colors and
  clip/elision. Header input respects client bounds including NaN/zero-height.
  Inspector text explicitly restores the caller font on legacy Cairo.
- Real GroupBox enum/layout/search, light/dark and narrow raster regressions,
  pointer-boundary, font-cache/state, hidden/editor and selection controls.

## Frozen candidate

`bin/qa-artifacts/property-inspector-20261117/`: 2,845 files / 2,083 Go files.
Archive SHA-256 `7b38fd246c84f4da14eb0723469bbf0122cbb2e49cee8df6893849ea653db737`.
No Go source has changed since this freeze. Final result docs/media are reviewed
exceptions, not replacement source for the tests.

| Platform | Full passes/skips | Race passes/skips | Real strict bundled-gopls |
| --- | ---: | ---: | --- |
| macOS arm64 | 9,608 /29 | 9,608 /29 | 4 passed, no skips |
| Linux arm64, Xvfb | 9,547 /37 | 9,547 /37 | 4 passed, no skips |
| Windows 10 amd64 | 9,405 /43 | 9,405 /43 | 4 passed, no skips |

All three platforms exit 0, failures empty. Named counts include subtests; skips are
not passes. The renderer-tag GUI/property subset has 150 passes, no skips;
CGO remains enabled for the native GLFW host. Changed-package scoped vet passes.
Full vet retains exactly 501 unchanged baseline diagnostics, none new/removed;
it is not clean. Independent Mac, Linux and Windows post-test source exports
match all 2,845 manifest hashes, with no extra Go files. The Windows post-native
audit also preserves every source hash and confirms no extra Go files.

## Final native evidence

### macOS

macOS final profile `/private/tmp/silk-property-inspector-v2-20261117`, payload
SHA-256 `76bab453a24f75ee79532365e508ccf56237563c8cd2b8d4083d1aecd434acb4`.
The complete executable-relative bundled gopls connects in the actual IDE.

- Actual group properties show Layout / Appearance / Behavior / Events / General.
  Searching Chinese `行为` in English UI reveals Checkable, Checked and Locked.
  Collapsed Behavior is temporarily revealed by `Behavior`; clearing restores it
  without floating child editors.
- Edit title to `Inspector verified 20261117`, Save; independent model readback.
  Toolbar Undo/Save reads `Process options | Inspector`; Redo/Save reads the new
  title again. Checkable=true, Checked=false and typed event binding survive.
- New and the actual native Open panel select the exact **current** profile
  path. The AX-selected file URL and reopened title confirm the real target.
  Saved document and final session independently agree. Kqueue confirms normal
  exit 0 for PID 69130, not merely disappearance. Original README JPEGs are
  byte-identical to final native captures.
- The actual final IDE-generated decl companion is copied unchanged to an
  isolated module. Its race-instrumented constructor/runtime test verifies the
  newly saved title, group content ownership, nested child state/value 42 and
  disabled/restored input. `go list -m` resolves the exact frozen current SDK.
### Windows 10 amd64

Final IDE payload SHA-256
`fa08e15f3bfc40819274867bb3a3c31251d678dbf3165fdcb9a63b66c3c4b48f`.
The same `silk_property_inspector_20261117` job completes full/race/strict and
builds this payload; no test job is restarted. Raw archives are hash-verified.

- Actual English UI searches `行为` and `Behavior`, showing only Checkable,
  Checked and Locked. Original PNGs are visually reviewed; the collapsed and
  clear-restores-collapse captures are byte-identical, without floating editors.
- Native title edit/Save reads `Windows inspector verified`; Undo/Save reads
  `Inspector verified 20261117`; Redo/Save restores the new title. A separate
  current-SDK reader finishes each read before subsequent input. Group flags
  and `onOptionsToggled(checked bool)` remain intact.
- Toolbar New visibly installs an empty untitled scene. The observed current
  project file-tree row reopens the saved document; selected group, title,
  properties and adjacent bundled-gopls connection are visually verified.
- Held process handle confirms PID 4344 exits normally, 0. Independent final
  document/session reads match the current Windows profile; exact owned IDE
  and bundled-gopls processes remaining are zero.
- The actual newly generated companion, SHA-256
  `669bc6fff3b81b13641e606b43cb2481946f7ba143ac951c6d42d2f2d988b4f7`,
  is copied unchanged into a separate consumer. It executes a race-instrumented
  constructor/title/child state/input-restoration test, 0. Its module identity
  points explicitly to the same frozen SDK, not a previous fixture.
- Quiet small-fixture CPU sample: 10.00765s, IDE 0.0625 CPU seconds, **0.625% of
  one core**; both exact bundled-gopls processes use 0 CPU seconds. This measures
  process CPU only, not FPS/GPU/startup, all native menus/DPI/IME or large-workspace
  performance. No universal performance guarantee is implied.

Original final Windows PNG and two macOS JPEGs are used unchanged in README.
The final publication/index/remote audit remains the publication gate.

## Retained failures and harness corrections

1. Original tests reproduce misclassification, duplicate/out-of-band header ink,
   whitespace chevrons and outside-client activation. A separate candidate test
   catches the legacy Cairo caller-font leak; all failures remain in raw QA.
2. First test fixture used the wrong NewFont arity; compile failure retained.
   First isolated Mac full suite lacked its own Git index: the source inventory
   guard fails with zero tracked Go files. After that runner terminates with 1,
   only metadata is corrected; identical source repeats the full/race gates.
3. Initial Windows root creation succeeds but local stderr decoding fails before
   upload. A read confirms an empty owned root and no task; upload resumes once,
   rather than recreating or restarting an existing test job. Final task is
   `silk_property_inspector_20261117`; never restart on collector timeouts.
4. First Mac native harness omitted adjacent strict tools and mistakenly reopened
   the prior QA profile, because the native Go To path field retained that path.
   Program Log records the exact old path. Its old title is therefore NOT evidence
   of failed persistence in the new document. Original captures and wrong-path
   files are retained; that session exits normally, and the old QA fixture is
   restored byte-for-byte from its previously archived originals. No user project
   was used. Final native profile includes tools and explicitly replaces/verifies
   the actual Go To field and AX-selected URL before clicking Open.
5. One asynchronous helper build/read overlapped a later Undo, so its late read
   naturally sees the Undo value. Retain it as a harness timing error; final
   reads are completed and asserted before subsequent UI changes. Transient CUA
   pipe/AX/ScreenCaptureKit errors are retained, not called product crashes.

## Publication and continuing scope

All final suites, native workflows, actual generated consumers and source exports
pass. README/changelog/SDK/plan are refreshed with the actual results and original
media. Publication uses an exact indexed source/media audit, the user-authorized
commit/push and independent remote HEAD readback. It does not mark the broader
product goal complete. Raw evidence is under `bin/qa-artifacts/property-inspector-20261117/`.

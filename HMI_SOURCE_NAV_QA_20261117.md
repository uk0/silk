# 2D process source navigation

Baseline: `afd5688`. This increment connects the SilkIDE visual editor to the
exact current `.silkhmi` buffer, with no change to the persisted schema or runtime.

- At open, the decode worker validates the frozen buffer and indexes object and
  ID positions. A caret inside a top-level node or pipe preselects that item.
  Nested endpoint references are not independent targets.
- **Go to source** returns the selected item's ID only when exactly one item
  with a source location is selected and the visual draft equals its baseline.
  The host rechecks editor identity, open tab, revision and concurrent ownership
  before moving the caret and recording Back/Forward history. No save, schema
  reformat, text undo entry, point store or acquisition is triggered.
- LF/CRLF, compact JSON, Unicode, escaped IDs, kind-separated ID namespaces
  and duplicate field decoding are covered. A superseded array is not indexed;
  if the decoder inherits a missing ID from an older duplicate array, there is
  no exact ID token in the current object and the jump is not offered.

## Verification

- macOS arm64, isolated source: full and race suites each passed 9,706 named
  tests across 99 packages (29 skips, 0 failures). A separate strict run passed
  four real bundled-gopls tests without skips.
- Linux arm64/Xvfb: full and race suites each passed 9,645 named tests across
  99 packages (37 skips, 0 failures). The ordinary Docker start path left the
  test container in Created; a separate offline BuildKit run verified every
  frozen source path before and after executing both suites. An isolated
  Linux arm64 bundle was built from the pinned tool module, verified inside
  Linux, and passed the same four real-gopls tests without skips.
- Windows 10 amd64: full and race suites each passed 9,503 named tests across
  100 packages (43 skips, 0 failures). A separate strict run passed all four
  real bundled-gopls tests without skips. All 2,889 source file hashes
  matched the frozen manifest before and after the tests, with no extra Go
  source files left behind.
- Windows 10 native: verified all 2,889 frozen source paths before building.
  In an isolated Session 1 profile, **Tools → Edit 2D Process...** preselected
  `pumpA`. Clicking the pipe and **Go to source** jumped to `flowRoute`
  (line 29, column 14). Toolbar Back/Forward restored line 10/29 respectively.
  The reopened modal preselected the pipe; nudging a tank disabled the jump
  without closing the modal. Cancel preserved the file. After Alt+F4 the held
  process handle reported exit 0, the bundled gopls process was gone, and the
  persisted editor location was zero-based `line=28,column=13`. The fixture's
  SHA-256 matched the unmodified input.
- macOS native: used Quick Open on the isolated Unicode/CRLF fixture at
  line 10, column 14, then opened the visual editor via the command palette.
  The pump was preselected. Clicking the pipe and **Go to source** placed the
  caret at `flowRoute` on line 29, column 14. Reopening selected the pipe.
  Selecting the tank and nudging it disabled the jump; clicking the disabled
  button did not leave the modal. Cancel discarded the draft. Back restored
  line 10, column 14; Forward was independently read back from the saved
  session at zero-based `line=28,column=13`.
- Cmd+Q closed the QA process. Independent SHA-256 readback of the fixture
  matched its original bytes. The first attempt to launch under the repository
  stalled before a window appeared; the same binary worked from an isolated
  temporary directory, without changing system permissions.
- The 512-node decode/index benchmark was 2.83–2.87 ms/op on this M2 Max.
  This runs once off the UI thread on modal open; it is not a paint benchmark.
- The pure Go source-index tests also passed with `CGO_ENABLED=0`.

Reproduce the focused tests with:

```sh
go test -race -count=1 ./hmi ./gui ./cmd/silkide \
  -run '^(TestIndexedSource|TestProcessAssetEditor|TestProcessDocumentModal)'
```

Detailed logs, exact source identities and native disk/session readback:
`bin/qa-artifacts/process-source-nav.7fA0Bh/`. Frozen source archive SHA-256:
`3109e184da4baa8fcd5df5414cee2cdffd77b2a5a5b100930f247187d6044dc9`.
[Windows native pipe selection](docs/images/hmi-source-nav-windows.png).
Publication is separate from
the feature checks. This does not implement runtime-pane item navigation,
point/alarm navigation or complete Qt Creator parity.

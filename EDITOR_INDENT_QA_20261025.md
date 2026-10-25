# Editor indentation QA — 2026-10-25

## Changes and limits

- Tab/Enter/line commands share `InsertSpaces` and `TabSize`. The default remains
  tabs; soft tabs advance to the next logical tab stop, not a pixel/wcwidth stop.
- Line indent/outdent is one reversible prefix-edit transaction, preserving old
  history, selection direction, original bytes and distinct multi-caret lines.
  Selection ending at the next line's column zero excludes that line.
- Shift+Tab outdents even without a multiline selection. Active snippet navigation
  still takes precedence. Ordinary Tab replaces a single-line selection.
- Edit/Command Palette expose Indent Lines and Outdent Lines with existing icons.
  Appearance persists the input policy; applying it does not rewrite any document.
- Status-bar indentation is a separate, optional field. It never displaces the
  existing caret/count/encoding/language/operation fields in a narrow pane.
- No automatic whitespace normalization, per-file detection or `.editorconfig`.
  Pasted text, snippets and formatter output retain their own bytes; Go formatting
  still follows gofmt. Final text publication/layout remain UI-thread work.

## Reproduction and regression

Three tests first failed against the previous implementation (`red.log`):
indent lost its Undo entry, single-caret Shift+Tab inserted another Tab, and
selection ending at column zero incorrectly indented the excluded line.
New coverage also exercises widths 1/2/4/8/16, Unicode/invalid UTF-8, CRLF, reverse
selection, distinct caret lines, independent soft-tab stops, Enter, no-op outdent,
pre-existing Undo history, one Changed notification, preferences and command guards.
History tests ensure unchanged megabyte-sized line bodies are not copied into
line-indent undo entries. Example layout tests cover normal, narrow, tiny and
zero-sized parents.

The first two host full runs caught the status label replacing standalone `JSON`
with a combined label. The first Windows run used those earlier inputs and also
failed. Final wiring preserves standalone language text and uses a sixth optional
slot; tests cover widths 0–1199 and JSON drawing across themes/scales. Failed logs
are retained separately rather than reclassified as successful runs.

## macOS acceptance

Go 1.26.10, darwin/arm64, native Cairo, `GOMAXPROCS=2`:

```sh
go test -race -short -json -count=1 -p 2 \
  ./gui ./ged ./cmd/silkide ./settings ./examples/editor-indent \
  ./examples/editor-format ./paint ./purecairo
go vet -composites=false ./gui ./ged ./cmd/silkide ./settings \
  ./examples/editor-indent ./examples/editor-format ./paint ./purecairo
```

- Eight packages pass: **4,672 test/subtest pass events, 44 skips**. Extra
  graph/prop/settings regression: **238 passes, 1 skip** (settings overlaps).
  Short/platform/opt-in skips are not passes. Vet passes with composites disabled.
- Both native apps build. Tool-bundle verification passes; three separately
  required real bundled-gopls tests pass, zero skips: unsaved Unicode completion
  and formatting, renamed unsaved definitions, type/implementation/back navigation.
- Native IDE: actual Appearance dropdown selected Spaces:2, preferences read back,
  Ctrl/Cmd+A → Tab → Save, Undo/Redo → Save, Shift+Tab → Save, caret-only outdent,
  Edit-menu indent and Palette outdent. Eight saved stages were checked byte-exactly.
  Relaunch with the same isolated preferences verified soft Tab in another document.
- Native SDK example: **eight actual-widget JSON snapshots** verify policy-only
  changes, selected-line indent, Undo/Redo/outdent, two-caret soft Tab, whole-group
  Undo and switching back to Tabs:4 without rewriting text. Both apps exited.
  Initial SDK capture attempts hit ScreenCaptureKit error -3811; after closing
  the IDE and rebinding, screenshots and all eight interactions succeeded.

## Prefix microbenchmark

`BenchmarkNextLineIndentLongPrefix`, 8,192 leading spaces, Apple M2 Max,
`GOMAXPROCS=2`, one-second benchmark; baseline helper replayed via a Go overlay:

| Helper | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Previous repeated concatenation | 3,746,166 | 35,797,059 | 16,383 |
| Slice-based prefix scan | 13,348 | 0 | 0 |

This measures only the indentation helper, not entire key-to-paint latency,
frame rate, arbitrary-size documents or sustained application CPU.

## Evidence and scope

Local evidence: `bin/qa-artifacts/editor-indent-20261025/` (ignored test artifacts).
`accepted-source-hashes.json` freezes **1,569 build/source inputs**; all matched
again after native testing. Host gate, race JSONL, bundle tests, before/after
benchmarks, exact saved stages and SDK JSON snapshots are retained there.
Windows evidence is recorded separately below. This is a focused editor increment,
not a full-repository, Linux-native, IME or mixed-DPI certification.

## Windows 10 native acceptance

Windows 10 22H2, amd64, Go 1.26.10, UCRT64/Cairo, race detector:

- The same eight-package short/race gate passes: **4,557 test/subtest pass events,
  65 skips**, no failed tests. Three separately required bundled-gopls integration
  tests pass, zero skips. IDE and SDK example native executables build, and tool
  bundle verification passes.
- The immutable 1,801-file baseline plus **224 hash-verified overlay inputs** was
  used, not a stale deployment. `tested-overlay-hashes.json` matches local inputs.
- Native interactive desktop session (not SSH session 0), clean GUI PATH:
  Appearance dropdown → Spaces:2 → Apply → settings-file readback; whole-selection
  Tab/Undo/Redo/Shift+Tab; caret-only outdent/Undo; command-palette indent/outdent;
  actual Edit-menu screenshot; clean quit/relaunch and soft Tab in another file.
  **Nine saved stages** pass exact text comparison. Both menu entries/icons and
  the settings form are visible without overlapping fields in the captured size.
- A bounded post-restart responsiveness/CPU sample and screenshots are retained
  in `windows-ui-evidence.tar.gz`. It is not a frame-rate or long-duration benchmark.
  The two IDE instances exited normally; this round's four scheduled tasks were
  checked for success and removed, without touching unrelated applications.
- Windows SDK example is built and unit-tested here; its interactive button
  coverage comes from the macOS eight-snapshot run, not a Windows SDK UI run.

Remote artifacts: `C:\silk\qa-editor-indent-20261025-codex\`.
Local copies: `windows-accepted-evidence.tar.gz`, `windows-ui-evidence.tar.gz`.

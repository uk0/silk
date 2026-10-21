# PDF feedback: editor rendering, appearance and LSP

Development worktree based on `3c15c0722447fbaffa86b11bd5a802724cdabb1a`.
Not a release, and no commit/push is implied by this report.

## User report and reproduction

Read the complete one-page `silk使用问题发现(1).pdf`, including its extracted
1908 × 990 screenshot. The report describes Windows 10/11 source-display
corruption, fuzzy/aliased editor and side panels, language selection, and
customizable fonts/colors.

The screenshot's repeated boxes occur at tab indentation. Before the change,
`TestEditorTabsUseStopsWithoutDrawingControlGlyphs` failed: syntax token runs
passed literal tabs (also inside comments/raw strings) to Cairo, and a tab was
measured as 7.82666015625 px instead of the four-space 31.306640625 px on the
local test font. This was a display/layout defect, not evidence of damaged file
encoding. The tooltip also rendered a multiline LSP response as one glyph run.

## Changes

- Shared tab-stop geometry for token painting, caret/selection and mouse hit
  testing; tab width 1–16, default 4. Preserve source bytes. Binary-search hit
  testing avoids the old quadratic prefix measurement loop on long lines.
- Bounded multiline tooltip layout with tab expansion and code-fence removal.
- Win32 system DPI awareness initialized before class/window creation, proper
  desktop DC DPI query/release, grayscale AA plus full font/grid hinting.
- Tools → Appearance & Editor and the command palette expose persisted language,
  dark/light theme, UI/editor fonts and sizes, tab width, accent and five editor
  colors. Colors accept #RRGGBB; empty means theme default. Fonts/colors apply
  live; language applies after restart, with explicit environment override.
  IDE-owned dock captions no longer mix fixed Chinese names into English mode.
- One bounded latest-snapshot LSP worker per document. Ordered open/change/close,
  90 ms debounce, cancellation, two-second completion timeout, generation and
  document/caret guards, empty-result clearing, explicit Ctrl+Space provider
  request, flush-before-command, UTF-16 completion edits plus auto-imports in one
  undo entry, properly escaped file URIs, and startup/disconnect retirement.
- Request cancellation emits `$/cancelRequest`; Close has a pipe-closing watchdog
  so shutdown cannot wait forever when the peer stops reading stdin.
- IDE advertises plain-text completion and hover. Framework protocol decoding
  remains broader, but the IDE does not pretend to implement snippet tab stops.
- Semantic completion candidates precede local identifier guesses. A regression
  test reproduced `Pr` outranking `Println` and unrelated keywords appearing
  before member completions after a dot; both now select provider edits first.
- Native desktop testing caught Ctrl+Space producing an extra printable WM_CHAR
  space, which dismissed completion. Suppress that shortcut's space without
  blocking AltGr text; native key-state/message tests exercise both cases.
- Add host-owned dock captions because many tool views have no SetTitle method.
  The override follows redocking and does not alter shared SDK registry names.
- Keep LSP error/warning/info/hint severities distinct. Improvement hints no
  longer produce red editor error markers or count as build failures.
- Completion transactions restore the final caret on redo, including when
  auto-imports move it to a different line. Changed callbacks observe the final
  caret rather than issuing the next request at the old position.
- Clip completion popups, use opaque backgrounds, and elide signatures into a
  separate remaining-width column. Native Windows testing reproduced CJK
  fallback-font advance exceeding ink width at 40 px; budget both extents and
  binary-search rune-safe prefixes instead of measuring every prefix.

## Local verification

Commands (Go 1.26.5, macOS arm64, native Cairo):

```sh
# gopls v0.22.0 installed into the ignored QA tools directory for these tests
PATH="$PWD/bin/qa-artifacts/pdf-editor-lsp-20261020/tools:$PATH" \
  go test -race -count=1 ./core ./paint ./gui ./ged ./cmd/silkide
PATH="$PWD/bin/qa-artifacts/pdf-editor-lsp-20261020/tools:$PATH" \
  go test -race -count=20 -run 'TestLSPDocument|TestLSPRequestCancellation|TestLSPCloseUnblocks' ./cmd/silkide ./core
go vet -composites=false ./core ./paint ./gui ./cmd/silkide
CGO_ENABLED=0 go test -count=1 -tags silk_pure_go ./paint ./core
go build -o bin/qa-artifacts/pdf-editor-lsp-20261020/silkide ./cmd/silkide
```

All listed local regression groups passed. The real gopls test uses an isolated
module, unsaved content, an emoji before the completion position and a filename
containing Unicode, spaces, `#` and `%`. It gets `Println` with a real UTF-16
textEdit, applies it, requests formatting of the unsaved buffer and verifies undo.
It is not just a mock server test, nor does formatting prove type correctness.

A deterministic 1,000-edit burst yields one completion and the latest version;
20 race-instrumented scheduler/cancellation runs passed. New tests also cover
close/reopen ordering, malformed appearance settings, on-disk setting readback,
per-editor palette isolation, tooltip bounds, and tab caret/hit-test round trips.
After the completion-ranking correction, the native `gui` and `cmd/silkide`
race suites were run again, including the real gopls test, and passed.

M2 Max local microbenchmarks: printable monospace measurement **38.85 ns/op,
0 B/op, 0 allocs/op**; representative editor token-width loop **384.7 ns/op,
0 B/op, 0 allocs/op**. These are not whole-application input-latency numbers.

## Windows native verification

Native Windows 10 22H2 (19045), Go 1.26.5/UCRT64 Cairo. The initial five-package
`-race -short -count=1` gate passed, followed by a separately executed real gopls
integration test (not skipped). After the ranking fix, the complete `gui` and
`cmd/silkide` race suites passed again. After the dock/severity/native-shortcut
changes, the complete `gui`, `ged` and `cmd/silkide` race suites passed; the caret
follow-up passed complete `gui`/`cmd/silkide` suites. The final popup follow-up
passed focused native race tests for completion layout/ranking, appearance,
Ctrl+Space and completion application (6.902 s / 5.111 s), then built the IDE.
Final macOS five-package race tests also passed after all code changes.

Native Windows commands (with Go, UCRT64 and the isolated gopls on PATH):

```powershell
go test -race -p 1 -short -count=1 ./core ./paint ./gui ./ged ./cmd/silkide
go test -race -count=1 -run TestGoplsUnsavedUnicodeCompletionAndFormatting -v ./cmd/silkide
go test -race -count=1 ./gui ./ged ./cmd/silkide
go test -race -count=1 -run "TestProviderCompletion|TestCompletionRow|TestAppearance|TestWindowsCtrlSpace|TestLSPCompletion" ./gui ./cmd/silkide
go build -ldflags '-H windowsgui' -o qa-bin/silkide.exe ./cmd/silkide
```

The source manifest plus four incremental manifests cover 1,557 inputs. Go
source hashes were compared with the delivery worktree: no mismatches. Docs and
screenshots were updated afterward. Final tested native executable SHA-256:
`b85bb27dffaa98b5053f7db003acf751ccfab668f00820d1040a827166373ebf`.

The first interactive pass verified a real Session 1 desktop, system DPI
awareness (`DPI=96`, `AWARENESS=1`), and exact tab/Chinese/emoji save readback.
It also changed UI size to 16, editor size to 18, background to #17202B, keywords
to #80CBC4 and comments to #8FA3B8 through the actual dialog, then read those
values and locale `zh-CN` back from the isolated settings file. Screenshot
inspection caught the dock-caption and hint-severity issues above. The first
completion replay failed (an inserted shortcut space); this is preserved as a
failed run, not described as a pass. The first app exit cleanly closed its
gopls child.

The first isolated source upload verified only the code-file subset and omitted
`go.mod` and other build inputs. Go consequently resolved the parent checkout's
module and failed with missing packages / old Cairo glyph definitions. That
failed log is retained. The corrected upload includes all tracked build inputs,
verifies their hashes, and checks `go env GOMOD` points inside the QA directory
before retrying. Missing-input failures are not presented as passing tests.

## Final desktop replay (2026-10-21 PDT)

The final executable was run through interactive Scheduled Tasks, not SSH
Session 0, in an isolated workspace/settings directory with a minimal Windows
PATH. The IDE located Go and the QA gopls from the configured GOPATH itself.
Both desktop task results were checked separately from build/test exit codes.

1. Restart with saved Chinese locale and custom UI/editor fonts/colors. Open a
   Go file containing real tabs, Chinese comments and an emoji; save and compare
   the entire file against the input. Exact text round trip passed; screenshot
   shows aligned indentation without tab tofu. Window reports `DPI=96`,
   `AWARENESS=1`.
2. Run the LSP restart command from the palette. Confirm the old child exited
   and exactly one replacement gopls child started.
3. In a Go main function with `import "fmt"`, type `fmt.Printl`, press real
   Ctrl+Space and Enter, then save. Readback is `fmt.Println` with no injected
   space. One undo restores `fmt.Printl`. Redo followed by pasting `("QA")`
   produces `fmt.Println("QA")` at the correct caret. Each stage was saved and
   independently read back, not inferred from a screenshot.
4. Inspect completion labels/details, open the appearance dialog through the
   palette, and check Chinese labels including Apply. Close the dialog and
   sample idle IDE CPU for 8 seconds: **0.976% of one logical core**. This is a
   small-workspace, bounded parent-process sample, not whole-machine CPU,
   gopls CPU, large-project latency or a hardware-independent guarantee.
5. Exit and confirm the gopls child closes. Restart the same executable with
   `SILKIDE_LOCALE=en`; inspect English menus/dock captions/appearance dialog
   and real multiline `fmt.Println` hover. Confirm saved locale remains `zh-CN`
   (environment override does not rewrite preferences). Exit and verify no
   owned gopls survives.

All replay assertions passed. Screenshots below were opened and visually
inspected, including the previously failing overlapping completion details.
The tests cover native desktop input and persistence, not only offscreen widgets.

- [Chinese editor, tabs and Unicode](screenshots/silkide-editor-lsp-windows.png)
- [Completion columns and elision](screenshots/silkide-completion-windows.png)
- [Chinese appearance settings](screenshots/silkide-appearance-windows.png)
- [English appearance settings](screenshots/silkide-appearance-windows-en.png)
- [English workbench and actual gopls hover](screenshots/silkide-hover-windows.png)

## Remaining scope boundaries

- System-DPI aware is not per-monitor-v2: different-DPI monitor transitions still
  need coordinated layout/input/IME scaling. Win11 is not certified by Win10 QA.
- No Linux/macOS native desktop QA in this iteration; local builds, offscreen
  widget tests and pure-Go tests are separate evidence.
- Language applies on restart; SDK-specific property labels and domain content
  are not an exhaustive translation of every framework widget.
- LSP completion snippets are intentionally disabled; code-action resource
  operations, all server features and all language servers are not certified.
- Cancellation is cooperative with the server; no hard real-time latency or
  all-hardware sharpness guarantee is claimed.

Evidence directory (ignored): `bin/qa-artifacts/pdf-editor-lsp-20261020/`.
No unrelated `gen_image/`, root `silkide` binary or restricted `local/` changes.

References: [Microsoft DPI initialization rules](https://learn.microsoft.com/en-us/windows/win32/hidpi/setting-the-default-dpi-awareness-for-a-process)
and [LSP specification](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/).

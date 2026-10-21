# Code editor position coordinates

`CodeEditor` stores **zero-based lines and Unicode code-point (rune) columns**.
A rune column is not a UTF-8 byte offset, UTF-16 code-unit offset or visual cell
column. Tabs and combining marks do not change the logical coordinate contract.

| Interface | Line | Column |
| --- | --- | --- |
| `CursorLine()` / `CursorCol()` | zero-based | rune |
| `ScrollToLineCol(line, column)` | zero-based | rune |
| `CursorUTF16Col()` | current line | UTF-16 units |
| `ScrollToUTF16Position(line, column)` | zero-based | UTF-16 units |
| `RuneColumnFromUTF16(lineText, column)` | one supplied line | UTF-16 input, rune output |

```go
// LSP locations use zero-based lines and UTF-16 columns by default.
// Invoke from the UI thread, after opening the destination document.
editor.ScrollToUTF16Position(location.Range.Start.Line,
    location.Range.Start.Character)
```

The conversion uses the **live editor buffer**, including unsaved changes.
Do not convert using disk text and then apply the column to an unsaved buffer.
Do not trim indentation before converting a column; trim only display previews.
A server negotiated to another position encoding needs its own adapter.

Negative coordinates clamp to the beginning, large coordinates to the last
line/end of line. UTF-16 offsets inside a surrogate pair round down to its rune.
`RuneColumnFromUTF16` has no allocations and accepts invalid UTF-8 using Go's
replacement-rune behavior. It is a pure function safe off-thread; editor methods
remain UI-thread-only. Navigation is not a buffer edit and does not mark a saved
document dirty or discard unsaved content.

For example, in `a😀Target`, the `T` is at UTF-16 column **3**, rune column **2**,
and UTF-8 byte offset **5**. Compiler/`go/token` positions generally use 1-based
UTF-8 byte columns and require a different conversion.

Tests: `TestRuneColumnFromUTF16`, `TestCodeEditorScrollToUTF16Position`, and the
IDE's `TestLSPNavigation*` / `TestLSPReferenceAndDiagnosticColumns`. These are
model/native-painter regression tests, not substitutes for desktop UI replay.

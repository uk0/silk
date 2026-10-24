# Editor line and position access

All editor methods are UI-thread-only. Background workers must receive immutable
snapshots; never call these accessors directly from a worker.

```go
line, ok := editor.LineText(12) // zero-based; empty valid lines return "", true
position, ok := editor.CursorTextPosition()
if ok {
    // One prefix traversal, no document split or whole-line rune allocation.
    lspLine, lspColumn := position.Line, position.UTF16Column
    _, _ = lspLine, lspColumn
    if position.HasPreviousRune {
        precedingCodePoint := position.PreviousRune
        _ = precedingCodePoint
    }
}
position, ok = editor.TextPositionAt(12, 7) // input column is in Unicode runes
```

`EditorTextPosition` explicitly distinguishes `RuneColumn`, `ByteColumn` and
`UTF16Column`. Columns are zero-based. Rune columns count Unicode code points,
not visual cells or grapheme clusters: combining marks remain separate, and an
astral code point such as an emoji uses two UTF-16 units. Tabs and NUL each count
as one rune/unit. Invalid UTF-8 bytes are read as U+FFFD, matching Go's range.
`HasPreviousRune` distinguishes the beginning of a line from a preceding NUL.

Invalid line indices return `ok=false`; negative columns clamp to zero, oversized
columns to the actual line end. Reads do not move the caret, emit changes, build
full text or publish revisions, even when a low-level edit is pending. `LineText`
returns the current immutable line without copying the line slice; old strings
remain valid after later edits. Such a string can retain the original source
storage; use `strings.Clone` if a small line must outlive a large document.

`CursorUTF16Col` and `UTF16ColumnAt` use the same allocation-free prefix traversal.
The inverse remains `RuneColumnFromUTF16`, which rounds an interior surrogate
unit down to the code point's beginning. Complexity is O(prefix bytes): asking
for the end of a huge line still scans that prefix; this is not a line-position
index or a promise of constant-time arbitrary lookups.

The IDE uses the combined snapshot for automatic LSP completion policy and uses
byte offsets to inspect fallback completion prefixes backwards. Full-text
notification contents, monotonic versions, coalescing and worker-side wire order
are unchanged. If notification text differs from the editor snapshot, text is
still synchronized but a completion is not requested using unrelated coordinates.
Explicit server text edits and additional edits still apply atomically.

Invalid-byte reading uses replacement code points without rewriting source bytes.
The editor's standard input/history paths now also preserve original bytes; see
[edit semantics and multi-caret transactions](CODE_EDITOR_EDITS.md). This does not
certify arbitrary formatter/provider transformations or binary clipboard transport.

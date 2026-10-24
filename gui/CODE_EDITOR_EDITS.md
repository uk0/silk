# Byte-preserving edits and multi-caret history

CodeEditor keeps immutable source strings. Storage edits splice **bytes**, while
public caret/selection columns continue to count Unicode code points, not byte
indices, UTF-16 units, visual cells or grapheme clusters. Use the
[position APIs](CODE_EDITOR_POSITIONS.md) to convert explicitly.

Typing, pairing/wrapping, backspace, forward delete, newline, selection replacement,
clipboard paste and find/replace preserve untouched bytes. Invalid UTF-8 displays
one replacement glyph per invalid byte, and NUL displays as U+2400 (or the
font's missing-glyph box). Paint measurement and drawing use the same display-only
conversion; neither malformed UTF-8 nor an embedded NUL can blank/truncate the
rest of the line. Source bytes are not rewritten merely because another character on the
line was edited. A splice can join invalid byte sequences into a valid character;
history retains the original byte anchor and exact bytes, not a reinterpreted rune
count. A caret endpoint inside a newly formed character rounds forward.

Native input/clipboard and find operations record undo. `Undo()` and `Redo()` are
also available to menus/toolbars. Lower-level `ReplaceSelection`/`DeleteSelection`
retain their existing host-managed-history semantics; they do not automatically
create a separate user undo action. `ReplaceAllTextAt` is the atomic whole-buffer
replacement API for providers. All mutations/accessors remain **UI-thread-only**.

`AddCursorAtLine` creates an additional caret. The native Windows route also
delivers Ctrl+Alt+Up/Down, including key repeat, to the focused editor through
`IEventSystemKeyDown`. Controls must return false for unhandled system keys;
Alt+F4 and Alt+Space retain the native window behavior. If the desktop intercepts
Ctrl+Alt+Up/Down, the existing Ctrl+Alt+Shift+Up/Down variant uses the same action.
This variant is the real Windows QA machine's verified input route; the original
chord was observed not to deliver its arrow key to the application. With additional carets active,
typing (including multi-line text), paste, Backspace, Delete and Enter are each a
single transaction. Enter computes indentation independently at each caret. Undo
and redo restore primary and secondary caret positions; they do not just revert
the primary edit. Escape exits multi-caret mode. This is not a per-caret selection
model, and it does not provide arbitrary concurrent editing from worker threads.

## Discoverable caret commands

`CursorCount`, `CanAddCursorAbove`, `CanAddCursorBelow`, `AddCursorAbove`,
`AddCursorBelow` and `KeepPrimaryCursor` expose the same UI-thread-only commands
used by the IDE Edit menu, command palette and SDK context menu. Successful adds
retain the primary caret, clear its selection, dismiss transient input overlays,
unfold and reveal the new caret, and return focus to the editor. They do not edit
text, publish a document revision or append an undo entry. Boundary checks are
pure; adding past the first/last line returns false without changing state.

The next physical line is relative to the extreme existing caret. Columns count
runes and clamp on shorter lines; repeated addition from a short/empty line uses
that clamped column. `KeepPrimaryCursor` reveals the original primary caret and
retains its selection. `ClearAdditionalCursors` remains the lower-level operation
without these command focus/reveal side effects. The status bar includes the
active caret count. Caret commands do not provide multi-selection editing.

Caret clamping and visibility measure only the requested byte prefix rather than
copying the entire line into a rune slice. This is a narrow allocation improvement,
not a claim that full long-line rendering, hit testing or edits are constant time.

## Viewport rendering and hit testing

Caret/selection drawing, click hit testing and textual bracket matching preserve
rune coordinates without copying a complete line to `[]rune`. Hit testing first
brackets the clicked prefix; backward bracket matching is a linear byte traversal,
not a repeated whole-line conversion. Bracket matching retains its textual
semantics: it is not a Go parser and does not skip brackets in strings/comments.

Stable token-cache reads compare immutable source/state/semantic keys rather than
rehashing the full line. Fixed-cell ASCII text is submitted only for the visible
horizontal span, with font-measured ink overscan. Tabs retain their original line
origin. Every ASCII glyph is checked: font exceptions such as Menlo `#` receive
their actual advance and disqualify the affected line from fixed-cell clipping.
Unicode, malformed bytes and proportional fonts keep the existing shaping path;
this change does not claim bounded work for every long Unicode/proportional run.
Error waves and indentation guides emit only visible decorations.
Offscreen bracket outlines are culled before native rasterization, because the
backend clip alone cannot prevent huge path-coordinate overflow. Selection,
find/occurrence and caret fills retain a bounded guard band, preserving fractional
clip-edge ink; hover underlines retain pen overscan. Outline rectangles are never
resized into false viewport-edge borders. Pure-Go fill/stroke masks use clip-local
device coordinates while source sampling remains in surface coordinates.

The transaction stores byte edit ranges and small deleted strings instead of two
full-document snapshots per multi-caret keystroke. All anchors are planned against
the original model, applied right-to-left, and reversed left-to-right. Rendering,
revision publication, LSP notification and outline consumers see the final state,
not a transient deletion in a selection replacement. Existing bounded undo-entry
count does not establish a hard byte-memory limit for huge pastes.

Do not treat these guarantees as a generic binary editor or an encoding converter.
Clipboard/OS input is Unicode; files can still be changed by formatters, code
providers or other transformations. No UTF-8 normalization or automatic repair
is performed by the byte-splice primitive. See
[regression scope and remaining limits](../EDITOR_BYTE_EDITS_QA_20261024.md).

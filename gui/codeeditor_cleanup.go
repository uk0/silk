package gui

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// ErrEditorCleanupCR prevents a cleanup from reinterpreting content CR as the
// CR in a CRLF separator. The operation is rejected before any text is changed.
var ErrEditorCleanupCR = errors.New("cleanup would turn a standalone CR into a CRLF separator")

// trailingWhitespacePlan stores only changed ASCII suffixes, not whole lines or
// document copies. LF and CRLF identity, lone CR, malformed UTF-8 and non-ASCII
// whitespace remain untouched. A clean document takes no allocations here.
func trailingWhitespacePlan(lines []string) ([]editorByteEdit, error) {
	var edits []editorByteEdit
	for row := len(lines) - 1; row >= 0; row-- {
		s := lines[row]
		end := len(s)
		crlf := row < len(lines)-1 && strings.HasSuffix(s, "\r")
		if crlf {
			end--
		}
		start := end
		for start > 0 && (s[start-1] == ' ' || s[start-1] == '\t') {
			start--
		}
		if start == end {
			continue
		}
		if row < len(lines)-1 && !crlf && start > 0 && s[start-1] == '\r' {
			return nil, ErrEditorCleanupCR
		}
		edits = append(edits, editorByteEdit{line: row, column: start, before: strings.Clone(s[start:end])})
	}
	return edits, nil
}

// trimTrailingInText is the pure adapter retained for existing users/tests of
// the internal transformation. Unsafe CR ambiguity leaves the input unchanged.
func trimTrailingInText(text string) string {
	lines := strings.Split(text, "\n")
	edits, err := trailingWhitespacePlan(lines)
	if err != nil || len(edits) == 0 {
		return text
	}
	for _, edit := range edits {
		s := lines[edit.line]
		lines[edit.line] = s[:edit.column] + s[edit.column+len(edit.before):]
	}
	return strings.Join(lines, "\n")
}

// TrimTrailingWhitespace preserves its original no-result SDK signature.
// Use TrimTrailingWhitespaceChecked when the host needs an error for readonly
// input or ambiguous lone CR. Both methods are UI-thread-only and atomic.
func (e *CodeEditor) TrimTrailingWhitespace() { _ = e.TrimTrailingWhitespaceChecked() }

// TrimTrailingWhitespaceChecked removes ASCII spaces/tabs before each existing
// LF/CRLF or EOF. It preserves selection direction, carets, bookmarks, source
// bytes and separator identities outside those suffixes. One Undo restores the
// exact original text and anchors. No-op/error does not change history/revision.
func (e *CodeEditor) TrimTrailingWhitespaceChecked() error {
	if e.readOnly {
		return ErrEditorReadOnly
	}
	edits, err := trailingWhitespacePlan(e.lines)
	if err != nil || len(edits) == 0 {
		return err
	}
	before := append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
	selection := e.selectionSnapshot()
	// Only anchor rows need a rune count; unchanged rows/long bodies are not
	// converted to []rune just to trim a suffix elsewhere in the document.
	type suffix struct{ start, count int }
	limits := make(map[int]suffix, len(before)+2)
	for _, p := range before {
		limits[p.line] = suffix{}
	}
	if selection.active {
		limits[selection.start.line] = suffix{}
		limits[selection.end.line] = suffix{}
	}
	for _, edit := range edits {
		if _, needed := limits[edit.line]; needed {
			limits[edit.line] = suffix{utf8.RuneCountInString(e.lines[edit.line][:edit.column]), len(edit.before)}
		}
	}
	e.prepareCaretCommand()
	e.CancelPendingPaste()
	for _, edit := range edits {
		e.replaceByteRange(edit.line, edit.column, len(edit.before), "")
	}
	shift := func(p cursorPos) cursorPos {
		if limit := limits[p.line]; limit.count > 0 && p.col > limit.start {
			p.col = max(limit.start, p.col-limit.count)
		}
		return p
	}
	after := make([]cursorPos, len(before))
	for i, p := range before {
		after[i] = shift(p)
	}
	after = dedupCursorsList(after)
	next := selection
	if next.active {
		next.start, next.end = shift(next.start), shift(next.end)
	}
	e.restoreEditCursors(after)
	e.restoreSelection(next)
	e.pushUndo(editAction{kind: 4, group: edits, beforeCursors: before, afterCursors: after, selection: &editorSelectionHistory{selection, next}})
	e.rebuildText()
	e.ensureCursorVisible()
	e.Update()
	return nil
}

// HasFinalNewline reports whether the nonempty buffer ends in LF (including
// CRLF). Empty buffers return false. This storage query does not copy/scan text.
func (e *CodeEditor) HasFinalNewline() bool {
	return len(e.lines) > 1 && e.lines[len(e.lines)-1] == ""
}

// EnsureFinalNewline appends one separator using the current Enter preference
// only if nonempty text lacks a final newline. It never trims extra blank lines
// or moves the selection/carets to EOF. The insertion is a single Undo step.
// Appending LF after a standalone CR is rejected; explicitly selecting CRLF
// first lets the original CR remain content. No automatic save cleanup occurs.
func (e *CodeEditor) EnsureFinalNewline() error {
	if e.readOnly {
		return ErrEditorReadOnly
	}
	if len(e.lines) == 0 || len(e.lines) == 1 && e.lines[0] == "" || e.HasFinalNewline() {
		return nil
	}
	row := len(e.lines) - 1
	s := e.lines[row]
	newline := "\n"
	if e.preferredLineEnding == LineEndingCRLF {
		newline = "\r\n"
	} else if strings.HasSuffix(s, "\r") {
		return ErrEditorCleanupCR
	}
	before := append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
	selection := e.selectionSnapshot()
	e.prepareCaretCommand()
	e.CancelPendingPaste()
	edit := editorByteEdit{line: row, column: len(s), after: newline}
	e.replaceByteRange(edit.line, edit.column, 0, newline)
	// Deliberately left-affine anchors: a caret at the old EOF stays before the
	// appended separator, while the new empty sentinel is an unmarked row.
	e.restoreEditCursors(before)
	e.restoreSelection(selection)
	e.pushUndo(editAction{kind: 4, group: []editorByteEdit{edit}, beforeCursors: before, afterCursors: before, selection: &editorSelectionHistory{selection, selection}})
	e.rebuildText()
	e.ensureCursorVisible()
	e.Update()
	return nil
}

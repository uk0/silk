package gui

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// EditorLineEnding controls newly typed Enter and explicit document conversion.
// Pasted text, formatter output and host SetText are never silently normalized.
type EditorLineEnding uint8

const (
	LineEndingLF EditorLineEnding = iota
	LineEndingCRLF
)

func (s EditorLineEnding) String() string {
	switch s {
	case LineEndingLF:
		return "LF"
	case LineEndingCRLF:
		return "CRLF"
	}
	return "Unknown"
}

var ErrEditorLineEnding = errors.New("unsupported editor line ending")
var ErrEditorAmbiguousCR = errors.New("cannot convert CRCRLF to LF without making a standalone CR ambiguous")

// EditorLineEndingInfo counts LF separators (excluding CRLF) and CRLF pairs.
// A standalone CR remains content, not a logical line separator. Preferred is
// used for Enter, including empty/single-line documents. Access is UI-thread-only.
type EditorLineEndingInfo struct {
	LF, CRLF  int
	Preferred EditorLineEnding
}

func (s EditorLineEndingInfo) String() string {
	if s.LF > 0 && s.CRLF > 0 {
		return "Mixed"
	}
	if s.CRLF > 0 {
		return "CRLF"
	}
	if s.LF > 0 {
		return "LF"
	}
	return s.Preferred.String()
}
func (e *CodeEditor) LineEndings() EditorLineEndingInfo {
	e.Text()
	return EditorLineEndingInfo{e.lineEndingLF, e.lineEndingCRLF, e.preferredLineEnding}
}
func (e *CodeEditor) cacheLineEndings(text string) {
	e.lineEndingCRLF = strings.Count(text, "\r\n")
	e.lineEndingLF = strings.Count(text, "\n") - e.lineEndingCRLF
	e.ambiguousLineEndingCR = strings.Contains(text, "\r\r\n")
}
func (e *CodeEditor) detectPreferredLineEnding() {
	e.preferredLineEnding = LineEndingLF
	if e.lineEndingCRLF > e.lineEndingLF {
		e.preferredLineEnding = LineEndingCRLF
	} else if e.lineEndingCRLF > 0 && e.lineEndingCRLF == e.lineEndingLF {
		if i := strings.IndexByte(e.text, '\n'); i > 0 && e.text[i-1] == '\r' {
			e.preferredLineEnding = LineEndingCRLF
		}
	}
}
func (e *CodeEditor) CanConvertLineEndings(style EditorLineEnding) bool {
	if e.readOnly || style > LineEndingCRLF {
		return false
	}
	e.Text()
	if style == LineEndingLF && e.ambiguousLineEndingCR {
		return false
	}
	return style != e.preferredLineEnding || style == LineEndingLF && e.lineEndingCRLF > 0 || style == LineEndingCRLF && e.lineEndingLF > 0
}

// History retains one bit per separator, not the document or one copy per line.
// Row bodies and number of rows do not change, so folds retain their positions.
type editorLineEndingHistory struct {
	original      []uint64
	before, after EditorLineEnding
	changesText   bool
}

func (h *editorLineEndingHistory) wasCRLF(row int) bool {
	return h.original[row/64]&(uint64(1)<<uint(row%64)) != 0
}

// ConvertLineEndings atomically normalizes LF/CRLF and sets the Enter preference.
// One Undo restores mixed endings, preference, caret(s) and selection direction.
// EOF termination and all other bytes are preserved. A CRCRLF -> LF conversion
// is rejected before mutation: otherwise its preserved standalone CR would
// become another CRLF, and repeated conversion could silently delete content.
// In a document without separators only the preference changes; it is undoable
// but does not change text revision/dirty state or emit a text-change event.
func (e *CodeEditor) ConvertLineEndings(style EditorLineEnding) error {
	if e.readOnly {
		return ErrEditorReadOnly
	}
	if style > LineEndingCRLF {
		return ErrEditorLineEnding
	}
	e.Text()
	if style == LineEndingLF && e.ambiguousLineEndingCR {
		return ErrEditorAmbiguousCR
	}
	if !e.CanConvertLineEndings(style) {
		return nil
	}
	h := &editorLineEndingHistory{before: e.preferredLineEnding, after: style,
		changesText: style == LineEndingLF && e.lineEndingCRLF > 0 || style == LineEndingCRLF && e.lineEndingLF > 0}
	if h.changesText {
		h.original = make([]uint64, (len(e.lines)-1+63)/64)
		for i, s := range e.lines[:len(e.lines)-1] {
			if strings.HasSuffix(s, "\r") {
				h.original[i/64] |= uint64(1) << uint(i%64)
			}
		}
	}
	before := append([]cursorPos{{e.cursorLine, e.cursorCol}}, e.additionalCursors...)
	selection := e.selectionSnapshot()
	e.prepareCaretCommand()
	e.CancelPendingPaste()
	e.applyLineEndingHistory(h, false)
	after := append([]cursorPos(nil), before...)
	for i, p := range after {
		after[i] = e.clampMovedPosition(p)
	}
	next := selection
	if next.active {
		next.start = e.clampMovedPosition(next.start)
		next.end = e.clampMovedPosition(next.end)
	}
	e.restoreEditCursors(after)
	e.restoreSelection(next)
	e.pushUndo(editAction{kind: 7, lineEndings: h, beforeCursors: before, afterCursors: after, selection: &editorSelectionHistory{selection, next}})
	e.finishLineEndingChange(h)
	return nil
}
func (e *CodeEditor) applyLineEndingHistory(h *editorLineEndingHistory, reverse bool) {
	e.bookmarkState.mapped = true
	e.preferredLineEnding = h.after
	if reverse {
		e.preferredLineEnding = h.before
	}
	if !h.changesText {
		return
	}
	for i, s := range e.lines[:len(e.lines)-1] {
		crlf := h.after == LineEndingCRLF
		if reverse {
			crlf = h.wasCRLF(i)
		}
		current := strings.HasSuffix(s, "\r")
		if current && !crlf {
			e.lines[i] = s[:len(s)-1]
		}
		if !current && crlf {
			e.lines[i] = s + "\r"
		}
	}
	e.textDirty = true
	e.clearTokenCache()
}
func (e *CodeEditor) finishLineEndingChange(h *editorLineEndingHistory) {
	if !h.changesText {
		e.publishBookmarks(false)
	}
	if h.changesText {
		e.rebuildText()
	} else {
		e.notifyHistoryChanged()
		e.notifyLineEndingsChanged()
	}
	e.ensureCursorVisible()
	e.Update()
}
func (e *CodeEditor) newlineWithIndent(line string, col int) string {
	text := editorNewlineWithIndent(line, col, e.indentUnit(), e.TabSize())
	if e.preferredLineEnding == LineEndingCRLF {
		return "\r" + text
	}
	return text
}

func (e *CodeEditor) lineHasCRLF(row int) bool {
	return row >= 0 && row < len(e.lines)-1 && strings.HasSuffix(e.lines[row], "\r")
}

func (e *CodeEditor) crlfBackspaceBoundary() bool {
	return e.cursorCol == 0 && e.lineHasCRLF(e.cursorLine-1) || e.lineHasCRLF(e.cursorLine) && e.cursorCol >= utf8.RuneCountInString(e.lines[e.cursorLine])
}
func (e *CodeEditor) crlfDeleteBoundary() bool {
	return e.lineHasCRLF(e.cursorLine) && e.cursorCol >= utf8.RuneCountInString(e.lines[e.cursorLine])-1
}

package gui

import "unicode/utf8"

// Edit history stores changed spans, not full buffers. Both the entry count and
// retained UTF-8 payload are bounded; an oversized edit clears history rather
// than leaving a gap through which an older command could corrupt new text.
const editHistoryLimit = 256
const editHistoryBytes = 1 << 20

type editDelta struct {
	start                     int
	before, after             string
	anchor, caret             int
	end                       int
	prefixBefore, prefixAfter int
}
type editHistory struct {
	entries         []editDelta
	position, bytes int
}

// ClearUndoHistory drops all retained text. SetText and constraint changes call
// this automatically. Like all Edit APIs, these methods are UI-thread-only.
func (e *Edit) ClearUndoHistory() { e.history = editHistory{} }
func (e *Edit) CanUndo() bool     { return e.IsEnabled() && !e.readonly && e.history.position > 0 }
func (e *Edit) CanRedo() bool {
	return e.IsEnabled() && !e.readonly && e.history.position < len(e.history.entries)
}

func (e *Edit) recordEdit(d editDelta) {
	if d.before == d.after {
		return
	}
	h := &e.history
	for _, old := range h.entries[h.position:] {
		h.bytes -= len(old.before) + len(old.after)
	}
	clear(h.entries[h.position:])
	h.entries = h.entries[:h.position]
	size := len(d.before) + len(d.after)
	if size > editHistoryBytes {
		e.ClearUndoHistory()
		return
	}
	for len(h.entries) > 0 && (len(h.entries) >= editHistoryLimit || h.bytes+size > editHistoryBytes) {
		h.bytes -= len(h.entries[0].before) + len(h.entries[0].after)
		copy(h.entries, h.entries[1:])
		h.entries[len(h.entries)-1] = editDelta{}
		h.entries = h.entries[:len(h.entries)-1]
	}
	h.entries = append(h.entries, d)
	h.bytes += size
	h.position = len(h.entries)
}

// Undo/Redo restore the changed span and selection, then publish one user edit.
// Navigation and selection are not undo entries. A committed input/paste is one
// entry; this field-level history does not change the IDE document undo stack.
func (e *Edit) Undo() { e.replayEdit(false) }
func (e *Edit) Redo() { e.replayEdit(true) }
func (e *Edit) replayEdit(redo bool) {
	if redo && !e.CanRedo() || !redo && !e.CanUndo() {
		return
	}
	h := &e.history
	i := h.position - 1
	if redo {
		i = h.position
	}
	d := h.entries[i]
	e.CancelPendingPaste()
	if redo {
		e.TextBlock.Replace(d.start, d.start+utf8.RuneCountInString(d.before), d.after)
		e.sel0, e.sel1 = d.end, d.end
		e.completionPrefixStart = d.prefixAfter
		h.position++
	} else {
		e.TextBlock.Replace(d.start, d.start+utf8.RuneCountInString(d.after), d.before)
		e.sel0, e.sel1 = d.anchor, d.caret
		e.completionPrefixStart = d.prefixBefore
		h.position--
	}
	e.editRevision++
	e.caretXSaved = false
	e.Layout()
	e.revalidate()
	e.refreshCompleter()
	e.emitEdited()
}

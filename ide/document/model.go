// Package document is Silk's shared text-document model: the single copy of
// a file's text that every view of that file agrees on, plus the revision
// counter, change notifications and stable positions an IDE needs on top of
// it.
//
// It exists because the editor owns its text per widget. When a tab is
// split, ged.EditorTabs.ToggleSplit creates a second gui.CodeEditor, copies
// the primary pane's text into it and cross-wires the two SigChanged
// callbacks; each pane then keeps its own buffer and the panes drift apart
// as soon as anything is not a plain full-text echo. With a Document the
// text lives in one place: no pane owns the truth, each one subscribes with
// OnChanged and applies the ChangeEvent it is handed.
//
// Conventions:
//
//   - Offsets are BYTE offsets into Text(). Slicing a string by byte offset
//     is what an edit ultimately does, so callers hand over the offsets they
//     already have; a caller that works in runes converts before calling.
//
//   - Lines are 0-based, matching gui.CodeEditor's line-keyed maps
//     (breakpoints, bookmarks, coverage, diff markers).
//
//   - Every edit funnels through Replace: it bumps the revision, remaps the
//     document's AnchorSet, then notifies subscribers. SetText is Replace
//     over the whole text, so there is exactly one change path.
//
// A Document is not safe for concurrent use: like the widgets that own it,
// it expects to be touched from the UI thread only.
package document

import "strings"

// ChangeEvent describes one edit that has already been applied to the
// document. Start and End are byte offsets into the text as it was BEFORE
// the edit ([Start,End) is the range that went away); NewText is what took
// its place, and Revision is the document revision after the edit.
//
// An insertion at p is {Start: p, End: p}; a deletion of [a,b) is
// {Start: a, End: b, NewText: ""}.
type ChangeEvent struct {
	Start    int
	End      int
	NewText  string
	Revision int
}

// Delta reports how many bytes the edit added (positive) or removed
// (negative). Every offset at or after End shifts by exactly this much.
func (ev ChangeEvent) Delta() int { return len(ev.NewText) - (ev.End - ev.Start) }

// subscriber is one OnChanged registration. The id lets the unsubscribe
// closure find its own entry after other subscribers have come and gone.
type subscriber struct {
	id int
	fn func(ev ChangeEvent)
}

// Document is one file's text shared by every view of it.
//
// The zero value is a usable empty document; New sets a Path and initial
// text.
type Document struct {
	// Path is the file this document stands for, kept as handed in (the
	// document itself never touches the filesystem). Empty for a scratch
	// buffer.
	Path string

	text string

	// revision counts applied edits; savedRevision is the revision the
	// on-disk copy corresponds to. Dirtiness is the comparison of the two,
	// so saving is a single assignment and needs no separate flag to keep
	// in sync.
	revision      int
	savedRevision int

	anchors *AnchorSet

	subs   []subscriber
	nextID int
}

// New returns a document holding text, tagged with path. The result is
// clean: revision 0 is treated as the saved state, so freshly loaded text
// is not dirty.
func New(path, text string) *Document {
	return &Document{Path: path, text: text}
}

// Text returns the whole document text.
func (d *Document) Text() string { return d.text }

// Len returns the document length in bytes.
func (d *Document) Len() int { return len(d.text) }

// SetText replaces the entire document with text. It is Replace over the
// full range, so subscribers see one ChangeEvent and anchors are remapped
// by the usual rules — which means anchors strictly inside the old text
// collapse to offset 0 and are marked deleted. Callers that reload a file
// and want line-keyed marks back should re-create them from the new text.
//
// Setting the text it already has is a no-op.
func (d *Document) SetText(text string) { d.Replace(0, len(d.text), text) }

// Replace is the document's one edit primitive: it drops the byte range
// [start,end) and puts s in its place, which covers insertion (start ==
// end), deletion (s == "") and replacement in a single compound step. One
// revision, one ChangeEvent, one anchor remap — an editor that expressed a
// replacement as delete-then-insert would move anchors twice and let
// subscribers observe an intermediate text that never really existed.
//
// The range is forgiving: it is ordered if reversed and clamped to the
// document, so a stale offset cannot panic. An edit that would not change
// the text is dropped: the revision does not move, no event is sent, and
// anchors stay put (a replacement by identical text has not deleted
// anything).
func (d *Document) Replace(start, end int, s string) {
	if start > end {
		start, end = end, start
	}
	n := len(d.text)
	start = min(max(start, 0), n)
	end = min(max(end, 0), n)
	if d.text[start:end] == s {
		return
	}

	d.text = d.text[:start] + s + d.text[end:]
	d.revision++
	ev := ChangeEvent{Start: start, End: end, NewText: s, Revision: d.revision}
	d.Anchors().Apply(ev)

	// Dispatch over a copy: a handler is allowed to unsubscribe (a closing
	// pane does exactly that) or to edit the document again, and neither may
	// disturb the walk in progress.
	subs := make([]subscriber, len(d.subs))
	copy(subs, d.subs)
	for _, sub := range subs {
		sub.fn(ev)
	}
}

// Revision returns the number of edits applied to the document. It only
// ever grows, so a view can cheaply tell whether it is looking at stale
// text by remembering the revision it last rendered.
func (d *Document) Revision() int { return d.revision }

// IsDirty reports whether the document has unsaved edits.
func (d *Document) IsDirty() bool { return d.revision != d.savedRevision }

// MarkSaved records that the current text is what sits on disk, clearing
// the dirty flag. Call it after a successful write.
func (d *Document) MarkSaved() { d.savedRevision = d.revision }

// OnChanged subscribes fn to every applied edit and returns the function
// that cancels the subscription. Handlers run in registration order, after
// the text and the anchors have been updated, so a handler sees the new
// state through Text() and Anchors().
//
// Cancelling is idempotent and safe from inside a handler. A view MUST
// cancel when it goes away — the leak the split editor works around today
// by saving and restoring the previous callback.
func (d *Document) OnChanged(fn func(ev ChangeEvent)) func() {
	if fn == nil {
		return func() {}
	}
	d.nextID++
	id := d.nextID
	d.subs = append(d.subs, subscriber{id: id, fn: fn})
	return func() {
		for i := range d.subs {
			if d.subs[i].id == id {
				d.subs = append(d.subs[:i], d.subs[i+1:]...)
				return
			}
		}
	}
}

// Anchors returns the document's anchor set. Every Replace remaps it, so
// positions registered here follow the text without the owner wiring
// anything up.
func (d *Document) Anchors() *AnchorSet {
	if d.anchors == nil {
		d.anchors = NewAnchorSet()
	}
	return d.anchors
}

// LineCount returns the number of lines, counting the empty last line that
// a trailing newline creates. An empty document has one line.
func (d *Document) LineCount() int { return strings.Count(d.text, "\n") + 1 }

// LineStart returns the byte offset at which the 0-based line begins.
// Out-of-range lines clamp: a negative line gives 0, a line past the last
// gives the document length.
func (d *Document) LineStart(line int) int {
	off := 0
	for i := 0; i < line; i++ {
		nl := strings.IndexByte(d.text[off:], '\n')
		if nl < 0 {
			return len(d.text)
		}
		off += nl + 1
	}
	return off
}

// LineOf returns the 0-based line containing the byte offset. A newline
// belongs to the line it terminates, and the offset is clamped into the
// document first.
func (d *Document) LineOf(offset int) int {
	offset = min(max(offset, 0), len(d.text))
	return strings.Count(d.text[:offset], "\n")
}

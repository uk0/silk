package document

import "sort"

// AnchorID identifies one anchor inside an AnchorSet. Owners keep the id,
// never a raw offset — the offset is what moves.
type AnchorID int

// Anchor is a snapshot of one anchor: where it currently sits and whether
// the text it pointed at has been deleted. It is a value, so reading one
// out of a set cannot accidentally mutate it.
type Anchor struct {
	ID      AnchorID
	Offset  int
	Deleted bool
}

// AnchorSet holds positions that stay attached to the text they were
// created on. Everything an IDE pins to a location — bookmarks,
// breakpoints, fold regions, diagnostics, a saved caret — is a byte offset
// that must survive edits made somewhere else in the file, which a plain
// int cannot do: insert one character above a breakpoint and every raw
// offset below it is wrong. gui.CodeEditor documents exactly this gap
// ("breakpoints are NOT re-mapped when lines are inserted/deleted").
//
// Apply moves the whole set through one edit. For an edit that replaced
// [Start,End) with NewText:
//
//   - offset <= Start: unchanged. Left gravity — text inserted at an anchor
//     lands after it, so the anchor keeps pointing at what it pointed at.
//   - offset >= End: shifted by the edit's Delta.
//   - Start < offset < End: collapsed to Start and marked Deleted. The text
//     it addressed is gone, so the anchor stays at the edit's start offset
//     and its owner decides whether to drop it or keep showing it.
//
// Deleted is sticky: later edits keep moving the anchor, but nothing clears
// the flag.
type AnchorSet struct {
	marks  map[AnchorID]Anchor
	nextID AnchorID
}

// NewAnchorSet returns an empty set. A zero-value AnchorSet is also usable;
// Add creates the map on demand.
func NewAnchorSet() *AnchorSet {
	return &AnchorSet{marks: make(map[AnchorID]Anchor)}
}

// Add pins a new anchor at the byte offset (clamped at 0) and returns its
// id. The set does not know the text length, so the caller is responsible
// for passing an offset that is inside the document.
func (s *AnchorSet) Add(offset int) AnchorID {
	if s.marks == nil {
		s.marks = make(map[AnchorID]Anchor)
	}
	s.nextID++
	s.marks[s.nextID] = Anchor{ID: s.nextID, Offset: max(offset, 0)}
	return s.nextID
}

// Get returns the anchor and whether the id is known.
func (s *AnchorSet) Get(id AnchorID) (Anchor, bool) {
	a, ok := s.marks[id]
	return a, ok
}

// Offset returns the anchor's current byte offset, or -1 for an unknown id.
func (s *AnchorSet) Offset(id AnchorID) int {
	if a, ok := s.marks[id]; ok {
		return a.Offset
	}
	return -1
}

// Remove drops the anchor and reports whether it was there.
func (s *AnchorSet) Remove(id AnchorID) bool {
	if _, ok := s.marks[id]; !ok {
		return false
	}
	delete(s.marks, id)
	return true
}

// Len returns how many anchors the set holds, deleted ones included.
func (s *AnchorSet) Len() int { return len(s.marks) }

// All returns every anchor ordered by id, i.e. in creation order. Ordered
// so callers and tests get a stable list out of the underlying map.
func (s *AnchorSet) All() []Anchor {
	out := make([]Anchor, 0, len(s.marks))
	for _, a := range s.marks {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Apply remaps every anchor through one applied edit, per the rules on
// AnchorSet. A Document does this for its own set on each Replace; call it
// directly on a set that is fed from a subscription instead.
func (s *AnchorSet) Apply(ev ChangeEvent) {
	start, end := ev.Start, ev.End
	if start > end {
		start, end = end, start
	}
	delta := len(ev.NewText) - (end - start)
	for id, a := range s.marks {
		if a.Offset <= start {
			continue
		}
		if a.Offset >= end {
			a.Offset += delta
		} else {
			a.Offset = start
			a.Deleted = true
		}
		s.marks[id] = a
	}
}

// LineAnchor is a line-keyed anchor: an anchor pinned to the start of a
// line, which reports the line it has drifted to rather than an offset.
// Bookmarks and breakpoints are line-keyed, so this is the shape they want
// — store a LineAnchor instead of an int and ask Line() when rendering.
//
// Because it sits at the line's START offset, deleting the whole line
// leaves the anchor at the start of the following line: the mark survives
// and is not flagged Deleted (that flag is for an anchor swallowed strictly
// inside a deleted range). An owner that would rather drop such a mark can
// compare Line() before and after the edit.
type LineAnchor struct {
	doc *Document
	id  AnchorID
}

// NewLineAnchor pins an anchor to the start of the 0-based line and returns
// the handle. The anchor lives in the document's own AnchorSet, so it is
// remapped by every Replace.
func (d *Document) NewLineAnchor(line int) *LineAnchor {
	return &LineAnchor{doc: d, id: d.Anchors().Add(d.LineStart(line))}
}

// ID returns the underlying anchor id in the document's AnchorSet.
func (a *LineAnchor) ID() AnchorID { return a.id }

// Line returns the 0-based line the anchor now sits on, or -1 once the
// anchor has been removed from the set.
func (a *LineAnchor) Line() int {
	an, ok := a.doc.Anchors().Get(a.id)
	if !ok {
		return -1
	}
	return a.doc.LineOf(an.Offset)
}

// Offset returns the anchor's current byte offset, or -1 once it has been
// removed.
func (a *LineAnchor) Offset() int { return a.doc.Anchors().Offset(a.id) }

// Deleted reports whether the anchor was swallowed by a deletion, or has
// been removed from the set outright.
func (a *LineAnchor) Deleted() bool {
	an, ok := a.doc.Anchors().Get(a.id)
	return !ok || an.Deleted
}

// Remove drops the anchor from the document's set. Call it when the mark it
// backs is deleted, otherwise the set grows for the life of the document.
func (a *LineAnchor) Remove() { a.doc.Anchors().Remove(a.id) }

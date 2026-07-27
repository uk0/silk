package document

import "testing"

// --- helpers ---------------------------------------------------------

// wantAnchor checks an anchor's offset, its deleted flag, and — the point of
// an anchor — that it still addresses the text it was created on.
func wantAnchor(t *testing.T, d *Document, id AnchorID, offset int, deleted bool, at string) {
	t.Helper()
	a, ok := d.Anchors().Get(id)
	if !ok {
		t.Fatalf("anchor %d missing from the set", id)
	}
	if a.Offset != offset {
		t.Fatalf("anchor %d Offset = %d, want %d (text %q)", id, a.Offset, offset, d.Text())
	}
	if a.Deleted != deleted {
		t.Fatalf("anchor %d Deleted = %v, want %v", id, a.Deleted, deleted)
	}
	if at == "" {
		return
	}
	end := a.Offset + len(at)
	if end > len(d.Text()) || d.Text()[a.Offset:end] != at {
		t.Fatalf("anchor %d at offset %d points at %q, want %q (text %q)",
			id, a.Offset, safeSlice(d.Text(), a.Offset, end), at, d.Text())
	}
}

func safeSlice(s string, from, to int) string {
	if from > len(s) {
		from = len(s)
	}
	if to > len(s) {
		to = len(s)
	}
	return s[from:to]
}

// --- set basics ------------------------------------------------------

func TestAnchorSetAddGet(t *testing.T) {
	s := NewAnchorSet()
	if s.Len() != 0 {
		t.Fatalf("fresh set Len() = %d, want 0", s.Len())
	}
	a := s.Add(4)
	b := s.Add(9)
	if a == b {
		t.Fatalf("Add returned the same id twice (%d)", a)
	}
	if got := s.Offset(a); got != 4 {
		t.Errorf("Offset(a) = %d, want 4", got)
	}
	if got := s.Offset(b); got != 9 {
		t.Errorf("Offset(b) = %d, want 9", got)
	}
	if got := s.Offset(AnchorID(12345)); got != -1 {
		t.Errorf("Offset(unknown) = %d, want -1", got)
	}
	if _, ok := s.Get(AnchorID(12345)); ok {
		t.Errorf("Get(unknown) reported ok")
	}
	if got := s.Add(-5); s.Offset(got) != 0 {
		t.Errorf("Add(-5) offset = %d, want 0 (clamped)", s.Offset(got))
	}
	// Reading an anchor out yields a copy: mutating it cannot corrupt the set.
	got, _ := s.Get(a)
	got.Offset = 999
	if s.Offset(a) != 4 {
		t.Errorf("mutating a returned Anchor changed the set")
	}
}

func TestAnchorSetRemoveAndAll(t *testing.T) {
	s := NewAnchorSet()
	a, b, c := s.Add(1), s.Add(2), s.Add(3)
	if s.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", s.Len())
	}
	if !s.Remove(b) {
		t.Fatalf("Remove(b) = false, want true")
	}
	if s.Remove(b) {
		t.Fatalf("Remove(b) twice = true, want false")
	}
	if s.Len() != 2 {
		t.Fatalf("Len() after remove = %d, want 2", s.Len())
	}
	all := s.All()
	if len(all) != 2 || all[0].ID != a || all[1].ID != c {
		t.Fatalf("All() = %+v, want anchors %d then %d in creation order", all, a, c)
	}
}

func TestZeroValueAnchorSetUsable(t *testing.T) {
	var s AnchorSet // no NewAnchorSet: the map is created on demand
	id := s.Add(3)
	s.Apply(ChangeEvent{Start: 0, End: 0, NewText: "xx"})
	if got := s.Offset(id); got != 5 {
		t.Fatalf("Offset() = %d, want 5", got)
	}
}

// --- remapping through one edit --------------------------------------

// TestAnchorRemap walks every shape an edit can have against an anchor on
// the 'w' of "hello world" (offset 6) and checks where the anchor lands.
func TestAnchorRemap(t *testing.T) {
	const text = "hello world" // h0 e1 l2 l3 o4 _5 w6 o7 r8 l9 d10
	const anchorAt = 6

	cases := []struct {
		name        string
		start, end  int
		s           string
		wantOffset  int
		wantDeleted bool
		wantText    string // whole document after the edit
		wantAt      string // what the anchor addresses afterwards
	}{
		{
			name: "insert-before", start: 0, end: 0, s: "say ",
			wantOffset: 10, wantText: "say hello world", wantAt: "world",
		},
		{
			name: "insert-just-before", start: 5, end: 5, s: "!",
			wantOffset: 7, wantText: "hello! world", wantAt: "world",
		},
		{
			// Left gravity: text typed at the anchor lands after it, so the
			// anchor keeps its offset and now addresses the new text.
			name: "insert-at-anchor", start: 6, end: 6, s: "big ",
			wantOffset: 6, wantText: "hello big world", wantAt: "big ",
		},
		{
			name: "insert-after", start: 8, end: 8, s: "-",
			wantOffset: 6, wantText: "hello wo-rld", wantAt: "wo-",
		},
		{
			name: "insert-at-end", start: 11, end: 11, s: "!",
			wantOffset: 6, wantText: "hello world!", wantAt: "world!",
		},
		{
			name: "delete-before", start: 0, end: 6, s: "",
			wantOffset: 0, wantText: "world", wantAt: "world",
		},
		{
			// The anchor sits exactly on the deletion's end bound, which is
			// outside [start,end): it shifts, it is not swallowed.
			name: "delete-up-to-anchor", start: 2, end: 6, s: "",
			wantOffset: 2, wantText: "heworld", wantAt: "world",
		},
		{
			name: "delete-after", start: 7, end: 11, s: "",
			wantOffset: 6, wantText: "hello w", wantAt: "w",
		},
		{
			// The anchor is strictly inside the deleted range: it collapses
			// to the range start and is flagged deleted.
			name: "delete-spanning", start: 5, end: 8, s: "",
			wantOffset: 5, wantDeleted: true, wantText: "hellorld", wantAt: "rld",
		},
		{
			name: "delete-all", start: 0, end: 11, s: "",
			wantOffset: 0, wantDeleted: true, wantText: "", wantAt: "",
		},
		{
			name: "replace-shorter", start: 0, end: 5, s: "hi",
			wantOffset: 3, wantText: "hi world", wantAt: "world",
		},
		{
			name: "replace-longer", start: 0, end: 5, s: "greetings",
			wantOffset: 10, wantText: "greetings world", wantAt: "world",
		},
		{
			name: "replace-same-length", start: 0, end: 5, s: "HELLO",
			wantOffset: 6, wantText: "HELLO world", wantAt: "world",
		},
		{
			name: "replace-spanning", start: 4, end: 8, s: "XY",
			wantOffset: 4, wantDeleted: true, wantText: "hellXYrld", wantAt: "XY",
		},
		{
			name: "replace-whole", start: 0, end: 11, s: "new",
			wantOffset: 0, wantDeleted: true, wantText: "new", wantAt: "new",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := New("a.txt", text)
			id := d.Anchors().Add(anchorAt)
			d.Replace(c.start, c.end, c.s)
			wantText(t, d, c.wantText)
			wantAnchor(t, d, id, c.wantOffset, c.wantDeleted, c.wantAt)
		})
	}
}

// TestAnchorAtDeletionStartSurvives documents the other boundary: an anchor
// exactly on the first byte of a deleted range is not "inside" it, so it
// survives at that offset (now the text that followed the deletion) rather
// than being flagged deleted.
func TestAnchorAtDeletionStartSurvives(t *testing.T) {
	d := New("a.txt", "hello world")
	id := d.Anchors().Add(6)
	d.Replace(6, 11, "") // delete "world", the very text the anchor pinned
	wantText(t, d, "hello ")
	wantAnchor(t, d, id, 6, false, "")
}

func TestAnchorsRemappedWithoutSubscription(t *testing.T) {
	d := New("a.txt", "abcdef")
	first := d.Anchors().Add(0)
	mid := d.Anchors().Add(3)
	last := d.Anchors().Add(6)

	d.Replace(1, 2, "XYZ") // abc -> aXYZcdef, delta +2

	wantText(t, d, "aXYZcdef")
	wantAnchor(t, d, first, 0, false, "a")
	wantAnchor(t, d, mid, 5, false, "d")
	wantAnchor(t, d, last, 8, false, "")
	if d.Anchors().Len() != 3 {
		t.Fatalf("Len() = %d, want 3", d.Anchors().Len())
	}
}

// TestAnchorSetFedBySubscription: a consumer keeping its own set (so its
// anchor ids are private) wires Apply to OnChanged and gets the same result.
func TestAnchorSetFedBySubscription(t *testing.T) {
	d := New("a.txt", "hello world")
	own := NewAnchorSet()
	id := own.Add(6)
	stop := d.OnChanged(func(ev ChangeEvent) { own.Apply(ev) })

	d.Replace(0, 5, "hi")
	if got := own.Offset(id); got != 3 {
		t.Fatalf("Offset() = %d, want 3", got)
	}
	stop()
	d.Replace(0, 0, "// ") // no longer subscribed: the set must stand still
	if got := own.Offset(id); got != 3 {
		t.Fatalf("Offset() after unsubscribe = %d, want 3", got)
	}
}

func TestApplyOrdersReversedRange(t *testing.T) {
	s := NewAnchorSet()
	id := s.Add(10)
	// A hand-built event with the bounds the wrong way round still remaps as
	// [3,8): delta -5, so the anchor at 10 lands on 5.
	s.Apply(ChangeEvent{Start: 8, End: 3, NewText: ""})
	if got := s.Offset(id); got != 5 {
		t.Fatalf("Offset() = %d, want 5", got)
	}
}

// --- sequences of edits ----------------------------------------------

// TestAnchorMultiEditSequence tracks one word through a run of unrelated
// edits — the case a single-edit remap can pass and a stateful one cannot.
func TestAnchorMultiEditSequence(t *testing.T) {
	d := New("a.txt", "hello world")
	id := d.Anchors().Add(6) // the 'w' of "world"

	d.Replace(0, 0, "// ") // prepend a comment marker
	wantText(t, d, "// hello world")
	wantAnchor(t, d, id, 9, false, "world")

	d.Replace(3, 8, "HELLO") // same-length rewrite before the anchor
	wantText(t, d, "// HELLO world")
	wantAnchor(t, d, id, 9, false, "world")

	d.Replace(d.Len(), d.Len(), "!") // append after the anchor
	wantText(t, d, "// HELLO world!")
	wantAnchor(t, d, id, 9, false, "world!")

	d.Replace(0, 3, "") // drop the comment marker again
	wantText(t, d, "HELLO world!")
	wantAnchor(t, d, id, 6, false, "world!")

	d.Replace(2, 5, "y") // shrink before the anchor
	wantText(t, d, "HEy world!")
	wantAnchor(t, d, id, 4, false, "world!")

	if d.Revision() != 5 || !d.IsDirty() {
		t.Fatalf("Revision() = %d, IsDirty() = %v; want 5 and true", d.Revision(), d.IsDirty())
	}
}

// TestAnchorDeletedFlagIsSticky: once swallowed, an anchor keeps being
// remapped (so its offset stays meaningful) but never loses the flag.
func TestAnchorDeletedFlagIsSticky(t *testing.T) {
	d := New("a.txt", "one two three")
	id := d.Anchors().Add(4) // the 't' of "two"

	d.Replace(3, 7, "") // delete " two", swallowing the anchor
	wantText(t, d, "one three")
	wantAnchor(t, d, id, 3, true, " three")

	d.Replace(0, 0, "zero ") // an unrelated later edit still moves it
	wantText(t, d, "zero one three")
	wantAnchor(t, d, id, 8, true, " three")

	d.Replace(d.Len(), d.Len(), "!") // an edit after it leaves it alone
	wantAnchor(t, d, id, 8, true, " three")
}

// TestSetTextCollapsesAnchors pins the documented consequence of SetText
// being a full-range Replace: everything strictly inside the old text
// collapses to 0 and is marked deleted, so a caller reloading a file must
// re-create its line-keyed marks.
func TestSetTextCollapsesAnchors(t *testing.T) {
	d := New("a.txt", "one\ntwo\n")
	head := d.Anchors().Add(0)
	inner := d.Anchors().Add(4)

	d.SetText("ONE\nTWO\n")
	wantAnchor(t, d, head, 0, false, "ONE")
	wantAnchor(t, d, inner, 0, true, "ONE")
}

// --- line anchors ----------------------------------------------------

func wantLine(t *testing.T, a *LineAnchor, line int, deleted bool) {
	t.Helper()
	if got := a.Line(); got != line {
		t.Fatalf("Line() = %d, want %d", got, line)
	}
	if got := a.Deleted(); got != deleted {
		t.Fatalf("Deleted() = %v, want %v", got, deleted)
	}
}

func TestNewLineAnchorPinsLineStart(t *testing.T) {
	d := New("a.txt", "l0\nl1\nl2\n")
	for line := 0; line < d.LineCount(); line++ {
		a := d.NewLineAnchor(line)
		if got := a.Offset(); got != d.LineStart(line) {
			t.Errorf("line %d: Offset() = %d, want %d", line, got, d.LineStart(line))
		}
		wantLine(t, a, line, false)
	}
	// Out-of-range lines clamp the same way LineStart does.
	if got := d.NewLineAnchor(-1).Line(); got != 0 {
		t.Errorf("NewLineAnchor(-1).Line() = %d, want 0", got)
	}
	if got := d.NewLineAnchor(99).Line(); got != d.LineCount()-1 {
		t.Errorf("NewLineAnchor(99).Line() = %d, want %d", got, d.LineCount()-1)
	}
}

// TestLineAnchorSurvivesLineInsertion is the breakpoint/bookmark case
// gui.CodeEditor cannot handle today: insert a line above a mark and the
// mark must follow, not stay on its old number.
func TestLineAnchorSurvivesLineInsertion(t *testing.T) {
	d := New("a.txt", "line0\nline1\nline2\nline3\n")
	mark := d.NewLineAnchor(2) // on "line2"

	d.Replace(d.LineStart(1), d.LineStart(1), "NEW\n") // whole line above it
	wantText(t, d, "line0\nNEW\nline1\nline2\nline3\n")
	wantLine(t, mark, 3, false)
	if got := mark.Offset(); got != d.LineStart(3) {
		t.Fatalf("Offset() = %d, want LineStart(3) = %d", got, d.LineStart(3))
	}

	// A line inserted below the mark must not move it.
	d.Replace(d.LineStart(4), d.LineStart(4), "TAIL\n")
	wantLine(t, mark, 3, false)

	// A split of the marked line itself (Enter pressed inside it) keeps the
	// mark on the first half.
	d.Replace(d.LineStart(3)+2, d.LineStart(3)+2, "\n")
	wantLine(t, mark, 3, false)
}

// TestLineAnchorSurvivesLineRemoval: delete a line above the mark and the
// mark's number drops by one.
func TestLineAnchorSurvivesLineRemoval(t *testing.T) {
	d := New("a.txt", "line0\nline1\nline2\nline3\n")
	mark := d.NewLineAnchor(2)

	d.Replace(0, d.LineStart(1), "") // drop "line0\n"
	wantText(t, d, "line1\nline2\nline3\n")
	wantLine(t, mark, 1, false)
	if got := mark.Offset(); got != d.LineStart(1) {
		t.Fatalf("Offset() = %d, want LineStart(1) = %d", got, d.LineStart(1))
	}

	// Joining the two lines above the mark pulls it up again.
	d.Replace(5, 6, "") // remove the newline ending "line1"
	wantText(t, d, "line1line2\nline3\n")
	wantLine(t, mark, 0, false)
}

// TestLineAnchorMarkedLineDeleted documents the line-anchor boundary: the
// anchor sits at the line's start offset, so deleting the whole line leaves
// it at the start of the following line — the mark survives and is NOT
// flagged deleted. An owner that wants such marks dropped compares Line().
func TestLineAnchorMarkedLineDeleted(t *testing.T) {
	d := New("a.txt", "line0\nline1\nline2\nline3\n")
	mark := d.NewLineAnchor(2)

	d.Replace(d.LineStart(2), d.LineStart(3), "") // delete "line2\n"
	wantText(t, d, "line0\nline1\nline3\n")
	wantLine(t, mark, 2, false)
	if got := d.Text()[mark.Offset() : mark.Offset()+5]; got != "line3" {
		t.Fatalf("mark now addresses %q, want line3", got)
	}
}

// TestLineAnchorDeletedInsideRange: a mark on a line that is swallowed by a
// multi-line deletion starting mid-line does get flagged.
func TestLineAnchorDeletedInsideRange(t *testing.T) {
	d := New("a.txt", "line0\nline1\nline2\nline3\n")
	mark := d.NewLineAnchor(2)

	// Delete from the middle of line1 through the end of line2.
	d.Replace(d.LineStart(1)+2, d.LineStart(3), "")
	wantText(t, d, "line0\nliline3\n")
	wantLine(t, mark, 1, true)
}

// TestLineAnchorsAsBookmarks drives the real consumer shape: a set of
// line-keyed marks kept across edits made elsewhere in the file.
func TestLineAnchorsAsBookmarks(t *testing.T) {
	d := New("main.go", "line0\nline1\nline2\nline3\n")
	marks := []*LineAnchor{d.NewLineAnchor(1), d.NewLineAnchor(2), d.NewLineAnchor(3)}

	d.Replace(0, 0, "top\n") // a line inserted above all three
	for i, want := range []int{2, 3, 4} {
		wantLine(t, marks[i], want, false)
	}

	// Delete the line the first mark sits on ("line1", now line 2). It falls
	// onto the following line, where the second mark has also landed.
	d.Replace(d.LineStart(2), d.LineStart(3), "")
	wantText(t, d, "top\nline0\nline2\nline3\n")
	for i, want := range []int{2, 2, 3} {
		wantLine(t, marks[i], want, false)
	}

	// Dropping a mark releases its anchor.
	before := d.Anchors().Len()
	marks[0].Remove()
	if got := d.Anchors().Len(); got != before-1 {
		t.Fatalf("Anchors().Len() = %d after Remove, want %d", got, before-1)
	}
	if got := marks[0].Line(); got != -1 {
		t.Fatalf("removed mark Line() = %d, want -1", got)
	}
	if got := marks[0].Offset(); got != -1 {
		t.Fatalf("removed mark Offset() = %d, want -1", got)
	}
	if !marks[0].Deleted() {
		t.Fatalf("removed mark Deleted() = false, want true")
	}
	marks[0].Remove() // removing twice must not panic

	// The surviving marks keep tracking.
	d.Replace(0, 0, "// header\n")
	wantLine(t, marks[1], 3, false)
	wantLine(t, marks[2], 4, false)
	if got := marks[1].ID(); got == 0 {
		t.Fatalf("ID() = 0, want a live anchor id")
	}
}

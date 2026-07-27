package document

import "testing"

// --- helpers ---------------------------------------------------------

func wantText(t *testing.T, d *Document, want string) {
	t.Helper()
	if got := d.Text(); got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}

func wantRev(t *testing.T, d *Document, rev int, dirty bool) {
	t.Helper()
	if got := d.Revision(); got != rev {
		t.Fatalf("Revision() = %d, want %d", got, rev)
	}
	if got := d.IsDirty(); got != dirty {
		t.Fatalf("IsDirty() = %v at revision %d, want %v", got, rev, dirty)
	}
}

// --- construction ----------------------------------------------------

func TestNewInitialState(t *testing.T) {
	d := New("/tmp/main.go", "package main\n")
	if d.Path != "/tmp/main.go" {
		t.Fatalf("Path = %q, want /tmp/main.go", d.Path)
	}
	wantText(t, d, "package main\n")
	if d.Len() != len("package main\n") {
		t.Fatalf("Len() = %d, want %d", d.Len(), len("package main\n"))
	}
	// Freshly loaded text is the saved state: revision 0, not dirty.
	wantRev(t, d, 0, false)
}

func TestZeroValueDocumentUsable(t *testing.T) {
	var d Document // no New: anchors map and subscriber list are lazy
	wantText(t, &d, "")
	wantRev(t, &d, 0, false)

	id := d.Anchors().Add(0)
	d.Replace(0, 0, "abc")
	wantText(t, &d, "abc")
	wantRev(t, &d, 1, true)
	if got := d.Anchors().Offset(id); got != 0 {
		t.Fatalf("anchor at 0 after insert at 0: Offset() = %d, want 0", got)
	}
}

// --- revision / dirty / save -----------------------------------------

func TestRevisionAndDirtyTransitions(t *testing.T) {
	d := New("a.txt", "one")
	wantRev(t, d, 0, false)

	d.SetText("two")
	wantRev(t, d, 1, true)

	d.Replace(3, 3, "-three")
	wantText(t, d, "two-three")
	wantRev(t, d, 2, true)

	d.MarkSaved()
	wantRev(t, d, 2, false) // revision is untouched by saving

	d.Replace(0, 3, "TWO")
	wantText(t, d, "TWO-three")
	wantRev(t, d, 3, true)

	d.MarkSaved()
	wantRev(t, d, 3, false)
	d.MarkSaved() // idempotent
	wantRev(t, d, 3, false)
}

// TestDirtyAfterRevertingEdit pins the revision-counter semantics: dirtiness
// is "revision != savedRevision", so typing a character and deleting it
// again leaves the document dirty. Only MarkSaved clears it.
func TestDirtyAfterRevertingEdit(t *testing.T) {
	d := New("a.txt", "abc")
	d.Replace(3, 3, "d")
	d.Replace(3, 4, "")
	wantText(t, d, "abc")
	wantRev(t, d, 2, true)
}

func TestNoOpEditsDoNotBumpRevision(t *testing.T) {
	d := New("a.txt", "hello")
	var events int
	d.OnChanged(func(ev ChangeEvent) { events++ })

	d.SetText("hello")       // same whole text
	d.Replace(0, 5, "hello") // same range content
	d.Replace(2, 2, "")      // empty insert
	d.Replace(3, 3, "")      // empty insert elsewhere
	d.Replace(99, 120, "")   // clamps to [5,5): empty delete past the end
	d.Replace(-4, -1, "")    // clamps to [0,0)
	wantText(t, d, "hello")
	wantRev(t, d, 0, false)
	if events != 0 {
		t.Fatalf("no-op edits fired %d ChangeEvents, want 0", events)
	}
}

// --- Replace ---------------------------------------------------------

func TestReplaceCompoundEdit(t *testing.T) {
	cases := []struct {
		name             string
		text             string
		start, end       int
		s                string
		want             string
		wantStart, wantE int
	}{
		{"insert-at-start", "world", 0, 0, "hello ", "hello world", 0, 0},
		{"insert-in-middle", "ab", 1, 1, "XY", "aXYb", 1, 1},
		{"insert-at-end", "ab", 2, 2, "!", "ab!", 2, 2},
		{"delete-range", "hello world", 5, 11, "", "hello", 5, 11},
		{"delete-all", "hello", 0, 5, "", "", 0, 5},
		{"replace-shorter", "hello world", 0, 5, "hi", "hi world", 0, 5},
		{"replace-longer", "hello world", 0, 5, "greetings", "greetings world", 0, 5},
		{"replace-same-length", "hello", 0, 5, "HELLO", "HELLO", 0, 5},
		{"reversed-range-ordered", "hello", 4, 1, "-", "h-o", 1, 4},
		{"range-clamped-high", "hello", 3, 99, "!", "hel!", 3, 5},
		{"range-clamped-low", "hello", -7, 2, "X", "Xllo", 0, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := New("a.txt", c.text)
			var got []ChangeEvent
			d.OnChanged(func(ev ChangeEvent) { got = append(got, ev) })

			d.Replace(c.start, c.end, c.s)
			wantText(t, d, c.want)
			wantRev(t, d, 1, true)

			if len(got) != 1 {
				t.Fatalf("got %d ChangeEvents, want 1", len(got))
			}
			ev := got[0]
			// The event reports the normalised range, so a subscriber can
			// splice it into its own copy without re-deriving the clamp.
			if ev.Start != c.wantStart || ev.End != c.wantE || ev.NewText != c.s || ev.Revision != 1 {
				t.Fatalf("event = %+v, want {Start:%d End:%d NewText:%q Revision:1}",
					ev, c.wantStart, c.wantE, c.s)
			}
		})
	}
}

func TestChangeEventDelta(t *testing.T) {
	cases := []struct {
		ev   ChangeEvent
		want int
	}{
		{ChangeEvent{Start: 0, End: 0, NewText: "abc"}, 3},   // pure insert
		{ChangeEvent{Start: 2, End: 5, NewText: ""}, -3},     // pure delete
		{ChangeEvent{Start: 2, End: 5, NewText: "ab"}, -1},   // shorter
		{ChangeEvent{Start: 2, End: 5, NewText: "abcde"}, 2}, // longer
		{ChangeEvent{Start: 2, End: 5, NewText: "abc"}, 0},   // same length
	}
	for _, c := range cases {
		if got := c.ev.Delta(); got != c.want {
			t.Errorf("%+v Delta() = %d, want %d", c.ev, got, c.want)
		}
	}
}

// --- subscriptions ---------------------------------------------------

func TestOnChangedEventSequence(t *testing.T) {
	d := New("a.txt", "abc")
	var got []ChangeEvent
	d.OnChanged(func(ev ChangeEvent) { got = append(got, ev) })

	d.Replace(3, 3, "def") // abcdef
	d.Replace(0, 1, "A")   // Abcdef
	d.SetText("x")         // x

	want := []ChangeEvent{
		{Start: 3, End: 3, NewText: "def", Revision: 1},
		{Start: 0, End: 1, NewText: "A", Revision: 2},
		{Start: 0, End: 6, NewText: "x", Revision: 3},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestOnChangedSeesNewState: a handler runs after the text and anchors have
// been updated, which is what lets a pane repaint straight from the document.
func TestOnChangedSeesNewState(t *testing.T) {
	d := New("a.txt", "abc")
	id := d.Anchors().Add(2)
	var seenText string
	var seenOffset, seenRev int
	d.OnChanged(func(ev ChangeEvent) {
		seenText = d.Text()
		seenOffset = d.Anchors().Offset(id)
		seenRev = d.Revision()
	})

	d.Replace(0, 0, "12")
	if seenText != "12abc" {
		t.Errorf("handler saw Text() = %q, want %q", seenText, "12abc")
	}
	if seenOffset != 4 {
		t.Errorf("handler saw anchor offset %d, want 4", seenOffset)
	}
	if seenRev != 1 {
		t.Errorf("handler saw Revision() = %d, want 1", seenRev)
	}
}

func TestOnChangedMultipleSubscribersInOrder(t *testing.T) {
	d := New("a.txt", "")
	var order []string
	d.OnChanged(func(ev ChangeEvent) { order = append(order, "first") })
	d.OnChanged(func(ev ChangeEvent) { order = append(order, "second") })
	d.OnChanged(nil) // ignored, must not panic or shift the order

	d.Replace(0, 0, "x")
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("dispatch order = %v, want [first second]", order)
	}
}

func TestOnChangedUnsubscribe(t *testing.T) {
	d := New("a.txt", "")
	var a, b int
	stopA := d.OnChanged(func(ev ChangeEvent) { a++ })
	d.OnChanged(func(ev ChangeEvent) { b++ })

	d.Replace(0, 0, "1")
	stopA()
	d.Replace(1, 1, "2")
	stopA() // cancelling twice is a no-op
	d.Replace(2, 2, "3")

	if a != 1 {
		t.Errorf("cancelled subscriber got %d events, want 1", a)
	}
	if b != 3 {
		t.Errorf("live subscriber got %d events, want 3", b)
	}
	// Cancelling nil's stub must also be harmless.
	d.OnChanged(nil)()
}

// TestOnChangedUnsubscribeDuringDispatch: dispatch walks a snapshot, so a
// handler that tears itself (and a sibling) down mid-event — a split pane
// closing in response to a change — cannot corrupt the walk in progress.
func TestOnChangedUnsubscribeDuringDispatch(t *testing.T) {
	d := New("a.txt", "")
	var a, b int
	var stopA, stopB func()
	stopA = d.OnChanged(func(ev ChangeEvent) {
		a++
		stopA()
		stopB()
	})
	stopB = d.OnChanged(func(ev ChangeEvent) { b++ })

	d.Replace(0, 0, "1")
	if a != 1 || b != 1 {
		t.Fatalf("first event: a=%d b=%d, want 1 and 1 (snapshot dispatch)", a, b)
	}
	d.Replace(1, 1, "2")
	if a != 1 || b != 1 {
		t.Fatalf("after self-cancel: a=%d b=%d, want both still 1", a, b)
	}
}

// TestOnChangedReentrantEdit: a handler may edit the document again (an
// auto-indent or trailing-whitespace trim would). The nested edit gets its
// own revision and its own event, and the text ends up consistent.
func TestOnChangedReentrantEdit(t *testing.T) {
	d := New("a.txt", "")
	var revs []int
	done := false
	d.OnChanged(func(ev ChangeEvent) {
		revs = append(revs, ev.Revision)
		if !done {
			done = true
			d.Replace(d.Len(), d.Len(), "\n")
		}
	})

	d.Replace(0, 0, "line")
	wantText(t, d, "line\n")
	wantRev(t, d, 2, true)
	if len(revs) != 2 || revs[0] != 1 || revs[1] != 2 {
		t.Fatalf("observed revisions %v, want [1 2]", revs)
	}
}

// --- the split-pane scenario -----------------------------------------

// pane stands in for a gui.CodeEditor view of a document. It keeps its own
// copy of the text (a widget must) and refreshes it by splicing each
// ChangeEvent — proving the event alone carries enough information, with no
// full-text echo between panes.
type pane struct {
	text   string
	events int
	rev    int
}

func (p *pane) attach(d *Document) func() {
	p.text = d.Text()
	return d.OnChanged(func(ev ChangeEvent) {
		p.text = p.text[:ev.Start] + ev.NewText + p.text[ev.End:]
		p.events++
		p.rev = ev.Revision
	})
}

// TestSharedDocumentKeepsPanesInSync is the reason this package exists:
// two views of one document, edits arriving from either of them, and both
// views still showing the same text as the document afterwards.
func TestSharedDocumentKeepsPanesInSync(t *testing.T) {
	d := New("main.go", "package main\n\nfunc main() {}\n")
	primary := &pane{}
	stopPrimary := primary.attach(d)
	split := &pane{}
	stopSplit := split.attach(d)

	// An edit typed in the primary pane.
	d.Replace(len("package main\n"), len("package main\n"), "\nimport \"fmt\"\n")
	// An edit typed in the split pane, at the very end.
	d.Replace(d.Len(), d.Len(), "\n// tail\n")
	// A whole-document rewrite (gofmt-on-save).
	d.SetText("package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(1) }\n")

	for name, p := range map[string]*pane{"primary": primary, "split": split} {
		if p.text != d.Text() {
			t.Errorf("%s pane text = %q, want %q", name, p.text, d.Text())
		}
		if p.events != 3 {
			t.Errorf("%s pane got %d events, want 3", name, p.events)
		}
		if p.rev != d.Revision() {
			t.Errorf("%s pane at revision %d, want %d", name, p.rev, d.Revision())
		}
	}

	// Closing the split pane must not stop the primary from tracking.
	stopSplit()
	d.Replace(0, 0, "// header\n")
	if split.events != 3 {
		t.Errorf("closed split pane still receiving events (%d)", split.events)
	}
	if primary.text != d.Text() {
		t.Errorf("primary pane text = %q, want %q", primary.text, d.Text())
	}
	stopPrimary()
}

// --- line helpers ----------------------------------------------------

func TestLineCount(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 1},
		{"a", 1},
		{"a\n", 2}, // trailing newline opens an empty last line
		{"a\nb", 2},
		{"a\nb\n", 3},
		{"\n\n", 3},
	}
	for _, c := range cases {
		if got := New("a.txt", c.text).LineCount(); got != c.want {
			t.Errorf("LineCount(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}

func TestLineStart(t *testing.T) {
	d := New("a.txt", "l0\nl1\nl2\n")
	cases := []struct{ line, want int }{
		{-3, 0},
		{0, 0},
		{1, 3},
		{2, 6},
		{3, 9}, // the empty line after the trailing newline
		{4, 9}, // past the end clamps to the document length
		{99, 9},
	}
	for _, c := range cases {
		if got := d.LineStart(c.line); got != c.want {
			t.Errorf("LineStart(%d) = %d, want %d", c.line, got, c.want)
		}
	}
	// No trailing newline: the last line still starts where it starts.
	if got := New("a.txt", "l0\nl1").LineStart(1); got != 3 {
		t.Errorf("LineStart(1) without trailing newline = %d, want 3", got)
	}
}

func TestLineOf(t *testing.T) {
	d := New("a.txt", "l0\nl1\nl2\n")
	cases := []struct{ offset, want int }{
		{-5, 0},
		{0, 0},
		{2, 0},
		{3, 1},
		{5, 1},
		{6, 2},
		{9, 3}, // end of text == start of the empty last line
		{99, 3},
	}
	for _, c := range cases {
		if got := d.LineOf(c.offset); got != c.want {
			t.Errorf("LineOf(%d) = %d, want %d", c.offset, got, c.want)
		}
	}
	// A newline belongs to the line it terminates.
	if got := d.LineOf(2); got != 0 {
		t.Errorf("LineOf(newline of line 0) = %d, want 0", got)
	}
}

// TestLineHelpersRoundTrip: LineOf(LineStart(n)) == n for every line.
func TestLineHelpersRoundTrip(t *testing.T) {
	d := New("a.txt", "alpha\n\nbeta\ngamma\n")
	for line := 0; line < d.LineCount(); line++ {
		if got := d.LineOf(d.LineStart(line)); got != line {
			t.Errorf("LineOf(LineStart(%d)) = %d, want %d", line, got, line)
		}
	}
}

// TestLineHelpersMultibyte: offsets are byte offsets, so multi-byte text
// must not shift the line arithmetic.
func TestLineHelpersMultibyte(t *testing.T) {
	d := New("a.txt", "书签\n断点\n")
	if got := d.LineCount(); got != 3 {
		t.Fatalf("LineCount() = %d, want 3", got)
	}
	if got := d.LineStart(1); got != 7 { // 2 runes x 3 bytes + '\n'
		t.Fatalf("LineStart(1) = %d, want 7", got)
	}
	if got := d.LineOf(7); got != 1 {
		t.Fatalf("LineOf(7) = %d, want 1", got)
	}
}

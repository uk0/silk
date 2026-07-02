package gui

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Blame (Annotate) API + pure helpers (no GL / no Draw)
//
// Annotations are keyed 0-based, matching cursorLine / breakpoints / diffMarkers.
// The editor never runs git; the host pushes the per-line "shorthash author"
// set. Placement is option (b): a dim, right-aligned column pinned to the text
// area's right edge, so gutterW / textOffX are unchanged whether blame is on or
// off. These tests exercise the state layer and the pure helpers only.
// ---------------------------------------------------------------------------

func TestCodeEditorSetBlameAnnotationsCopiesAndShows(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc\nd")

	if e.BlameVisible() {
		t.Fatalf("new editor should not show blame")
	}
	if got := e.BlameAnnotations(); len(got) != 0 {
		t.Fatalf("new editor should have no blame, got %v", got)
	}

	in := map[int]string{0: "a1b2c3 alice", 2: "d4e5f6 bob"}
	e.SetBlameAnnotations(in)

	if !e.BlameVisible() {
		t.Errorf("SetBlameAnnotations should turn the view on")
	}
	want := map[int]string{0: "a1b2c3 alice", 2: "d4e5f6 bob"}
	if got := e.BlameAnnotations(); !reflect.DeepEqual(got, want) {
		t.Errorf("BlameAnnotations() = %v, want %v", got, want)
	}

	// Mutating the input map after Set must not change internal state.
	in[0] = "zzzzzz eve"
	in[9] = "999999 mallory"
	if got := e.BlameAnnotations(); !reflect.DeepEqual(got, want) {
		t.Errorf("mutating input map leaked into editor: got %v, want %v", got, want)
	}

	// Mutating the returned map must not change internal state either.
	ret := e.BlameAnnotations()
	ret[2] = "clobbered"
	delete(ret, 0)
	if got := e.BlameAnnotations(); !reflect.DeepEqual(got, want) {
		t.Errorf("mutating returned map leaked into editor: got %v, want %v", got, want)
	}
}

func TestCodeEditorSetBlameAnnotationsNilShowsEmpty(t *testing.T) {
	e := NewCodeEditor()
	// nil still turns the view on, with an empty (all-blank) column.
	e.SetBlameAnnotations(nil)
	if !e.BlameVisible() {
		t.Errorf("SetBlameAnnotations(nil) should still turn the view on")
	}
	if got := e.BlameAnnotations(); len(got) != 0 {
		t.Errorf("SetBlameAnnotations(nil) should install an empty set, got %v", got)
	}
}

func TestCodeEditorClearBlame(t *testing.T) {
	e := NewCodeEditor()
	e.SetBlameAnnotations(map[int]string{0: "a1b2c3 alice", 3: "d4e5f6 bob"})
	if !e.BlameVisible() || len(e.BlameAnnotations()) != 2 {
		t.Fatalf("setup: want 2 annotations + visible, got visible=%v set=%v",
			e.BlameVisible(), e.BlameAnnotations())
	}

	e.ClearBlame()
	if e.BlameVisible() {
		t.Errorf("ClearBlame should hide the column (BlameVisible == false)")
	}
	if got := e.BlameAnnotations(); len(got) != 0 {
		t.Errorf("after ClearBlame, BlameAnnotations() = %v, want empty", got)
	}
}

func TestBlameColumnWidth(t *testing.T) {
	if got := blameColumnWidth(false); got != 0 {
		t.Errorf("blameColumnWidth(false) = %v, want 0 (blame off reserves no space)", got)
	}
	if got := blameColumnWidth(true); got != blameBaseColumnW {
		t.Errorf("blameColumnWidth(true) = %v, want %v", got, blameBaseColumnW)
	}
	if blameColumnWidth(true) <= 0 {
		t.Errorf("blameColumnWidth(true) must be positive")
	}
}

func TestTruncateBlame(t *testing.T) {
	cases := []struct {
		s    string
		max  int
		want string
	}{
		{"abc", 5, "abc"},            // within budget: unchanged
		{"abc", 3, "abc"},            // exactly at budget: unchanged
		{"abcdef", 4, "abc…"},        // over budget: last rune is the ellipsis
		{"abcdef", 1, "…"},           // 1 rune of room: just the ellipsis
		{"abcdef", 0, ""},            // no room
		{"abcdef", -3, ""},           // negative == no room
		{"", 5, ""},                  // empty stays empty
		{"héllo wörld", 6, "héllo…"}, // multi-byte safe (rune-based): 5 runes + ellipsis
	}
	for _, c := range cases {
		got := truncateBlame(c.s, c.max)
		if got != c.want {
			t.Errorf("truncateBlame(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
		}
		// Result must never exceed the rune budget.
		if c.max > 0 && len([]rune(got)) > c.max {
			t.Errorf("truncateBlame(%q, %d) = %q exceeds rune budget %d", c.s, c.max, got, c.max)
		}
	}
}

// TestCodeEditorBlameOffLayoutUnchanged pins the core requirement: turning blame
// on then off must leave the gutter / text geometry byte-identical. Because the
// blame column is drawn right-aligned (option b) it never reserves horizontal
// space, so gutterW is constant across every state and the text-start offset
// (gutterW+10, driven by a 0 column width) is unchanged whenever blame is off.
func TestCodeEditorBlameOffLayoutUnchanged(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("line one\nline two\nline three")
	baseGutter := e.gutterW

	// Blame off: the column contributes 0 width, so textOffX == gutterW+10-scroll.
	if w := blameColumnWidth(e.BlameVisible()); w != 0 {
		t.Fatalf("blame off: column width = %v, want 0", w)
	}

	e.SetBlameAnnotations(map[int]string{0: "a1b2c3 alice"})
	if e.gutterW != baseGutter {
		t.Errorf("blame on shifted gutterW: %v, want %v (option b never moves the gutter)", e.gutterW, baseGutter)
	}

	e.ClearBlame()
	if e.gutterW != baseGutter {
		t.Errorf("blame off shifted gutterW: %v, want %v", e.gutterW, baseGutter)
	}
	if w := blameColumnWidth(e.BlameVisible()); w != 0 {
		t.Errorf("after ClearBlame: column width = %v, want 0 (text layout must be unchanged)", w)
	}
}

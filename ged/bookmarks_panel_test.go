package ged

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"reflect"
	"strings"
	"testing"
)

// TestSortBookmarksStable verifies the pure sort helper orders by file
// then line, stably (rows sharing a (file,line) keep input order).
func TestSortBookmarksStable(t *testing.T) {
	in := []Bookmark{
		{File: "b.go", Line: 10, Label: "second-file"},
		{File: "a.go", Line: 30, Label: "a-30"},
		{File: "a.go", Line: 5, Label: "a-5"},
		{File: "a.go", Line: 5, Label: "a-5-dup"}, // same key as previous; must stay after it
		{File: "b.go", Line: 2, Label: "b-2"},
	}
	got := sortBookmarks(in)
	want := []Bookmark{
		{File: "a.go", Line: 5, Label: "a-5"},
		{File: "a.go", Line: 5, Label: "a-5-dup"},
		{File: "a.go", Line: 30, Label: "a-30"},
		{File: "b.go", Line: 2, Label: "b-2"},
		{File: "b.go", Line: 10, Label: "second-file"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortBookmarks = %+v\nwant %+v", got, want)
	}
	// Pure helper must not mutate its input.
	if in[0].File != "b.go" || in[1].File != "a.go" {
		t.Errorf("sortBookmarks mutated its input: %+v", in)
	}
}

// TestBookmarksAddSorted verifies Add accumulates entries and Bookmarks()
// returns them stably sorted by (file, line) regardless of insert order.
func TestBookmarksAddSorted(t *testing.T) {
	p := NewBookmarksPanel()
	p.Add("z.go", 9, "z9")
	p.Add("a.go", 20, "a20")
	p.Add("a.go", 3, "a3")

	got := p.Bookmarks()
	want := []Bookmark{
		{File: "a.go", Line: 3, Label: "a3"},
		{File: "a.go", Line: 20, Label: "a20"},
		{File: "z.go", Line: 9, Label: "z9"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Bookmarks() = %+v\nwant %+v", got, want)
	}
}

// TestBookmarksAddDedupUpdatesLabel verifies Add on an existing (file,line)
// updates the label rather than appending a duplicate row.
func TestBookmarksAddDedupUpdatesLabel(t *testing.T) {
	p := NewBookmarksPanel()
	p.Add("a.go", 10, "old label")
	p.Add("a.go", 10, "new label")

	got := p.Bookmarks()
	if len(got) != 1 {
		t.Fatalf("len(Bookmarks()) = %d, want 1 (dedup failed): %+v", len(got), got)
	}
	if got[0].Label != "new label" {
		t.Errorf("Label = %q, want %q (update failed)", got[0].Label, "new label")
	}
}

// TestBookmarksRemove verifies Remove deletes the matching (file,line)
// and is a no-op for an absent location.
func TestBookmarksRemove(t *testing.T) {
	p := NewBookmarksPanel()
	p.Add("a.go", 1, "one")
	p.Add("a.go", 2, "two")

	p.Remove("a.go", 99) // absent: no-op
	if len(p.Bookmarks()) != 2 {
		t.Fatalf("Remove of absent entry changed count: %+v", p.Bookmarks())
	}

	p.Remove("a.go", 1)
	got := p.Bookmarks()
	want := []Bookmark{{File: "a.go", Line: 2, Label: "two"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after Remove, Bookmarks() = %+v\nwant %+v", got, want)
	}
}

// TestBookmarksToggle verifies Toggle adds when absent and removes when
// present.
func TestBookmarksToggle(t *testing.T) {
	p := NewBookmarksPanel()

	p.Toggle("a.go", 5, "added") // absent -> add
	if len(p.Bookmarks()) != 1 {
		t.Fatalf("Toggle did not add: %+v", p.Bookmarks())
	}

	p.Toggle("a.go", 5, "ignored") // present -> remove
	if len(p.Bookmarks()) != 0 {
		t.Fatalf("Toggle did not remove: %+v", p.Bookmarks())
	}
}

// TestBookmarksClear verifies Clear empties the list.
func TestBookmarksClear(t *testing.T) {
	p := NewBookmarksPanel()
	p.Add("a.go", 1, "one")
	p.Add("b.go", 2, "two")
	p.Clear()
	if got := p.Bookmarks(); len(got) != 0 {
		t.Fatalf("after Clear, Bookmarks() = %+v, want empty", got)
	}
}

// TestBookmarksOnLeftDownActivates verifies clicking a row fires
// SigActivated with that row's (file, line). Rows are laid out below the
// 22px header at rowHeight each, in sorted order.
func TestBookmarksOnLeftDownActivates(t *testing.T) {
	p := NewBookmarksPanel()
	p.Add("a.go", 11, "first")
	p.Add("b.go", 22, "second")

	var (
		gotFile string
		gotLine int
		fired   bool
	)
	p.SigActivated(func(file string, line int) {
		gotFile = file
		gotLine = line
		fired = true
	})

	// Header is 22px; row 1 (the second sorted entry, b.go:22) sits at
	// y in [22+rowHeight, 22+2*rowHeight). Click into its middle.
	y := 22.0 + p.rowHeight + p.rowHeight/2
	p.OnLeftDown(5, y)

	if !fired {
		t.Fatal("OnLeftDown did not fire SigActivated")
	}
	if gotFile != "b.go" || gotLine != 22 {
		t.Errorf("SigActivated(%q, %d), want (%q, %d)", gotFile, gotLine, "b.go", 22)
	}
}

// TestBookmarksOnLeftDownHeaderNoop verifies a click in the header region
// does not activate any row.
func TestBookmarksOnLeftDownHeaderNoop(t *testing.T) {
	p := NewBookmarksPanel()
	p.Add("a.go", 1, "one")

	fired := false
	p.SigActivated(func(string, int) { fired = true })
	p.OnLeftDown(5, 5) // inside the 22px header
	if fired {
		t.Error("OnLeftDown in header region fired SigActivated")
	}
}

func TestBookmarksBatchSelectionKeyboardAndShrink(t *testing.T) {
	p := NewBookmarksPanel()
	p.SetBounds(0, 0, 240, 66)
	p.SetBookmarks([]Bookmark{{"a", 3, "last"}, {"a", 1, "first"}, {"a", 2, "second"}, {"", 0, "bad"}, {"a", 2, "updated"}})
	if len(p.Bookmarks()) != 3 || p.Bookmarks()[1].Label != "updated" {
		t.Fatal("batch validation")
	}
	p.OnKeyDown(gui.KeyEnd, false)
	if p.selectedIdx != 2 || p.scrollY != 22 {
		t.Fatal(p.selectedIdx, p.scrollY)
	}
	var activated, removed int
	p.SigActivated(func(_ string, line int) { activated = line })
	p.OnKeyDown(gui.KeyEnter, false)
	p.SigRemoveRequested(func(file string, line int) { removed = line; p.Remove(file, line) })
	p.OnKeyDown(gui.KeyDelete, false)
	if activated != 3 || removed != 3 || p.scrollY != 0 || p.selectedIdx != -1 {
		t.Fatal("keyboard or scroll", activated, removed, p.scrollY)
	}
	p.OnKeyDown(gui.KeyHome, false)
	p.SetBookmarks([]Bookmark{{"a", 0, "bad"}, {"a", 1, "renamed"}, {"a", 2, "second"}})
	if p.selectedIdx != 0 {
		t.Fatal("selection identity lost")
	}
	first, last := p.visibleRows(44)
	if first != 0 || last != 1 {
		t.Fatal("visible range", first, last)
	}
	p.Clear()
	p.OnKeyDown(gui.KeyDelete, false)
	if len(p.marks) != 0 || p.scrollY != 0 {
		t.Fatal("clear")
	}
}

func TestBookmarksPainterThemeFontAndClipping(t *testing.T) {
	mode, font := gui.CurrentThemeMode(), gui.Theme().Font
	t.Cleanup(func() { gui.SetThemeMode(mode); gui.Theme().Font = font })
	for _, theme := range []gui.ThemeMode{gui.ThemeLight, gui.ThemeDark} {
		gui.SetThemeMode(theme)
		for _, size := range []int{12, 24} {
			gui.Theme().Font = paint.NewFont(font.Family(), size, false, false)
			p := NewBookmarksPanel()
			p.SetSize(180, 160)
			var rows []Bookmark
			for i := 1; i <= 5000; i++ {
				rows = append(rows, Bookmark{"sample.txt", i, strings.Repeat("long ", 30)})
			}
			p.SetBookmarks(rows)
			p.selectedIdx = 1
			p.hoverIdx = 2
			p.scrollY = 5
			pix := paint.NewPixmap(180, 160)
			r := &explorerDrawRecorder{Painter: pix.NewPainter()}
			before := r.CurrentState()
			p.Draw(r)
			if r.CurrentState() != before || p.rowHeight < float64(size)+8 {
				t.Fatal("painter state/font")
			}
			if len(r.texts) > 9 {
				t.Fatal("offscreen drawing", len(r.texts))
			}
			for i, x := range r.texts {
				if x.x+x.width > 180.1 || i > 0 && x.cy <= 0 {
					t.Fatal("clipping", x)
				}
			}
			for _, want := range []paint.Color{gui.Theme().FormColor, gui.Theme().ViewBGColor, gui.Theme().TextColor, gui.Theme().HighLightColor, gui.Theme().MenuActiveTextColor, gui.Theme().ButtonHoverBGColor} {
				found := false
				for _, got := range r.colors {
					found = found || got == want
				}
				if !found {
					t.Fatal("missing theme color", want)
				}
			}
			if aboutContrast(gui.Theme().FormColor, gui.Theme().TextColor) < 4.5 || aboutContrast(gui.Theme().ButtonHoverBGColor, gui.Theme().TextColor) < 4.5 {
				t.Fatal("low contrast")
			}
		}
	}
}

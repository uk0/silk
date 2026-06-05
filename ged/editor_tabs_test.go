package ged

import (
	"os"
	"path/filepath"
	"testing"
)

// openTempTab creates a real file under t.TempDir() and opens it as a tab.
// EditorTabs.OpenFile reads the file from disk, so the content must exist.
func openTempTab(t *testing.T, et *EditorTabs, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	et.OpenFile(path)
	return path
}

// TestEditorTabsToggleSplit drives the public split API end to end:
// a freshly opened tab is not split; ToggleSplit turns it on; toggling
// again turns it off. This is the headless state contract -- it exercises
// no GL, only widget state and the GL-free Layout path.
func TestEditorTabsToggleSplit(t *testing.T) {
	et := NewEditorTabs()
	et.SetSize(400, 300) // gives Layout a non-zero area to work with

	openTempTab(t, et, "a.go", "package a\n")
	if et.TabCount() != 1 {
		t.Fatalf("TabCount = %d, want 1", et.TabCount())
	}

	if et.IsSplit() {
		t.Fatalf("IsSplit() = true right after open, want false")
	}

	et.ToggleSplit()
	if !et.IsSplit() {
		t.Fatalf("IsSplit() = false after first ToggleSplit, want true")
	}
	if et.splitEditor == nil {
		t.Fatalf("splitEditor is nil after ToggleSplit, want a second pane")
	}

	et.ToggleSplit()
	if et.IsSplit() {
		t.Fatalf("IsSplit() = true after second ToggleSplit, want false")
	}
	if et.splitEditor != nil {
		t.Fatalf("splitEditor not cleared after toggling split off")
	}
}

// TestEditorTabsToggleSplitNoTab: ToggleSplit is a no-op with no active tab.
// There is nothing to mirror into a second pane, so the widget stays single.
func TestEditorTabsToggleSplitNoTab(t *testing.T) {
	et := NewEditorTabs()
	et.SetSize(400, 300)

	et.ToggleSplit()
	if et.IsSplit() {
		t.Fatalf("IsSplit() = true with no open tab, want false")
	}
}

// TestEditorTabsSplitLaysOutBothPanes verifies the split actually produces
// two laid-out panes: with split on, both the active editor and the split
// editor get non-zero bounds, and the active pane sits left of the split
// pane (default side-by-side orientation).
func TestEditorTabsSplitLaysOutBothPanes(t *testing.T) {
	et := NewEditorTabs()
	et.SetSize(400, 300)
	openTempTab(t, et, "a.go", "package a\n")

	et.ToggleSplit()
	et.Layout()

	active := et.ActiveEditor()
	if active == nil {
		t.Fatal("ActiveEditor() = nil after split")
	}
	ax, _, aw, ah := active.Bounds()
	if aw <= 0 || ah <= 0 {
		t.Fatalf("primary pane has empty bounds: w=%v h=%v", aw, ah)
	}
	sx, _, sw, sh := et.splitEditor.Bounds()
	if sw <= 0 || sh <= 0 {
		t.Fatalf("secondary pane has empty bounds: w=%v h=%v", sw, sh)
	}
	if !(sx > ax) {
		t.Fatalf("secondary pane (x=%v) should be right of primary (x=%v)", sx, ax)
	}
}

// --- pure layout math: splitPaneRects ---

const splitEps = 1e-9

func feq(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < splitEps
}

// TestSplitPaneRectsVertical: side-by-side panes tile the width with exactly
// `gap` between them and the full height each.
func TestSplitPaneRectsVertical(t *testing.T) {
	const x, y, w, h, gap = 0.0, 30.0, 400.0, 270.0, 4.0
	p, s := splitPaneRects(x, y, w, h, true, 0.5, gap)

	if !feq(p.Width+s.Width+gap, w) {
		t.Errorf("widths+gap = %v, want %v", p.Width+s.Width+gap, w)
	}
	if !feq(p.Height, h) || !feq(s.Height, h) {
		t.Errorf("pane heights = %v/%v, want %v", p.Height, s.Height, h)
	}
	if !feq(p.X, x) {
		t.Errorf("primary X = %v, want %v", p.X, x)
	}
	if !feq(s.X, p.X+p.Width+gap) {
		t.Errorf("secondary X = %v, want %v (no overlap, gap respected)", s.X, p.X+p.Width+gap)
	}
	if !feq(p.Width, s.Width) {
		t.Errorf("ratio 0.5 should give equal widths: %v vs %v", p.Width, s.Width)
	}
}

// TestSplitPaneRectsHorizontal: stacked panes tile the height with exactly
// `gap` between them and the full width each.
func TestSplitPaneRectsHorizontal(t *testing.T) {
	const x, y, w, h, gap = 0.0, 30.0, 400.0, 270.0, 4.0
	p, s := splitPaneRects(x, y, w, h, false, 0.5, gap)

	if !feq(p.Height+s.Height+gap, h) {
		t.Errorf("heights+gap = %v, want %v", p.Height+s.Height+gap, h)
	}
	if !feq(p.Width, w) || !feq(s.Width, w) {
		t.Errorf("pane widths = %v/%v, want %v", p.Width, s.Width, w)
	}
	if !feq(p.Y, y) {
		t.Errorf("primary Y = %v, want %v", p.Y, y)
	}
	if !feq(s.Y, p.Y+p.Height+gap) {
		t.Errorf("secondary Y = %v, want %v (no overlap, gap respected)", s.Y, p.Y+p.Height+gap)
	}
}

// TestSplitPaneRectsRatio: a non-0.5 ratio shifts the divide proportionally
// (vertical orientation), while the gap stays fixed.
func TestSplitPaneRectsRatio(t *testing.T) {
	const x, y, w, h, gap = 0.0, 0.0, 300.0, 100.0, 4.0
	p, s := splitPaneRects(x, y, w, h, true, 0.25, gap)

	usable := w - gap
	if !feq(p.Width, usable*0.25) {
		t.Errorf("primary width = %v, want %v", p.Width, usable*0.25)
	}
	if !feq(p.Width+s.Width+gap, w) {
		t.Errorf("widths+gap = %v, want %v", p.Width+s.Width+gap, w)
	}
}

// TestSplitPaneRectsBadRatio: a ratio outside (0,1) falls back to an even
// split rather than producing a degenerate pane.
func TestSplitPaneRectsBadRatio(t *testing.T) {
	const x, y, w, h, gap = 0.0, 0.0, 200.0, 100.0, 4.0
	for _, r := range []float64{0, 1, -0.3, 1.5} {
		p, s := splitPaneRects(x, y, w, h, true, r, gap)
		if !feq(p.Width, s.Width) {
			t.Errorf("ratio %v: expected even split, got %v vs %v", r, p.Width, s.Width)
		}
	}
}

// NOTE: full split *rendering* (Draw) needs a live GL/Cairo context and is not
// unit-testable here. These tests cover what is testable headless: the split
// state machine (IsSplit/ToggleSplit), that both panes receive non-zero,
// non-overlapping bounds via the GL-free Layout path, and the pure pane-rect
// math (splitPaneRects).

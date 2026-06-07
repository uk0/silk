package gui

import (
	"testing"
)

// ---------------------------------------------------------------------------
// CodeEditor right-click context menu.
//
// OnRightDown shows a popup menu (Cut / Copy / Paste / Select All / Rename /
// Go to Definition / Find References) anchored at the click point. Because
// ShowContextMenu can't run without a live window, the per-entry decisions
// (label / enabled / action wiring) live in the pure-ish helper
// contextMenuItems, which is what these tests drive.
// ---------------------------------------------------------------------------

// findMenuItem returns the entry with the given label, or nil if absent.
func findMenuItem(items []editorContextMenuItem, label string) *editorContextMenuItem {
	for i := range items {
		if items[i].Label == label {
			return &items[i]
		}
	}
	return nil
}

func TestContextMenuItemsLabels(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("package p\n")

	items := e.contextMenuItems(false, false, "")

	// The seven action labels plus one separator after Select All. We don't
	// pin a strict ordering test beyond "the canonical entries are present"
	// because future maintainers may insert further entries.
	wantLabels := []string{"剪切", "复制", "粘贴", "全选", "重命名符号", "跳转定义", "查找引用"}
	for _, lbl := range wantLabels {
		if findMenuItem(items, lbl) == nil {
			t.Errorf("context menu missing entry %q", lbl)
		}
	}

	// At least one separator must be present (between the clipboard block
	// and the symbol-action block).
	sepCount := 0
	for _, it := range items {
		if it.Separator {
			sepCount++
		}
	}
	if sepCount == 0 {
		t.Errorf("expected at least one separator in context menu, got 0")
	}
}

func TestContextMenuItemsDisabledWithoutSelection(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("package p\nfunc Foo() {}\n")
	e.cursorLine = 1
	e.cursorCol = 5 // inside "Foo"

	// No selection, no clipboard, but a word is at the cursor.
	items := e.contextMenuItems(false, false, e.wordAtCursor())

	if cut := findMenuItem(items, "剪切"); cut == nil || cut.Enabled {
		t.Errorf("剪切 should be disabled without a selection: %+v", cut)
	}
	if cp := findMenuItem(items, "复制"); cp == nil || cp.Enabled {
		t.Errorf("复制 should be disabled without a selection: %+v", cp)
	}
	if pst := findMenuItem(items, "粘贴"); pst == nil || pst.Enabled {
		t.Errorf("粘贴 should be disabled without clipboard text: %+v", pst)
	}
	if all := findMenuItem(items, "全选"); all == nil || !all.Enabled {
		t.Errorf("全选 should always be enabled: %+v", all)
	}
	if rn := findMenuItem(items, "重命名符号"); rn == nil || !rn.Enabled {
		t.Errorf("重命名符号 should be enabled when a word sits at the cursor: %+v", rn)
	}
}

func TestContextMenuItemsEnabledWithSelection(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("package p\nfunc Foo() {}\n")

	items := e.contextMenuItems(true, true, "Foo")

	if cut := findMenuItem(items, "剪切"); cut == nil || !cut.Enabled {
		t.Errorf("剪切 should be enabled with a selection: %+v", cut)
	}
	if cp := findMenuItem(items, "复制"); cp == nil || !cp.Enabled {
		t.Errorf("复制 should be enabled with a selection: %+v", cp)
	}
	if pst := findMenuItem(items, "粘贴"); pst == nil || !pst.Enabled {
		t.Errorf("粘贴 should be enabled when clipboard has text: %+v", pst)
	}
}

func TestContextMenuItemsRenameDisabledOnEmptyWord(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("package p\n")

	items := e.contextMenuItems(false, false, "")

	if rn := findMenuItem(items, "重命名符号"); rn == nil || rn.Enabled {
		t.Errorf("重命名符号 should be disabled on empty word: %+v", rn)
	}
	if goTo := findMenuItem(items, "跳转定义"); goTo == nil || goTo.Enabled {
		t.Errorf("跳转定义 should be disabled on empty word: %+v", goTo)
	}
	if ref := findMenuItem(items, "查找引用"); ref == nil || ref.Enabled {
		t.Errorf("查找引用 should be disabled on empty word: %+v", ref)
	}
}

// TestContextMenuActionsWire verifies the Action closures actually invoke
// the underlying editor methods. We hit editorSelectAll via the menu and
// confirm a selection appeared — that exercises the full label -> action
// wiring without needing a live popup.
func TestContextMenuActionsWire(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("alpha\nbeta\ngamma")
	items := e.contextMenuItems(false, false, "")

	all := findMenuItem(items, "全选")
	if all == nil || all.Action == nil {
		t.Fatalf("全选 missing Action: %+v", all)
	}
	all.Action()
	if !e.HasSelection() {
		t.Errorf("全选 action should create a selection across the buffer")
	}
}

// TestOnRightDownMovesCursor checks that right-clicking inside the text area
// repositions the caret to the click point, matching Qt Creator behaviour.
// We replicate the click-Y math from posFromXY (see codeeditor_breakpoint_test.go)
// so the test stays robust across font-metric differences.
func TestOnRightDownMovesCursor(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(400, 300)
	e.SetText("line0\nline1\nline2\nline3\nline4")

	lh := e.font.FontExtents().Height + 2
	topOff := e.topOffset()
	yForLine := func(line int) float64 {
		return topOff + float64(line)*lh + lh/2
	}
	// Sanity: posFromXY resolves the chosen Y back to the intended line.
	if got, _ := e.posFromXY(5, yForLine(2)); got != 2 {
		t.Fatalf("posFromXY resolved line %d, want 2 (font metrics mismatch)", got)
	}

	// Right-click inside the text area (x past the gutter) on line 2 should
	// move the cursor there. OnRightDown also invokes ShowContextMenu, which
	// is a no-op when no items are added; here items ARE added, so the popup
	// would attempt to draw — but with no parent window the popup logic
	// falls through harmlessly (NewPopupMenu / MapToGlobal don't require a
	// live window for our purposes). We only assert the caret moved.
	textX := e.gutterW + 20
	e.cursorLine = 0
	e.cursorCol = 0
	e.OnRightDown(textX, yForLine(2))
	if e.cursorLine != 2 {
		t.Errorf("OnRightDown should move cursor to line 2, got %d", e.cursorLine)
	}
}

// TestOnRightDownPreservesSelectionWhenClickingInside verifies the IDE-canonical
// behaviour: right-clicking inside an existing selection leaves the selection
// alone so Cut / Copy operate on it; right-clicking outside collapses the
// selection to a caret at the click point.
func TestOnRightDownPreservesSelectionWhenClickingInside(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(400, 300)
	e.SetText("line0\nline1\nline2\nline3\nline4")

	lh := e.font.FontExtents().Height + 2
	topOff := e.topOffset()
	yForLine := func(line int) float64 {
		return topOff + float64(line)*lh + lh/2
	}

	// Select line 1 entirely.
	e.selStartLine = 1
	e.selStartCol = 0
	e.selEndLine = 1
	e.selEndCol = 5
	e.hasSelection = true
	e.cursorLine = 1
	e.cursorCol = 5

	// Right-click inside the selection (line 1, mid-text) keeps the selection.
	textX := e.gutterW + 20
	e.OnRightDown(textX, yForLine(1))
	if !e.HasSelection() {
		t.Errorf("right-click inside selection should preserve it")
	}

	// Right-click outside the selection collapses to caret at the click point.
	e.OnRightDown(textX, yForLine(3))
	if e.HasSelection() {
		t.Errorf("right-click outside selection should collapse it")
	}
	if e.cursorLine != 3 {
		t.Errorf("cursor should move to line 3 after outside right-click, got %d", e.cursorLine)
	}
}

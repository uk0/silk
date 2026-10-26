package gui

import (
	"fmt"
	"math"
	"testing"
)

func TestCodeEditorKeyboardContextAnchorWithinViewport(t *testing.T) {
	for _, size := range [][2]float64{{400, 240}, {20, 10}, {1, 1}, {0, 0}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			e, _ := editorContextHarness(t)
			e.SetSize(size[0], size[1])
			e.SetFocus()
			e.SelectAll()
			before := e.selectionSnapshot()
			text, revision := e.Text(), e.TextRevision()
			calls := 0
			showEditorContextPopup = func(_ *Menu, x, y float64) {
				calls++
				if math.IsNaN(x) || math.IsNaN(y) || x < 0 || y < 0 || x > size[0] || y > size[1] {
					t.Errorf("keyboard popup outside editor: %v,%v for %v", x, y, size)
				}
			}
			if !e.editorContextKeyWithMods(KeyF10, false, ModShift) || calls != 1 {
				t.Fatal("keyboard menu not opened once")
			}
			if e.Text() != text || e.TextRevision() != revision || e.selectionSnapshot() != before {
				t.Fatal("anchor query mutated editor")
			}
		})
	}
}

func TestEditKeyboardContextAnchorsCaretWithoutChangingSelection(t *testing.T) {
	for _, size := range [][2]float64{{400, 240}, {20, 10}, {1, 1}, {0, 0}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			e, _ := editMenuTest(t)
			e.SetMultiLine(true)
			e.SetSize(size[0], size[1])
			e.SetText("first\n中🙂 last")
			e.SetSelection(2, 10)
			e.SetFocus()
			a, b := e.Selection()
			text, revision := e.Text(), e.editRevision
			scrollX, scrollY := e.ScrollPos()
			caret := e.InputMethodCursorRect()
			calls := 0
			showEditContextPopup = func(_ *Menu, x, y float64) {
				calls++
				if math.Abs(x-caret.X) > .01 || math.Abs(y-caret.Bottom()) > .01 || x < 0 || y < 0 || x > size[0] || y > size[1] {
					t.Errorf("keyboard popup not at clamped caret: %v,%v for %+v size %v", x, y, caret, size)
				}
			}
			if !e.editContextKeyWithMods(KeyF10, false, ModShift) || calls != 1 {
				t.Fatal("keyboard menu not opened once")
			}
			if !e.editContextKeyWithMods(KeyF10, true, ModShift) || calls != 1 {
				t.Fatal("key repeat reopened menu")
			}
			for _, mods := range []uint8{0, ModAction, ModAlt, ModShift | ModAction} {
				if e.editContextKeyWithMods(KeyF10, false, mods) {
					t.Fatal("unrelated chord claimed")
				}
			}
			gotA, gotB := e.Selection()
			sx, sy := e.ScrollPos()
			if e.Text() != text || e.editRevision != revision || gotA != a || gotB != b || sx != scrollX || sy != scrollY {
				t.Fatal("keyboard menu mutated document/selection/scroll")
			}
			focusWidget = NewEdit()
			if e.editContextKeyWithMods(KeyF10, false, ModShift) {
				t.Fatal("unfocused field claimed keyboard menu")
			}
		})
	}
}

//go:build !windows

package gui

import (
	"fmt"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestCodeEditorDocumentBoundaryKeys(t *testing.T) {
	for _, mods := range editActionTestModifiers() {
		t.Run(fmt.Sprint(mods), func(t *testing.T) {
			withEditActionModifiers(t, mods)
			e := NewCodeEditor()
			e.SetText("first\n中间\n尾😀")
			e.cursorLine, e.cursorCol = 1, 1
			e.OnKeyDown(KeyEnd, false)
			if e.cursorLine != 2 || e.cursorCol != 2 {
				t.Fatalf("action+End stopped at line end: %d:%d", e.cursorLine, e.cursorCol)
			}
			shift := mods | glfw.ModShift
			eventModifiers = &shift
			e.OnKeyDown(KeyHome, false)
			if e.cursorLine != 0 || e.cursorCol != 0 || e.SelectedText() != e.Text() {
				t.Fatal("action+Shift+Home did not extend selection to document start")
			}
			eventModifiers = &mods
			e.OnKeyDown(KeyEnd, false)
			if e.HasSelection() || e.cursorLine != 2 || e.cursorCol != 2 {
				t.Fatal("unshifted document motion did not clear selection")
			}
		})
	}
}

//go:build windows && !silk_pure_go

package gui

import "testing"

func TestWindowsDefaultEditorFontIsReallyMonospace(t *testing.T) {
	editor := NewCodeEditor()
	editor.ensureFontMetrics()
	if editor.font.Family() != "Consolas" || !editor.monoFont {
		t.Fatalf("Windows editor font is not resolved as monospace: family=%q width=%v", editor.font.Family(), editor.charWidth)
	}
}

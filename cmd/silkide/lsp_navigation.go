package main

import (
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// Keep wire coordinates intact until the actual target buffer is available.
// Looking up disk text first is wrong when the destination has unsaved edits.
func openFileInEditorAtLSP(tabs *gui.TabWidget, path string, position core.LSPPosition) {
	if path == "" || !openFileInEditor(tabs, path) {
		return
	}
	if ed := openEditors[path]; ed != nil {
		ed.ScrollToUTF16Position(position.Line, position.Character)
	}
}

func lspReferenceLocation(path string, position core.LSPPosition, sourceLine string) ged.ReferenceLoc {
	return ged.ReferenceLoc{
		File: path, Line: position.Line + 1,
		Col:     gui.RuneColumnFromUTF16(sourceLine, position.Character),
		Preview: strings.TrimSpace(sourceLine),
	}
}

// Diagnostics arrive off-thread. Resolve columns only after their session and
// version guards pass on the UI thread, using the same live buffer as squiggles.
// An unavailable line has no trustworthy column: navigate to its start instead
// of accidentally interpreting a UTF-16 offset as a rune column.
func normalizeLSPProblemColumns(problems []ged.Problem, editor *gui.CodeEditor) {
	var lines []string
	if editor != nil {
		lines = editor.Lines()
	}
	for i := range problems {
		line := problems[i].Line - 1
		if line >= 0 && line < len(lines) {
			problems[i].Col = gui.RuneColumnFromUTF16(lines[line], problems[i].Col)
		} else {
			problems[i].Col = 0
		}
	}
}

package main

import "github.com/uk0/silk/gui"

// Captured and checked on the UI thread; workers never inspect mutable widgets.
type editorHintSnapshot struct {
	editor                            *gui.CodeEditor
	path                              string
	textRevision, interactionRevision uint64
}

func captureEditorHint(ed *gui.CodeEditor, path string) editorHintSnapshot {
	return editorHintSnapshot{ed, path, ed.TextRevision(), ed.LanguageHintRevision()}
}

func (s editorHintSnapshot) current() bool {
	return s.editor != nil && openEditors[s.path] == s.editor &&
		s.editor.FilePath() == s.path && !fileRenameAffects(s.path) &&
		s.editor.TextRevision() == s.textRevision &&
		s.editor.LanguageHintRevision() == s.interactionRevision &&
		s.editor.IsVisible() && s.editor.IsAllAncentorsVisible()
}

func utf16ColForRune(ed *gui.CodeEditor, line, col int) int {
	return ed.UTF16ColumnAt(line, col)
}

package gui

import (
	"path/filepath"
	"strings"
)

// Language returns the built-in document dialect inferred from FilePath:
// "go", "silkgo", "json", or "plaintext". An unnamed SDK editor retains Go as its
// historical default. Unknown named formats are plain text, not pretend Go.
// This is a UI-thread accessor; external language services remain host-owned.
func (e *CodeEditor) Language() string { return editorLanguageForPath(e.filePath) }

func editorLanguageForPath(path string) string {
	if path == "" || strings.EqualFold(filepath.Ext(path), ".go") {
		return "go"
	}
	if strings.EqualFold(filepath.Ext(path), ".silkgo") {
		return "silkgo"
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return "json"
	}
	return "plaintext"
}

func (e *CodeEditor) usesGoSyntax() bool {
	return goSyntaxLanguage(e.Language())
}

func goSyntaxLanguage(language string) bool {
	return language == "go" || language == "silkgo"
}

func (e *CodeEditor) languageLabel() string {
	switch e.Language() {
	case "go":
		return "Go"
	case "silkgo":
		return "SilkGo"
	case "json":
		return "JSON"
	default:
		return "Plain Text"
	}
}

// Switching document identity must not reuse syntax, symbols or asynchronous
// completions from the old dialect, even when the source revision is unchanged.
func (e *CodeEditor) resetDocumentLanguage() {
	e.CancelCompletion()
	e.CancelSymbolIndex()
	e.ClearExternalCompletions()
	e.DismissLanguageHints()
	e.clearTokenCache()
	e.updateGoLineStates()
	e.symbolCache = codeSymbolCache{}
	e.additionalSymbols = nil
	e.syntaxHighlights = nil
	e.snippetSess = nil
	e.foldedLines = make(map[int]bool)
	e.invalidateFoldCache()
	e.symbolsRevision++
	e.notifySymbolsChanged()
}

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func TestEditorSymbolHostActionsTargetOwnerNotCurrentTab(t *testing.T) {
	tabs := gui.NewTabWidget()
	a, b := gui.NewCodeEditor(), gui.NewCodeEditor()
	a.SetText("target")
	b.SetText("other")
	tabs.AddTab(a, "a.go", nil)
	tabs.AddTab(b, "b.go", nil)
	var calls []string
	handler := func(name string) func(*gui.TabWidget) {
		return func(received *gui.TabWidget) {
			if received != tabs || activeEditor(tabs) != a || !a.HasFocus() {
				t.Fatal("context command targeted the wrong editor")
			}
			calls = append(calls, name)
		}
	}
	a.SetSymbolActions(editorSymbolActions(tabs, a, handler("rename"), handler("definition"), handler("references")))
	for i, action := range []func(){a.RequestRenameSymbol, a.RequestGoToDefinition, a.RequestFindReferences} {
		tabs.SetCurrentIndex(1)
		action()
		if len(calls) != i+1 || calls[i] != []string{"rename", "definition", "references"}[i] {
			t.Fatalf("bad host dispatch: %v", calls)
		}
	}
	// Closing a menu's owner while another tab remains must not invoke the
	// semantic action against that remaining file.
	tabs.RemoveTab(0)
	a.RequestRenameSymbol()
	a.RequestGoToDefinition()
	a.RequestFindReferences()
	if len(calls) != 3 || activeEditor(tabs) != b {
		t.Fatal("stale context action changed a different document")
	}
}

func TestOpenedEditorContextCommandsReportMissingLSPWithoutLocalFallback(t *testing.T) {
	oldEditors, oldLSP, oldToast := openEditors, globalLSP, silkideToast
	t.Cleanup(func() { openEditors, globalLSP, silkideToast = oldEditors, oldLSP, oldToast })
	openEditors, globalLSP = make(map[string]*gui.CodeEditor), nil
	var messages []string
	silkideToast = func(message string, level gui.ToastLevel) {
		if level != gui.ToastWarning {
			t.Fatalf("missing LSP reported as %v", level)
		}
		messages = append(messages, message)
	}
	path := filepath.Join(t.TempDir(), "source.go")
	source := "package p\nfunc target() {}\nfunc caller() { target() }\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	tabs := gui.NewTabWidget()
	if !openFileInEditor(tabs, path) {
		t.Fatal("openFileInEditor failed")
	}
	ed := activeEditor(tabs)
	ed.ScrollToLineCol(2, 18)
	for _, action := range []func(){ed.RequestRenameSymbol, ed.RequestGoToDefinition, ed.RequestFindReferences} {
		before := len(messages)
		action() // Rename must return before opening any native input dialog.
		if len(messages) != before+1 || messages[before] != "[SILK-LSP001] "+i18n.T("LSP not running") {
			t.Fatalf("missing-LSP command was silent: %v", messages)
		}
		if ed.Text() != source || ed.CursorLine() != 2 || ed.CursorCol() != 18 || ed.Modified() {
			t.Fatal("missing LSP silently ran a local editor operation")
		}
	}
	// The F2 entry point uses exactly the same unavailable-service guard.
	renameSymbolAtActiveEditor(tabs)
	if len(messages) != 4 || messages[3] != "[SILK-LSP001] "+i18n.T("LSP not running") {
		t.Fatal("F2 rename differs from context rename")
	}
}

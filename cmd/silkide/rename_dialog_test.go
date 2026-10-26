package main

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"path/filepath"
	"testing"
)

func TestRenameDialogIDEPrefillValidationAndStaleGuard(t *testing.T) {
	oldLSP, oldEditors, oldDocs := globalLSP, openEditors, lspDocuments
	oldPrompt, oldSubmit, oldToast := showSymbolRenameInput, submitSymbolRename, silkideToast
	t.Cleanup(func() {
		globalLSP, openEditors, lspDocuments = oldLSP, oldEditors, oldDocs
		showSymbolRenameInput, submitSymbolRename, silkideToast = oldPrompt, oldSubmit, oldToast
	})
	for _, kind := range []string{"valid", "cancel", "same", "invalid", "edit_undo", "caret", "readonly", "disabled", "path", "parent", "removed", "service", "document"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.go")
			e := gui.NewCodeEditor()
			defer e.CancelCompletion()
			e.SetFilePath(path)
			source := "package p\nfunc oldName(){}\nfunc use(){/*🙂*/oldName()}\n"
			e.SetText(source)
			e.ScrollToLineCol(2, 19)
			tabs := gui.NewTabWidget()
			tabs.AddTab(e, "source.go", nil)
			openEditors = map[string]*gui.CodeEditor{path: e}
			lspDocuments = map[string]*lspDocument{}
			globalLSP = &core.LSPClient{}
			calls, prompts := 0, 0
			silkideToast = func(string, gui.ToastLevel) {}
			submitSymbolRename = func(got *gui.CodeEditor, p, n string) {
				if got != e || p != path || n != "newName" {
					t.Fatal("retargeted rename")
				}
				calls++
			}
			showSymbolRenameInput = func(parent gui.IWidget, icon paint.Icon, title, label, def string, v gui.Validator) (string, bool) {
				prompts++
				if def != "oldName" || v.Validate("for") == gui.Acceptable {
					t.Fatal("missing prefill/validation", def)
				}
				switch kind {
				case "cancel":
					return "newName", false
				case "same":
					return def, true
				case "invalid":
					return "for", true
				case "edit_undo":
					e.ReplaceAllText(source + "\n")
					e.Undo()
				case "caret":
					e.ScrollToLineCol(1, 6)
				case "readonly":
					e.SetReadOnly(true)
				case "disabled":
					e.SetEnabled(false)
				case "path":
					e.SetFilePath(path + ".other")
				case "parent":
					e.SetParent(gui.NewForm())
				case "removed":
					delete(openEditors, path)
				case "service":
					globalLSP = &core.LSPClient{}
				case "document":
					lspDocuments[path] = &lspDocument{}
				}
				return "newName", true
			}
			renameSymbolAtActiveEditor(tabs)
			if prompts != 1 {
				t.Fatal("prompt missing", prompts)
			}
			want := 0
			if kind == "valid" {
				want = 1
			}
			if calls != want {
				t.Fatal("wrong dispatch count", kind, calls, want)
			}
			if e.Text() != source {
				t.Fatal("prompt mutated source")
			}
		})
	}
}

func TestRenameDialogRPCGuardAllowsNavigationNotDocumentReuse(t *testing.T) {
	old := openEditors
	t.Cleanup(func() { openEditors = old })
	e := gui.NewCodeEditor()
	e.SetFilePath("source.go")
	e.SetText("word\nnext")
	openEditors = map[string]*gui.CodeEditor{"source.go": e}
	guard := renameEditorGuard(e, "source.go", false)
	e.ScrollToLineCol(1, 2)
	if !guard() {
		t.Fatal("accepted request was lost to navigation")
	}
	e.ReplaceAllText("other")
	e.Undo()
	if guard() {
		t.Fatal("edit/undo ABA accepted")
	}
}

func TestRenameDialogAllOpenTargetsValidatedBeforeApply(t *testing.T) {
	old, oldDocs := openEditors, lspDocuments
	t.Cleanup(func() { openEditors, lspDocuments = old, oldDocs })
	for _, kind := range []string{"stable", "readonly", "edit_undo", "closed", "replaced", "opened_late"} {
		t.Run(kind, func(t *testing.T) {
			pathA, pathB := filepath.Join(t.TempDir(), "a.go"), filepath.Join(t.TempDir(), "b.go")
			a, b := gui.NewCodeEditor(), gui.NewCodeEditor()
			a.SetFilePath(pathA)
			b.SetFilePath(pathB)
			a.SetText("a")
			b.SetText("b")
			openEditors = map[string]*gui.CodeEditor{pathA: a, pathB: b}
			lspDocuments = map[string]*lspDocument{}
			if kind == "opened_late" {
				delete(openEditors, pathB)
			}
			snap := captureRenameBuffers()
			switch kind {
			case "readonly":
				b.SetReadOnly(true)
			case "edit_undo":
				b.ReplaceAllText("changed")
				b.Undo()
			case "closed":
				delete(openEditors, pathB)
			case "replaced":
				openEditors[pathB] = gui.NewCodeEditor()
			case "opened_late":
				openEditors[pathB] = b
			}
			edit := &core.LSPWorkspaceEdit{Changes: map[string][]core.LSPTextEdit{fileURIOf(pathA): nil, fileURIOf(pathB): nil}}
			if got := renameTargetsCurrent(edit, snap); got != (kind == "stable") {
				t.Fatal("target guard", kind, got)
			}
			if a.Text() != "a" || b.Text() != "b" {
				t.Fatal("guard mutated buffers")
			}
		})
	}
}

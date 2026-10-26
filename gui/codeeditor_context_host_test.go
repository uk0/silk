package gui

import "testing"

func TestCodeEditorContextSymbolActionsReachTheirHost(t *testing.T) {
	ed := NewCodeEditor()
	ed.SetText("package p\nfunc target() {}\nfunc caller() { target() }\n")
	ed.cursorLine, ed.cursorCol = 2, 18
	var calls []string
	host := func(name string) func(*CodeEditor) {
		return func(sender *CodeEditor) {
			if sender != ed {
				t.Fatal("context action lost its owning editor")
			}
			calls = append(calls, name)
		}
	}
	ed.SetSymbolActions(CodeEditorSymbolActions{
		RenameSymbol: host("rename"), GoToDefinition: host("definition"), FindReferences: host("references"),
	})
	items := ed.contextMenuItems(ed.wordAtCursor())
	for i, label := range []string{"Rename Symbol", "Go to Definition", "Find References"} {
		item := findMenuItem(items, label)
		if item == nil || !item.Enabled || item.Action == nil {
			t.Fatalf("missing host action %q", label)
		}
		item.Action()
		want := []string{"rename", "definition", "references"}[i]
		if len(calls) != i+1 || calls[i] != want {
			t.Fatalf("%s routed as %v", label, calls)
		}
	}
	if ed.cursorLine != 2 || len(ed.findMatches) != 0 || ed.renameActive {
		t.Fatal("host-handled symbol command also ran the SDK's local fallback")
	}
	// Raw command specs dispatch through current hooks. Actual built menu rows
	// also have a snapshot guard and are retired when host hooks are replaced.
	ed.SetSymbolActions(CodeEditorSymbolActions{GoToDefinition: host("replacement")})
	findMenuItem(items, "Go to Definition").Action()
	if calls[len(calls)-1] != "replacement" {
		t.Fatal("menu retained an obsolete host handler")
	}
}

func TestCodeEditorContextDefaultAndPartialOverridesKeepSDKBehavior(t *testing.T) {
	for _, configured := range []bool{false, true} {
		ed := NewCodeEditor()
		ed.SetText("package p\nfunc target() {}\nfunc caller() { target() }\n")
		ed.cursorLine, ed.cursorCol = 2, 18
		if configured {
			ed.SetSymbolActions(CodeEditorSymbolActions{RenameSymbol: func(*CodeEditor) { t.Fatal("unrelated hook invoked") }})
		}
		findMenuItem(ed.contextMenuItems(ed.wordAtCursor()), "Go to Definition").Action()
		if ed.cursorLine != 1 {
			t.Fatal("default SDK definition navigation was lost")
		}
		ed.cursorLine, ed.cursorCol = 2, 18
		findMenuItem(ed.contextMenuItems(ed.wordAtCursor()), "Find References").Action()
		if len(ed.findMatches) != 2 {
			t.Fatalf("default SDK references were lost: %v", ed.findMatches)
		}
		// Passing the empty configuration explicitly uninstalls host hooks.
		ed.SetSymbolActions(CodeEditorSymbolActions{})
		old, count, err := ed.RenameSymbolAtCursor("renamed")
		if err != nil || old != "target" || count != 2 {
			t.Fatalf("SDK local rename changed: %q %d %v", old, count, err)
		}
	}
}

func TestCodeEditorContextSymbolActionsRemainDisabledOnWhitespace(t *testing.T) {
	ed := NewCodeEditor()
	ed.SetText("\n")
	fail := func(*CodeEditor) { t.Fatal("disabled symbol command invoked its host") }
	ed.SetSymbolActions(CodeEditorSymbolActions{RenameSymbol: fail, GoToDefinition: fail, FindReferences: fail})
	for _, label := range []string{"Rename Symbol", "Go to Definition", "Find References"} {
		item := findMenuItem(ed.contextMenuItems(""), label)
		if item.Enabled {
			t.Fatalf("%s enabled without a symbol", label)
		}
		item.Action() // Even a stale/direct callback must not bypass the guard.
	}
}

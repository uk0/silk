package gui

import "testing"

func TestEditorClaimsWindowTab(t *testing.T) {
	e := NewCodeEditor()
	e.filePath = "notes.txt"
	e.SetSize(800, 500)
	e.SetText("iferr\n")
	e.ScrollToLineCol(0, 5)
	claim, ok := any(e).(IWantsTab)
	if !ok {
		t.Fatal("window consumes Tab before editor OnKeyDown: missing IWantsTab")
	}
	if !claim.WantsTab(false) || e.Text() != "iferr\t\n" {
		t.Fatal("plain Tab did not insert exactly one literal tab", e.Text())
	}
	e.Undo()
	if e.Text() != "iferr\n" {
		t.Fatal("Tab undo failed", e.Text())
	}
	e.Redo()
	if e.Text() != "iferr\t\n" {
		t.Fatal("Tab redo failed", e.Text())
	}
	// Find/replace is editor-owned keyboard UI, not a reason to jump panels.
	e.findActive, e.replaceVisible = true, true
	before := e.Text()
	if !claim.WantsTab(false) || !e.replaceFocused || e.Text() != before {
		t.Fatal("Tab did not move from find to replace without editing")
	}
}

func TestEditorClaimedTabKeepsGoSnippets(t *testing.T) {
	e := NewCodeEditor()
	e.SetSize(800, 500)
	e.SetText("iferr")
	e.ScrollToLineCol(0, 5)
	claim, ok := any(e).(IWantsTab)
	if !ok {
		t.Fatal("editor does not claim Tab")
	}
	if !claim.WantsTab(false) || e.Text() == "iferr" || e.snippetSess == nil {
		t.Fatal("Go snippet did not expand", e.Text())
	}
	snippets := NewGoSnippetSet()
	snippets.add(&Snippet{Trigger: "fields", Body: "${1:name} ${2:value}$0"})
	e.SetSnippets(snippets)
	e.SetText("fields")
	e.ScrollToLineCol(0, 6)
	claim.WantsTab(false)
	before := e.Text()
	if e.SelectedText() != "name" {
		t.Fatal("first snippet stop was not selected", e.SelectedText())
	}
	claim.WantsTab(false)
	if e.SelectedText() != "value" || e.Text() != before {
		t.Fatal("Tab did not navigate to the second snippet stop")
	}
	claim.WantsTab(true)
	if e.Text() != before || e.SelectedText() != "name" {
		t.Fatal("Shift+Tab did not return to the first snippet stop")
	}
}

func TestEditorClaimedShiftTabDedentsSelection(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("\tone\n\ttwo")
	e.editorSelectAll()
	e.WantsTab(true)
	if e.Text() != "one\ntwo" {
		t.Fatal("claimed Shift+Tab failed to dedent", e.Text())
	}
}

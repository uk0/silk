package ged

import (
	"github.com/uk0/silk/gui"
	"testing"
)

func TestGeneratedViewRejectsInheritedDocumentCommands(t *testing.T) {
	for name, run := range map[string]func(*gui.CodeEditor){
		"move":      (*gui.CodeEditor).MoveLinesDown,
		"duplicate": (*gui.CodeEditor).DuplicateLines,
		"delete":    (*gui.CodeEditor).DeleteLines,
		"indent":    (*gui.CodeEditor).IndentLines,
		"comment":   (*gui.CodeEditor).ToggleLineComment,
		"join":      (*gui.CodeEditor).JoinLines,
		"replace":   func(e *gui.CodeEditor) { e.ReplaceAllText("unexpected") },
		"selection": func(e *gui.CodeEditor) { e.SelectAll(); e.ReplaceSelection("unexpected") },
	} {
		t.Run(name, func(t *testing.T) {
			v := newGeneratedView()
			const source = "// generated\npackage demo\n"
			v.SetText(source)
			revision := v.TextRevision()
			run(&v.CodeEditor)
			if v.Text() != source || v.TextRevision() != revision || v.CanUndo() {
				t.Fatal("generated source was editable through inherited SDK command", v.Text())
			}
		})
	}
}

func TestGeneratedViewHostRefreshAndReaderCommands(t *testing.T) {
	v := newGeneratedView()
	v.SetText("generated one\nsecond\n")
	if !v.IsReadOnly() {
		t.Fatal("generated view is not locked")
	}
	v.SelectAll()
	if !v.CanCopy() || v.CanCut() || v.CanPaste() || v.SelectedText() != v.Text() {
		t.Fatal("reader command state")
	}
	v.SetText("generated two\n")
	if !v.IsReadOnly() || v.Text() != "generated two\n" {
		t.Fatal("regeneration did not refresh locked source")
	}
}

func TestGeneratedViewOwnsMutatingHostShortcuts(t *testing.T) {
	v := newGeneratedView()
	for _, c := range []struct {
		key  int
		mods uint8
	}{
		{'Z', gui.ModAction}, {'Y', gui.ModAction}, {'Z', gui.ModAction | gui.ModShift},
		{'I', gui.ModAction | gui.ModShift}, {'.', gui.ModAction}, {gui.KeyF2, 0},
	} {
		if !v.HandleShortcut(c.key, c.mods) {
			t.Fatalf("host mutation escaped: %d/%d", c.key, c.mods)
		}
	}
	for _, c := range []struct {
		key  int
		mods uint8
	}{
		{'C', gui.ModAction}, {'F', gui.ModAction}, {'P', gui.ModAction},
		{'S', gui.ModAction}, {'Q', gui.ModAction}, {'Z', 0}, {gui.KeyF2, gui.ModAction},
	} {
		if v.HandleShortcut(c.key, c.mods) {
			t.Fatalf("unrelated command intercepted: %d/%d", c.key, c.mods)
		}
	}
	var _ gui.IShortcutOverride = v
}

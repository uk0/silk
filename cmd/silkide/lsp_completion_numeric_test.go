package main

import (
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/gui"
)

func TestLSPAutomaticCompletionIgnoresNumbersWithoutDroppingSignature(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	d := installLSPSnapshotProbe(t, path)
	ed := gui.NewCodeEditor()
	ed.SetFilePath(path)
	t.Cleanup(func() { ed.CancelCompletion(); ed.CancelSymbolIndex(); ed.CancelGitStatus() })
	ed.SigChanged(func(text string) { updateLSPDocument(path, ed, text) })
	ed.SetText("package p\nvar _ = 𐐀call")
	ed.NavigateToLineCol(1, utf8.RuneCountInString("var _ = 𐐀call"))
	ed.OnTextInput("(")
	ed.OnTextInput("2")
	if d.latest.complete || !d.latest.signature || d.latest.col != ed.CursorUTF16Col() || d.latest.text != ed.Text() {
		t.Fatalf("numeric changed didChange/signature semantics or scheduled completion: %+v", d.latest)
	}
	ed.OnTextInput(".")
	if d.latest.complete || !d.latest.signature {
		t.Fatalf("decimal dot scheduled completion or lost signature: %+v", d.latest)
	}
	requestLSPCompletion(path, ed)
	if !d.latest.complete || d.latest.text != ed.Text() || d.latest.col != ed.CursorUTF16Col() {
		t.Fatal("explicit LSP completion was incorrectly admitted by the automatic policy")
	}
	for _, text := range []string{"sensor2", "测点2.单位", "items[2]."} {
		ed.SetText(text)
		ed.NavigateToLineCol(0, utf8.RuneCountInString(text))
		updateLSPDocument(path, ed, ed.Text())
		if !d.latest.complete {
			t.Fatalf("lost completion for %q: %+v", text, d.latest)
		}
	}
}

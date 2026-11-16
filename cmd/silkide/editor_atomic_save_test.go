package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/gui"
)

func TestEditorAtomicSavePreservesReadOnlyDocumentAndDirtyBuffer(t *testing.T) {
	oldToast, oldCanvas := silkideToast, globalCanvas
	globalCanvas = nil
	var levels []gui.ToastLevel
	silkideToast = func(_ string, level gui.ToastLevel) { levels = append(levels, level) }
	t.Cleanup(func() { silkideToast, globalCanvas = oldToast, oldCanvas })
	path := filepath.Join(t.TempDir(), "protected.go")
	original, edited := "package main\n", "package main\nfunc f( ){println(\"dirty😀\")}\n"
	if err := os.WriteFile(path, []byte(original), 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	ed := gui.NewCodeEditor()
	t.Cleanup(ed.CancelGitStatus)
	ed.SetText(original)
	ed.MarkSaved()
	ed.ReplaceAllText(edited)
	revision := ed.TextRevision()
	if saveEditorToDisk(ed, path) {
		t.Fatal("saved over read-only document")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != original || ed.Text() != edited || ed.TextRevision() != revision || !ed.Modified() {
		t.Fatal("failed atomic save altered disk, formatted the buffer, or lost dirty state")
	}
	if len(levels) != 1 || levels[0] != gui.ToastError {
		t.Fatalf("wrong save feedback: %v", levels)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	if !saveEditorToDisk(ed, path) {
		t.Fatal("save did not recover after removing read-only protection")
	}
	after, _ := os.Stat(path)
	data, err = os.ReadFile(path)
	if err != nil || string(data) != ed.Text() || ed.Modified() || after.Mode().Perm() != before.Mode().Perm() {
		t.Fatal("successful save did not persist exact formatted buffer/permissions")
	}
}

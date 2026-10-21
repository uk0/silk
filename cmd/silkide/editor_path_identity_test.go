package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A rename must change document identity without recreating its editor. Drive
// the existing changed signal after moving/rebinding a real file: callbacks
// must not keep the original openFileInEditor path captured in their closure.
func TestEditorCallbacksFollowCurrentFilePath(t *testing.T) {
	w := testWorkbench(t)
	prior := globalLSP
	globalLSP = nil
	t.Cleanup(func() { globalLSP = prior })
	old := writeFile(t, t.TempDir(), "before.go", "package p\n")
	next := filepath.Join(filepath.Dir(old), "after.silkgo")
	if !openFileInEditor(w.tabs, old) {
		t.Fatal("open")
	}
	ed := openEditors[old]
	if err := os.Rename(old, next); err != nil {
		t.Fatal(err)
	}
	delete(openEditors, old)
	openEditors[next] = ed
	ed.SetFilePath(next)
	ed.ReplaceAllText("package p\n//silk:macro M()\nvar Value int\n//silk:end\n")
	if title := w.tabs.TabTitle(0); !strings.HasPrefix(title, "after.silkgo") {
		t.Errorf("changed signal used stale identity: %q", title)
	}
	if macroEditors[next] == nil {
		t.Error("changed signal did not attach macro analysis to new identity")
	}
	if macroEditors[old] != nil {
		t.Error("old identity was resurrected")
	}
}

package ged

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEditorTabsNavigationRoundTripAndFailedOpen(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.go"), filepath.Join(dir, "b.go")
	for p, s := range map[string]string{a: "package p\nfunc target(){}\n", b: "package p\nfunc use(){target()}\n"} {
		if err := os.WriteFile(p, []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	tabs := NewEditorTabs()
	t.Cleanup(func() {
		for len(tabs.tabs) > 0 {
			tabs.CloseTab(0)
		}
	})
	tabs.OpenFile(b)
	tabs.ActiveEditor().NavigateToLineCol(1, 13)
	tabs.ActiveEditor().GoToDefinitionAtCursor()
	if tabs.ActiveEditor().FilePath() != a || tabs.ActiveEditor().CursorCol() != 5 {
		t.Fatal("SDK jump lost destination column")
	}
	tabs.ActiveEditor().NavGoBack()
	if tabs.ActiveEditor().FilePath() != b || tabs.ActiveEditor().CursorCol() != 13 {
		t.Fatal("SDK shared back failed")
	}
	tabs.ActiveEditor().NavGoForward()
	before := tabs.ActiveEditor().NavigationPosition()
	if tabs.OpenFileAtPosition(filepath.Join(dir, "missing.go"), 99, 2) || tabs.ActiveEditor().NavigationPosition() != before {
		t.Fatal("failed open moved unrelated editor")
	}
}

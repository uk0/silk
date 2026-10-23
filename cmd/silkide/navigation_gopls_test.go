package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// Opt-in real protocol/IDE flow. No fake replies: the on-disk caller is stale
// and gopls must consume didChange before resolving its Unicode-adjacent use.
func TestRealGoplsNavigationUnsavedTypeImplementationAndBack(t *testing.T) {
	if testing.Short() {
		t.Skip("real gopls integration")
	}
	bin, err := exec.LookPath("gopls")
	if err != nil {
		t.Skip("gopls not installed")
	}
	w := testWorkbench(t)
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module navigationqa\n\ngo 1.26.10\n")
	types := writeFile(t, dir, "types.go", "package demo\ntype Speaker interface { Speak() string }\ntype Greeter struct{}\nfunc (Greeter) Speak() string { return \"hello\" }\ntype Other struct{}\nfunc (Other) Speak() string { return \"other\" }\n")
	source := writeFile(t, dir, "source 空间.go", "package demo\nfunc use() {}\n")
	oldClient, oldDocs, oldQueue, oldPanel, oldToast := globalLSP, lspDocuments, symbolNavigation, globalReferencesPanel, silkideToast
	oldVersions := lspVersions
	lspVersions = map[string]int{}
	globalLSP = nil
	lspDocuments = map[string]*lspDocument{}
	openFileInEditor(w.tabs, source)
	ed := activeEditor(w.tabs)
	ed.SigChanged(nil)
	text := "package demo\nfunc use() { /*😀*/ var value Speaker = Greeter{}; _ = value.Speak() }\n"
	ed.ReplaceAllText(text)
	line := strings.Split(text, "\n")[1]
	column := len([]rune(line[:strings.LastIndex(line, "value")]))
	ed.NavigateToLineCol(1, column)
	c, err := core.LaunchLSPClient(bin)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for range c.Notifications() {
		}
	}()
	posts := make(chan func(), 4)
	symbolNavigation = navigationQueue{post: func(f func()) { posts <- f }}
	globalReferencesPanel = ged.NewReferencesPanel()
	silkideToast = func(message string, _ gui.ToastLevel) { t.Errorf("unexpected navigation feedback: %s", message) }
	t.Cleanup(func() {
		symbolNavigation.stop()
		for _, d := range lspDocuments {
			d.close()
		}
		for _, d := range lspDocuments {
			waitDocument(t, d)
		}
		c.Close()
		deadline := time.After(3 * time.Second)
		for symbolNavigation.running {
			select {
			case f := <-posts:
				f()
			case <-deadline:
				t.Error("navigation worker did not retire")
				return
			}
		}
		globalLSP, lspDocuments, symbolNavigation, globalReferencesPanel, silkideToast = oldClient, oldDocs, oldQueue, oldPanel, oldToast
		lspVersions = oldVersions
	})
	if _, err := c.Initialize(core.LSPInitializeParams{ProcessID: os.Getpid(), RootURI: fileURIOf(dir), Capabilities: ideLSPCapabilities()}); err != nil {
		t.Fatal(err)
	}
	globalLSP = c
	d := newLSPDocument(c, fileURIOf(source), "package demo\nfunc use() {}\n", nil, nil)
	lspDocuments[source] = d
	d.update(lspSnapshot{text: text, version: 2})
	request := func(op string) {
		t.Helper()
		requestSymbolNavigation(w.tabs, op)
		select {
		case f := <-posts:
			f()
		case <-time.After(15 * time.Second):
			t.Fatal("navigation timeout", op)
		}
	}
	request("Go to Type Definition")
	if activeEditorPath(w.tabs) != types || activeEditor(w.tabs).CursorLine() != 1 || activeEditor(w.tabs).CursorCol() != 5 {
		t.Fatal("type definition did not resolve Speaker", editorNavigationPosition(w.tabs))
	}
	navigateEditorHistory(w.tabs, false)
	if activeEditor(w.tabs) != ed || ed.CursorCol() != column || ed.Text() != text || !ed.Modified() {
		t.Fatal("Back lost unsaved Unicode caller")
	}
	// Definition must point at the local value declaration in the unsaved buffer.
	request("Go to Definition")
	want := len([]rune(line[:strings.Index(line, "value")]))
	if activeEditor(w.tabs) != ed || ed.CursorCol() != want {
		t.Fatalf("definition column=%d want %d", ed.CursorCol(), want)
	}
	openFileInEditorAt(w.tabs, types, 2, len("type Speaker interface { "))
	request("Go to Implementation")
	rows := globalReferencesPanel.Locations()
	if len(rows) != 2 {
		t.Fatalf("implementations=%+v", rows)
	}
	for _, row := range rows {
		if row.File != filepath.Clean(types) || (row.Line != 4 && row.Line != 6) {
			t.Fatalf("wrong implementation: %+v", row)
		}
	}
	t.Logf("real gopls: unsaved Unicode type/definition/back and %d implementations verified", len(rows))
}

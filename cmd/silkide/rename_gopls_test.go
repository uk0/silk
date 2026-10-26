package main

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenameDialogRealGoplsUnsavedUnicode(t *testing.T) {
	bin := requireRealGopls(t)
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.go"), filepath.Join(dir, "b.go")
	first := "package demo\nfunc oldName()int{return 1}\nfunc f(){/*🙂*/_ = oldName()}\n"
	second := "package demo\nfunc other()int{return oldName()}\n"
	for name, text := range map[string]string{"go.mod": "module renameqa\n\ngo 1.26\n", "a.go": first, "b.go": "package demo\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	client, err := core.LaunchLSPClient(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	go func() {
		for range client.Notifications() {
		}
	}()
	if _, err = client.Initialize(core.LSPInitializeParams{ProcessID: os.Getpid(), RootURI: fileURIOf(dir), Capabilities: ideLSPCapabilities()}); err != nil {
		t.Fatal(err)
	}
	oldEditors, oldDocs := openEditors, lspDocuments
	t.Cleanup(func() { openEditors, lspDocuments = oldEditors, oldDocs })
	openEditors = map[string]*gui.CodeEditor{}
	lspDocuments = map[string]*lspDocument{}
	for p, text := range map[string]string{a: first, b: second} {
		e := gui.NewCodeEditor()
		e.SetFilePath(p)
		e.SetText(text)
		openEditors[p] = e
		d := newLSPDocument(client, fileURIOf(p), text, nil, nil)
		lspDocuments[p] = d
		t.Cleanup(func() { d.close(); waitDocument(t, d); e.CancelCompletion() })
	}
	e := openEditors[a]
	e.ScrollToLineCol(2, 21)
	if e.WordAtCursor() != "oldName" {
		t.Fatal("bad fixture cursor", e.WordAtCursor())
	}
	snapshot := captureRenameBuffers()
	if !syncRenameBuffers(snapshot) {
		t.Fatal("unsaved buffers not flushed")
	}
	edit, err := client.Rename(fileURIOf(a), e.CursorLine(), e.CursorUTF16Col(), "newName")
	if err != nil {
		t.Fatal(err)
	}
	if edit == nil || len(edit.Changes) != 2 || !renameTargetsCurrent(edit, snapshot) {
		t.Fatal("missing unsaved cross-file rename", edit)
	}
	for p, source := range map[string]string{a: first, b: second} {
		edits := edit.Changes[fileURIOf(p)]
		if len(edits) == 0 {
			t.Fatal("target missing", p)
		}
		after, err := core.ApplyTextEdits(source, edits)
		if err != nil || after != strings.ReplaceAll(source, "oldName", "newName") {
			t.Fatal("bad semantic edits", p, after, err)
		}
		ed := openEditors[p]
		ed.ReplaceAllText(after)
		ed.Undo()
		if ed.Text() != source {
			t.Fatal("rename history lost unsaved source")
		}
	}
	t.Logf("real gopls rename: 2 unsaved buffers, UTF16 column=%d, second-file disk unchanged", e.CursorUTF16Col())
	if disk, _ := os.ReadFile(b); string(disk) != "package demo\n" {
		t.Fatal("test wrote unsaved buffer to disk")
	}
}

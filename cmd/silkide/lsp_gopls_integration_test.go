package main

import (
	"context"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGoplsUnsavedUnicodeCompletionAndFormatting(t *testing.T) {
	if testing.Short() {
		t.Skip("real gopls integration")
	}
	bin, err := exec.LookPath("gopls")
	if err != nil {
		t.Skip("gopls not installed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "source 空间#%.go")
	for name, text := range map[string]string{"go.mod": "module qa\n\ngo 1.26\n", filepath.Base(path): "package demo\nfunc f(){}\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c, err := core.LaunchLSPClient(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	go func() {
		for range c.Notifications() {
		}
	}()
	if _, err = c.Initialize(core.LSPInitializeParams{ProcessID: os.Getpid(), RootURI: fileURIOf(dir), Capabilities: ideLSPCapabilities()}); err != nil {
		t.Fatal(err)
	}
	d := newLSPDocument(c, fileURIOf(path), "package demo\nfunc f(){}\n", nil, nil)
	defer func() { d.close(); waitDocument(t, d) }()
	text := "package demo\nimport \"fmt\"\nfunc f(){ /*😀*/ fmt.Pr }\n"
	s := lspSnapshot{text: text, version: 2, line: 2, col: 23}
	d.update(s)
	if _, err = d.sync(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	items, err := c.CompletionContext(ctx, fileURIOf(path), s.line, s.col)
	if err != nil {
		t.Fatal(err)
	}
	var item *core.LSPCompletionItem
	for _, v := range items {
		if v.Label == "Println" {
			cp := v
			item = &cp
			break
		}
	}
	if item == nil {
		t.Fatalf("Println absent in %d real server candidates: %+v", len(items), items)
	}
	ed := gui.NewCodeEditor()
	ed.SetText(text)
	ed.ScrollToLineCol(2, 22)
	if err = applyLSPCompletion(ed, s, *item); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ed.Text(), "fmt.Println") || strings.Contains(ed.Text(), "$") {
		t.Fatalf("bad insertion: %q", ed.Text())
	}
	// Real formatter must see the unsaved completion, not the disk baseline.
	d.update(lspSnapshot{text: ed.Text(), version: 3})
	if _, err = d.sync(); err != nil {
		t.Fatal(err)
	}
	edits, err := c.Formatting(fileURIOf(path))
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := core.ApplyTextEdits(ed.Text(), edits)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(formatted, "fmt.Println") {
		t.Fatal(formatted)
	}
	t.Logf("real gopls completion textEdit=%+v; formatted unsaved source=%q", item.TextEdit, formatted)
	ed.Undo()
	if ed.Text() != text {
		t.Fatal("undo lost unsaved source")
	}
}

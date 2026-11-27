package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func TestLSPStandbyRequiresAnOwnedGoEditor(t *testing.T) {
	oldEditors, oldRoot, oldPhase := openEditors, globalProjectRoot, lspActivityPhase
	oldClient, oldCancel := globalLSP, lspStartCancel
	t.Cleanup(func() {
		openEditors, globalProjectRoot, lspActivityPhase = oldEditors, oldRoot, oldPhase
		globalLSP, lspStartCancel = oldClient, oldCancel
	})
	globalLSP, lspStartCancel, globalProjectRoot, lspActivityPhase = nil, nil, "", lspStandby
	root := t.TempDir()
	source := filepath.Join(root, "lib", "file.go")
	foreign := filepath.Join(root, "foreign.go")
	design := filepath.Join(root, "screen.silkui")
	notes := filepath.Join(root, "notes.md")
	openEditors = map[string]*gui.CodeEditor{source: {}, design: {}, notes: {}}
	var launches []string
	start := func(dir string) {
		launches = append(launches, dir)
		lspActivityPhase = "starting"
	}
	if demandLSPForEditor(design, start) || demandLSPForEditor(notes, start) ||
		demandLSPForEditor(foreign, start) || len(launches) != 0 {
		t.Fatal("a design, non-Go document or unowned path started gopls")
	}
	if !demandLSPForEditor(source, start) || demandLSPForEditor(source, start) ||
		len(launches) != 1 || launches[0] != filepath.Dir(source) {
		t.Fatal("the first Go buffer should start exactly once at its own directory:", launches)
	}
	lspActivityPhase = "offline"
	if demandLSPForEditor(source, start) || len(launches) != 1 {
		t.Fatal("automatic demand retried a failed server")
	}
	lspActivityPhase = lspStandby
	globalProjectRoot = root
	if !demandLSPForEditor(source, start) || len(launches) != 2 || launches[1] != root {
		t.Fatal("an explicit workspace should take precedence:", launches)
	}
}

func TestSymbolFinderUsesLatestQueryWhenLSPConnects(t *testing.T) {
	oldClient, oldPhase, oldFinder, oldToast, oldButton := globalLSP, lspActivityPhase, activeSymbolFinder, silkideToast, statusBarLSPButton
	t.Cleanup(func() {
		globalLSP, lspActivityPhase, activeSymbolFinder, silkideToast, statusBarLSPButton = oldClient, oldPhase, oldFinder, oldToast, oldButton
	})
	globalLSP, lspActivityPhase, statusBarLSPButton = nil, "starting", nil
	silkideToast = func(message string, _ gui.ToastLevel) { t.Errorf("unexpected startup toast: %s", message) }
	posts, queries := make(chan func(), 8), make(chan symbolSearchQuery, 8)
	f := newSymbolFinder(nil, func(_ context.Context, q symbolSearchQuery) ([]core.LSPWorkspaceSymbol, error) {
		queries <- q
		return []core.LSPWorkspaceSymbol{{Name: q.text}}, nil
	}, func(fn func()) { posts <- fn })
	defer func() { f.close(); receiveSymbolTest(t, f.worker.done) }()
	activeSymbolFinder = f
	f.input.SetText("initial")
	f.input.SetText("latest")
	if f.status.Text() != i18n.T("Starting the language service; editor input remains available.") || f.list.Count() != 0 {
		t.Fatal(f.status.Text())
	}
	client := &core.LSPClient{}
	globalLSP = client
	setLSPActivityPhase("connected")
	if q := receiveSymbolTest(t, queries); q.client != client || q.text != "latest" {
		t.Fatalf("wrong resumed request: %+v", q)
	}
	receiveSymbolTest(t, posts)()
	if f.list.Count() != 1 || f.resultClient != client {
		t.Fatal("waiting query did not reach the connected service")
	}
	globalLSP = nil
	setLSPActivityPhase("offline")
	if f.list.Count() != 0 {
		t.Fatal("lost language session left stale workspace symbols actionable")
	}
}

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func TestIDEFailingLSPChild(t *testing.T) {
	if os.Args[len(os.Args)-1] != "silk-failed-lsp-child" {
		return
	}
	fmt.Fprint(os.Stdout, "Content-Length: 1\r\n\r\n!")
	os.Exit(0)
}
func TestLSPDisconnectCodedOnceAndOldSessionIgnored(t *testing.T) {
	oldClient, oldEpoch, oldToast, oldLog := globalLSP, lspSessionGeneration, silkideToast, globalLog
	oldDocuments, oldEditors := lspDocuments, openEditors
	lspDocuments = map[string]*lspDocument{}
	openEditors = map[string]*gui.CodeEditor{}
	defer func() {
		globalLSP, lspSessionGeneration, silkideToast, globalLog = oldClient, oldEpoch, oldToast, oldLog
		lspDocuments, openEditors = oldDocuments, oldEditors
	}()
	var messages []string
	silkideToast = func(s string, _ gui.ToastLevel) { messages = append(messages, s) }
	globalLog = ged.NewLogPanel()
	client, err := core.LaunchLSPClient(os.Args[0], "-test.run=^TestIDEFailingLSPChild$", "--", "silk-failed-lsp-child")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	done := make(chan struct{})
	go func() {
		for range client.Notifications() {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("failed child not retired")
	}
	if diagnostic.CodeOf(client.Err()) != diagnostic.LSPTransport {
		t.Fatal(client.Err())
	}
	globalLSP = client
	lspSessionGeneration = 42
	finishLSPConnection(client, 41)
	if globalLSP != client || len(messages) != 0 {
		t.Fatal("stale callback changed current session")
	}
	finishLSPConnection(client, 42)
	finishLSPConnection(client, 42)
	if globalLSP != nil || len(messages) != 1 || !strings.Contains(messages[0], "SILK-LSP003") {
		t.Fatalf("missing/duplicate session failure: %v", messages)
	}
	entries := globalLog.Entries()
	d, _ := diagnostic.Lookup(diagnostic.LSPTransport)
	if len(entries) != 1 || !strings.Contains(entries[0].Message, d.Hint) {
		t.Fatal("session recovery hint lost")
	}
}
func TestLSPStartupFailureSurvivesOwnCleanup(t *testing.T) {
	oldEpoch, oldCancel, oldToast, oldLog := lspSessionGeneration, lspStartCancel, silkideToast, globalLog
	defer func() {
		lspSessionGeneration, lspStartCancel, silkideToast, globalLog = oldEpoch, oldCancel, oldToast, oldLog
	}()
	ctx, cancel := context.WithCancel(context.Background())
	lspStartCancel = cancel
	lspSessionGeneration = 8
	globalLog = ged.NewLogPanel()
	var messages []string
	silkideToast = func(s string, _ gui.ToastLevel) { messages = append(messages, s) }
	cancel() // Handshake failure cleanup precedes the posted GUI callback.
	reportLSPStartFailure(7, io.EOF)
	if len(messages) != 0 {
		t.Fatal("old launch surfaced")
	}
	reportLSPStartFailure(8, fmt.Errorf("initialize failed: %w", io.EOF))
	if !errors.Is(ctx.Err(), context.Canceled) || lspStartCancel != nil || len(messages) != 1 || !strings.Contains(messages[0], "SILK-LSP001") || !strings.Contains(messages[0], "EOF") {
		t.Fatalf("startup failure swallowed: %v", messages)
	}
}

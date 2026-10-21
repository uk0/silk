package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func TestLSPFeedbackStableCodesAndCause(t *testing.T) {
	old, oldLog := silkideToast, globalLog
	globalLog = ged.NewLogPanel()
	defer func() { silkideToast, globalLog = old, oldLog }()
	var messages []string
	var levels []gui.ToastLevel
	silkideToast = func(message string, level gui.ToastLevel) {
		messages = append(messages, message)
		levels = append(levels, level)
	}
	u := reportLSPUnavailable()
	if u.Code != diagnostic.LSPUnavailable || u.Hint == "" || levels[0] != gui.ToastWarning || !strings.Contains(messages[0], "SILK-LSP001") {
		t.Fatalf("unavailable: %+v, %v", u, messages)
	}
	cause := errors.New("server disconnected")
	r := reportLSPRequestError("Find References", "target.go", cause)
	if r.Code != diagnostic.LSPRequest || r.Hint == "" || r.Location.File != "target.go" || !errors.Is(r, cause) || levels[1] != gui.ToastError || !strings.Contains(messages[1], "SILK-LSP002") || !strings.Contains(messages[1], cause.Error()) {
		t.Fatalf("request error: %+v, %v", r, messages)
	}
	entries := globalLog.Entries()
	if len(entries) != 2 || entries[0].Level != ged.LogWarn || entries[1].Level != ged.LogError || !strings.Contains(entries[1].Message, r.Hint) || !strings.Contains(entries[1].Message, "target.go") {
		t.Fatalf("persistent log lost severity, path, or recovery hint: %+v", entries)
	}
	for _, err := range []error{nil, context.Canceled} {
		if reportLSPRequestError("Find References", "target.go", err) != nil {
			t.Fatal("normal cancellation reported as failure")
		}
	}
	if len(messages) != 2 {
		t.Fatal("cancelled requests created toast noise")
	}
	if reportLSPRequestError("Find References", "target.go", context.DeadlineExceeded) == nil {
		t.Fatal("real timeout silently discarded")
	}
}

func TestLSPUnavailableActionsUseStableCode(t *testing.T) {
	w := testWorkbench(t)
	oldClient, oldToast := globalLSP, silkideToast
	globalLSP = nil
	defer func() { globalLSP, silkideToast = oldClient, oldToast }()
	for _, action := range []struct {
		name string
		run  func()
	}{
		{"definition", func() { goToDefinitionViaLSP(w.tabs) }},
		{"rename", func() { renameSymbolAtActiveEditor(w.tabs) }},
		{"references", func() { findReferencesViaLSP(w.tabs) }},
		{"actions", func() { codeActionsViaLSP(w.tabs) }},
		{"format", func() { formatDocumentViaLSP(w.tabs) }},
		{"symbols", func() { showSymbolFinder(w.frame, w.tabs) }},
	} {
		t.Run(action.name, func(t *testing.T) {
			var message string
			silkideToast = func(s string, _ gui.ToastLevel) { message = s }
			action.run()
			if !strings.Contains(message, "SILK-LSP001") {
				t.Fatalf("missing unavailable code: %q", message)
			}
		})
	}
}

func TestLSPTransportFailureKeepsCodeAndIsNotSilentCancellation(t *testing.T) {
	old, oldLog := silkideToast, globalLog
	globalLog = ged.NewLogPanel()
	defer func() { silkideToast, globalLog = old, oldLog }()
	var message string
	silkideToast = func(s string, _ gui.ToastLevel) { message = s }
	cause := diagnostic.New(diagnostic.LSPTransport, "partial pipe write", context.Canceled)
	got := reportLSPRequestError("Workspace symbols", "a.go", cause)
	if got == nil || got.Code != diagnostic.LSPTransport || !errors.Is(got, context.Canceled) || got.Hint != cause.Hint || !strings.Contains(message, "SILK-LSP003") {
		t.Fatalf("fatal cancellation hidden: %+v %q", got, message)
	}
	entries := globalLog.Entries()
	if len(entries) != 1 || !strings.Contains(entries[0].Message, cause.Hint) {
		t.Fatalf("missing recovery hint: %+v", entries)
	}
}

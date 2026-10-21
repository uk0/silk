package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func setupSymbolFinder(t *testing.T, request symbolSearchRequest) (*symbolFinder, chan func(), *[]string) {
	t.Helper()
	oldClient, oldToast := globalLSP, silkideToast
	globalLSP = new(core.LSPClient)
	var messages []string
	silkideToast = func(s string, _ gui.ToastLevel) { messages = append(messages, s) }
	posts := make(chan func(), 8)
	f := newSymbolFinder(nil, request, func(fn func()) { posts <- fn })
	t.Cleanup(func() { f.close(); receiveSymbolTest(t, f.worker.done); globalLSP, silkideToast = oldClient, oldToast })
	return f, posts, &messages
}

func TestSymbolFinderClearsOldActionsAndUsesSelectedRow(t *testing.T) {
	f, posts, _ := setupSymbolFinder(t, func(_ context.Context, q symbolSearchQuery) ([]core.LSPWorkspaceSymbol, error) {
		return []core.LSPWorkspaceSymbol{{Name: q.text + "first", Character: 17}, {Name: q.text + "second", Character: 20}}, nil
	})
	receiveSymbolTest(t, posts)()
	if f.list.Count() != 2 || f.list.ActiveIndex() != 0 {
		t.Fatal("initial results not selectable")
	}
	f.input.SetText("latest")
	if f.list.Count() != 0 || f.resultClient != nil {
		t.Fatal("old results actionable while pending")
	}
	f.choose(0)
	if f.selected != nil {
		t.Fatal("Enter accepted a stale result")
	}
	receiveSymbolTest(t, posts)()
	f.list.SetActiveIndex(1)
	f.input.OnKeyDown(gui.KeyEnter, false)
	if f.selected == nil || f.selected.Name != "latestsecond" || f.selected.Character != 20 {
		t.Fatalf("wrong selected row/UTF16: %+v", f.selected)
	}
}

func TestSymbolFinderBoundsResultsAndShowsEmptyState(t *testing.T) {
	f, posts, _ := setupSymbolFinder(t, func(_ context.Context, q symbolSearchQuery) ([]core.LSPWorkspaceSymbol, error) {
		if q.text == "empty" {
			return nil, nil
		}
		rows := make([]core.LSPWorkspaceSymbol, symbolFinderLimit+1)
		for i := range rows {
			rows[i].Name = fmt.Sprintf("Symbol%d", i)
		}
		return rows, nil
	})
	receiveSymbolTest(t, posts)()
	if f.list.Count() != symbolFinderLimit || !strings.Contains(f.status.Text(), "501") {
		t.Fatalf("limit not disclosed: %d %q", f.list.Count(), f.status.Text())
	}
	f.dialog.SetSize(560, 480)
	f.dialog.Layout()
	if f.list.Height() < 200 || f.list.Y()+f.list.Height() > f.status.Y()+1 {
		t.Fatal("status overlaps results or collapses viewport")
	}
	f.input.SetText("empty")
	receiveSymbolTest(t, posts)()
	if f.list.Count() != 0 || f.list.ActiveIndex() != -1 || f.status.Text() != i18n.T("No symbols found") {
		t.Fatal("empty result leaves old rows/selection")
	}
}

func TestSymbolFinderRetiresChangedServerAndClosedDialog(t *testing.T) {
	requests := make(chan symbolSearchQuery, 4)
	f, posts, _ := setupSymbolFinder(t, func(_ context.Context, q symbolSearchQuery) ([]core.LSPWorkspaceSymbol, error) {
		requests <- q
		return []core.LSPWorkspaceSymbol{{Name: q.text}}, nil
	})
	first := receiveSymbolTest(t, requests)
	cb := receiveSymbolTest(t, posts)
	globalLSP = new(core.LSPClient)
	cb()
	next := receiveSymbolTest(t, requests)
	if next.client == first.client || next.client != globalLSP || f.list.Count() != 0 {
		t.Fatal("replacement session received stale rows")
	}
	receiveSymbolTest(t, posts)()
	if f.list.Count() != 1 {
		t.Fatal("new session result missing")
	}
	f.input.SetText("close race")
	cb = receiveSymbolTest(t, posts)
	f.close()
	cb()
	if f.list.Count() != 0 {
		t.Fatal("closed dialog was repopulated")
	}
}

func TestSymbolFinderFailureCodesAndUnavailableDeduplication(t *testing.T) {
	f, posts, messages := setupSymbolFinder(t, func(_ context.Context, q symbolSearchQuery) ([]core.LSPWorkspaceSymbol, error) {
		if q.text == "cancel" {
			return nil, context.Canceled
		}
		return nil, errors.New("workspace probe failure")
	})
	receiveSymbolTest(t, posts)()
	if len(*messages) != 1 || !strings.Contains((*messages)[0], "workspace probe failure") || !strings.Contains(f.status.Text(), "SILK-LSP002") {
		t.Fatal("request error lost code or underlying cause")
	}
	f.input.SetText("cancel")
	receiveSymbolTest(t, posts)()
	if len(*messages) != 1 {
		t.Fatal("cancellation produced an error banner")
	}
	globalLSP = nil
	f.input.SetText("offline")
	f.input.SetText("offline2")
	if len(*messages) != 2 || !strings.Contains(f.status.Text(), "SILK-LSP001") || f.list.Count() != 0 {
		t.Fatal("unavailable feedback missing or repeated per keystroke")
	}
}

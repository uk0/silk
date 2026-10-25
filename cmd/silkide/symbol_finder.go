package main

import (
	"context"
	"fmt"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

const symbolFinderLimit = 500

// UI-thread-owned dialog state; all RPC work and coalescing live in worker.
type symbolFinder struct {
	dialog              *gui.Dialog
	input               *gui.Edit
	list                *gui.ListWidget
	status              *gui.Label
	worker              *symbolSearchWorker
	closed, unavailable bool
	resultClient        *core.LSPClient
	selected            *core.LSPWorkspaceSymbol
}

func showSymbolFinder(parent gui.IWidget, tabs *gui.TabWidget) {
	if parent == nil {
		return
	}
	if globalLSP == nil {
		reportLSPUnavailable()
		return
	}
	finder := newSymbolFinder(parent, func(ctx context.Context, q symbolSearchQuery) ([]core.LSPWorkspaceSymbol, error) {
		return q.client.WorkspaceSymbolContext(ctx, q.text)
	}, gui.Post)
	defer finder.close()
	result := finder.dialog.ShowModal()
	finder.close()
	if reportDialogFailure(finder.dialog.LastError()) {
		return
	}
	if result == gui.DialogOK && finder.selected != nil && tabs != nil {
		// Never focus/open another editor before the native modal has unwound.
		s := finder.selected
		openFileInEditorAtLSP(tabs, uriToPath(s.URI), core.LSPPosition{Line: s.Line, Character: s.Character})
	}
}

func newSymbolFinder(parent gui.IWidget, request symbolSearchRequest, post func(func())) *symbolFinder {
	f := &symbolFinder{dialog: gui.NewDialog(i18n.T("Go to Symbol in Workspace"), parent),
		input: gui.NewEdit(), list: gui.NewListWidget(), status: gui.NewLabel(i18n.T("Searching symbols..."))}
	box := gui.NewVBox()
	box.SetSpacing(6)
	box.AddWidget(f.input)
	f.list.SetFillAvailable(true)
	f.list.SetSelectionVisible(true)
	box.AddWidget(f.list)
	f.input.SetNavigationList(f.list)
	box.AddWidget(f.status)
	f.worker = newSymbolSearchWorker(90*time.Millisecond, request, post, func(r symbolSearchResult) {
		if f.closed {
			return
		}
		if r.query.client != globalLSP {
			f.query(f.input.Text())
			return
		}
		if r.err != nil {
			if err := reportLSPRequestError(i18n.T("Go to Symbol in Workspace"), "", r.err); err != nil {
				f.status.SetText(fmt.Sprintf("[%s] %s", err.Code, i18n.T("Symbol search failed")))
				gui.SetToolTip(f.status, err.Error()+"\n"+err.Hint)
			} else {
				f.status.SetText(i18n.T("Symbol search cancelled"))
			}
			return
		}
		count := min(len(r.symbols), symbolFinderLimit)
		items := make([]gui.ListItem, 0, count)
		for _, s := range r.symbols[:count] {
			label := s.Name
			if s.ContainerName != "" {
				label = s.ContainerName + "." + s.Name
			}
			items = append(items, gui.ListItem{Text: label, Data: s})
		}
		fillFinderList(f.list, items)
		f.resultClient = r.query.client
		switch {
		case len(r.symbols) == 0:
			f.status.SetText(i18n.T("No symbols found"))
		case len(r.symbols) > count:
			f.status.SetText(fmt.Sprintf(i18n.T("Showing %d of %d symbols; refine your search"), count, len(r.symbols)))
		default:
			f.status.SetText(fmt.Sprintf(i18n.T("%d symbols"), count))
		}
	})
	f.input.SigTextChanged(func(_ interface{}, q string) { f.query(q) })
	f.input.SigSubmit(func(_ interface{}, _ string) { f.choose(f.list.ActiveIndex()) })
	f.list.SigSubmit(func(interface{}) { f.choose(f.list.ActiveIndex()) })
	f.dialog.SetContent(box)
	f.dialog.AddButton(i18n.T("Cancel"), gui.DialogCancel)
	f.dialog.SetSize(560, 480)
	f.dialog.SetInitialFocus(f.input)
	f.query("")
	return f
}

func (f *symbolFinder) query(q string) {
	if f.closed {
		return
	}
	// An old selected result must not remain actionable during debounce/RPC.
	fillFinderList(f.list, nil)
	f.resultClient = nil
	gui.SetToolTip(f.status, "")
	f.worker.update(globalLSP, q)
	if globalLSP == nil {
		f.status.SetText(fmt.Sprintf("[%s] %s", diagnostic.LSPUnavailable, i18n.T("LSP not running")))
		if !f.unavailable {
			reportLSPUnavailable()
			f.unavailable = true
		}
	} else {
		f.unavailable = false
		f.status.SetText(i18n.T("Searching symbols..."))
	}
}

func (f *symbolFinder) choose(idx int) {
	if f.closed || idx < 0 || idx >= f.list.Count() {
		return
	}
	if f.resultClient != globalLSP {
		f.query(f.input.Text())
		return
	}
	if s, ok := f.list.Item(idx).Data.(core.LSPWorkspaceSymbol); ok {
		f.selected = &s
		dismissDialog(f.dialog)
	}
}

func (f *symbolFinder) close() {
	if f.closed {
		return
	}
	f.closed = true
	f.input.SetNavigationList(nil)
	f.worker.close()
}

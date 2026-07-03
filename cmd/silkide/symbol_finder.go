package main

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

// showSymbolFinder pops a "Go to Symbol in Workspace" modal (Cmd+T): the user
// types a query, gopls' workspace/symbol returns matches across the whole
// project, and selecting one jumps to its file:line. Mirrors showFileFinder,
// but the data source is an async LSP RPC instead of a local file walk — each
// keystroke fires a query; a generation counter drops stale responses so a
// slow earlier query can't overwrite a newer one's results.
func showSymbolFinder(parent gui.IWidget, tabs *gui.TabWidget) {
	if parent == nil {
		return
	}
	if globalLSP == nil {
		silkideToast(i18n.T("LSP not running"), gui.ToastWarning)
		return
	}

	dlg := gui.NewDialog(i18n.T("Go to Symbol in Workspace"), parent)
	box := gui.NewVBox()
	box.SetSpacing(6)

	input := gui.NewEdit()
	box.AddWidget(input)

	list := gui.NewListWidget()
	list.SetSelectionVisible(true)
	box.AddWidget(list)

	// gen guards against out-of-order async results: only the response whose
	// generation still matches the latest query is applied.
	var gen int
	query := func(q string) {
		gen++
		myGen := gen
		lsp := globalLSP
		if lsp == nil {
			return
		}
		go func() {
			syms, err := lsp.WorkspaceSymbol(q)
			if err != nil {
				return
			}
			gui.Post(func() {
				if myGen != gen {
					return // a newer keystroke superseded this query
				}
				list.Clear()
				for _, s := range syms {
					label := s.Name
					if s.ContainerName != "" {
						label = s.ContainerName + "." + s.Name
					}
					list.Append(gui.ListItem{Text: label, Data: s})
				}
			})
		}()
	}
	query("")

	input.SigTextChanged(func(_ interface{}, q string) { query(q) })

	openSelected := func(idx int) {
		if idx < 0 || idx >= list.Count() {
			return
		}
		s, ok := list.Item(idx).Data.(core.LSPWorkspaceSymbol)
		if !ok {
			return
		}
		dismissDialog(dlg)
		if tabs != nil {
			// LSPWorkspaceSymbol.Line is 0-based; openFileInEditorAt wants
			// 1-based. URI → filesystem path via the shared uriToPath.
			openFileInEditorAt(tabs, uriToPath(s.URI), s.Line+1, s.Character)
		}
	}

	// Enter on the search field opens the top match.
	input.SigSubmit(func(_ interface{}, _ string) { openSelected(0) })
	// Enter / double-click on the list opens the selected entry.
	list.SigSubmit(func(o interface{}) { openSelected(list.ActiveIndex()) })

	dlg.SetContent(box)
	dlg.AddButton(i18n.T("Cancel"), gui.DialogCancel)
	dlg.SetSize(560, 480)
	input.SetFocus()
	dlg.ShowModal()
}

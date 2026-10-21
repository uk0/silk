package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// Quick Open, the command palette and Go to Symbol all rebuild their list on
// every keystroke, and ListWidget.Clear resets the selection to -1. Nothing was
// putting it back, so the dialogs opened with no row highlighted and Enter on
// the list — which runs list.ActiveIndex() — did nothing at all until the user
// clicked a row. fillFinderList is the one seam all three now fill through.

// TestFillFinderListSelectsTheTopRow: after a fill the first row is the
// selected one, whatever was selected before. The identity check is the point —
// a list refilled while row 2 was selected has to come back on the row the new
// ranking put on top, not on whatever now happens to sit at the old index.
func TestFillFinderListSelectsTheTopRow(t *testing.T) {
	list := gui.NewListWidget()

	fillFinderList(list, []gui.ListItem{
		{Text: "main.go"}, {Text: "gui/button.go"}, {Text: "gui/list-widget.go"},
	})
	if got := list.ActiveIndex(); got != 0 {
		t.Fatalf("after fill: ActiveIndex = %d, want 0 (nothing highlighted, Enter dead)", got)
	}
	if got, want := list.Item(list.ActiveIndex()).Text, "main.go"; got != want {
		t.Errorf("selected row = %q, want the first item passed in, %q", got, want)
	}

	// The user arrows down to the third row, then types another character: the
	// refill has to move the selection back to the top of the new ranking.
	list.SetActiveIndex(2)
	fillFinderList(list, []gui.ListItem{
		{Text: "core/uuid.go"}, {Text: "core/log.go"}, {Text: "core/factory.go"},
	})
	if got := list.ActiveIndex(); got != 0 {
		t.Fatalf("after refill: ActiveIndex = %d, want 0", got)
	}
	if got, want := list.Item(list.ActiveIndex()).Text, "core/uuid.go"; got != want {
		t.Errorf("selected row after refill = %q, want the first item of the new results, %q", got, want)
	}
}

// TestFillFinderListWithNoMatchesSelectsNothing: a query that matches nothing
// leaves an empty list, and "nothing selected" is the only honest answer — row
// 0 of an empty list is a row Item() panics on.
func TestFillFinderListWithNoMatchesSelectsNothing(t *testing.T) {
	list := gui.NewListWidget()
	fillFinderList(list, []gui.ListItem{{Text: "main.go"}})
	fillFinderList(list, nil)

	if got := list.ActiveIndex(); got != -1 {
		t.Errorf("after filling with 0 items: ActiveIndex = %d, want -1", got)
	}
}

// TestFillFinderListKeepsEveryRowsPayload: the label is only half a row. Each
// finder hangs the value its handler needs off ListItem.Data and gets it back
// with a type assertion — showCommandPalette reads .(paletteCommand),
// showFileFinder .(fileEntry), showSymbolFinder .(core.LSPWorkspaceSymbol) —
// and a failed assertion is silent: ok comes back false and the handler
// returns. A fill that carried the text across and dropped Data would leave all
// three dialogs looking alive (rows drawn, row 0 lit, ActiveIndex 0) with
// Enter, double-click and Enter-on-the-search-field all doing nothing. So this
// reruns each finder's own assertion over the rows fillFinderList produced.
func TestFillFinderListKeepsEveryRowsPayload(t *testing.T) {
	// fired records which command's Fn ran, so the palette case can prove the
	// closure survived too — Name and Hint matching is not enough when Fn is
	// the field that does the work.
	fired := ""
	cmds := []paletteCommand{
		{Name: "Run", Hint: "F5", Fn: func() { fired = "Run" }},
		{Name: "Build", Hint: "F6", Fn: func() { fired = "Build" }},
	}
	files := []fileEntry{
		{Path: "/proj/main.go", Display: "main.go"},
		{Path: "/proj/gui/list-widget.go", Display: "gui/list-widget.go"},
	}
	syms := []core.LSPWorkspaceSymbol{
		{Name: "NewListWidget", ContainerName: "gui", URI: "file:///proj/gui/list-widget.go", Line: 240},
		{Name: "Append", ContainerName: "ListWidget", URI: "file:///proj/gui/list-widget.go", Line: 278, Character: 22},
	}

	cases := []struct {
		finder string // the dialog that reads these rows back
		assert string // the assertion its handler performs on Data
		items  []gui.ListItem
		// check reruns that assertion on row idx of the filled list and says
		// what is wrong with the payload, or "" when it came through intact.
		check func(list *gui.ListWidget, idx int) string
	}{
		{
			finder: "showCommandPalette",
			assert: "list.Item(idx).Data.(paletteCommand)",
			items: []gui.ListItem{
				{Text: "Run    F5", Data: cmds[0]},
				{Text: "Build    F6", Data: cmds[1]},
			},
			check: func(list *gui.ListWidget, idx int) string {
				cmd, ok := list.Item(idx).Data.(paletteCommand)
				if !ok {
					return fmt.Sprintf("Data is %T, not a paletteCommand", list.Item(idx).Data)
				}
				if cmd.Name != cmds[idx].Name || cmd.Hint != cmds[idx].Hint {
					return fmt.Sprintf("Data is command %q (%q), want %q (%q)",
						cmd.Name, cmd.Hint, cmds[idx].Name, cmds[idx].Hint)
				}
				fired = ""
				cmd.Fn()
				if fired != cmds[idx].Name {
					return fmt.Sprintf("running the command fired %q, want %q", fired, cmds[idx].Name)
				}
				return ""
			},
		},
		{
			finder: "showFileFinder",
			assert: "list.Item(idx).Data.(fileEntry)",
			items: []gui.ListItem{
				{Text: files[0].Display, Data: files[0]},
				{Text: files[1].Display, Data: files[1]},
			},
			check: func(list *gui.ListWidget, idx int) string {
				f, ok := list.Item(idx).Data.(fileEntry)
				if !ok {
					return fmt.Sprintf("Data is %T, not a fileEntry", list.Item(idx).Data)
				}
				if f != files[idx] {
					return fmt.Sprintf("Data is %+v, want %+v", f, files[idx])
				}
				return ""
			},
		},
		{
			finder: "showSymbolFinder",
			assert: "list.Item(idx).Data.(core.LSPWorkspaceSymbol)",
			items: []gui.ListItem{
				{Text: "gui.NewListWidget", Data: syms[0]},
				{Text: "ListWidget.Append", Data: syms[1]},
			},
			check: func(list *gui.ListWidget, idx int) string {
				s, ok := list.Item(idx).Data.(core.LSPWorkspaceSymbol)
				if !ok {
					return fmt.Sprintf("Data is %T, not a core.LSPWorkspaceSymbol", list.Item(idx).Data)
				}
				if s != syms[idx] {
					return fmt.Sprintf("Data is %+v, want %+v", s, syms[idx])
				}
				return ""
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.finder, func(t *testing.T) {
			list := gui.NewListWidget()
			fillFinderList(list, tc.items)

			if got, want := list.Count(), len(tc.items); got != want {
				t.Fatalf("fill left %d rows, want %d", got, want)
			}
			for idx := range tc.items {
				if bad := tc.check(list, idx); bad != "" {
					t.Errorf("row %d (%q): %s; %s reads %s and returns without doing anything when that fails",
						idx, list.Item(idx).Text, bad, tc.finder, tc.assert)
				}
			}

			// Enter and double-click both run whatever row is selected, so the
			// payload has to be intact at *that* index — a fill that preserved
			// every payload but came back with nothing selected is the same
			// dead Enter reached from the other side.
			idx := list.ActiveIndex()
			if idx < 0 || idx >= list.Count() {
				t.Fatalf("nothing selected after the fill (ActiveIndex = %d); %s reads that index", idx, tc.finder)
			}
			if bad := tc.check(list, idx); bad != "" {
				t.Errorf("selected row %d (%q) is the one Enter runs: %s", idx, list.Item(idx).Text, bad)
			}
		})
	}
}

// finderDialogs maps each finder that rebuilds a ListWidget to the file it
// lives in.
var finderDialogs = map[string]string{
	"newCommandPalette": "palette.go",
	"newFileFinder":     "file_finder.go",
	"newSymbolFinder":   "symbol_finder.go",
}

// TestEveryFinderFillsThroughFillFinderList: the test above proves the seam
// works, not that the dialogs use it — converting two of the three would leave
// the third opening with a dead Enter and the package still green. So this
// reads the list-construction functions out of the syntax tree (the palette
// now has a separately testable builder) and pins that each one fills through
// fillFinderList and none of them clears the list behind its back.
func TestEveryFinderFillsThroughFillFinderList(t *testing.T) {
	for name, file := range finderDialogs {
		fn := findFuncDecl(t, file, name)
		if !callsFunc(fn.Body, "fillFinderList") {
			t.Errorf("%s (%s) never calls fillFinderList; its list opens with no row selected and Enter on it does nothing", name, file)
		}
		if recv := clearedReceiver(fn.Body); recv != "" {
			t.Errorf("%s (%s) calls %s.Clear() directly; Clear drops the selection to -1, which is exactly what fillFinderList exists to put back", name, file, recv)
		}
	}
}

// findFuncDecl parses file (relative to the package dir, the way the test runs)
// and returns the named top-level function.
func findFuncDecl(t *testing.T, file, name string) *ast.FuncDecl {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("cannot parse %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == name && fn.Body != nil {
			return fn
		}
	}
	t.Fatalf("%s has no func %s; update finderDialogs to match", file, name)
	return nil
}

// callsFunc reports whether body calls the plain (non-method) function name,
// including from inside a closure — showSymbolFinder fills its list from a
// gui.Post callback nested in a goroutine.
func callsFunc(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		if id, ok := call.Fun.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// clearedReceiver returns the name of the first variable body calls Clear() on,
// or "" when it calls none.
func clearedReceiver(body *ast.BlockStmt) (recv string) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return recv == ""
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Clear" {
			return recv == ""
		}
		if id, ok := sel.X.(*ast.Ident); ok {
			recv = id.Name
		} else {
			recv = "it"
		}
		return false
	})
	return
}

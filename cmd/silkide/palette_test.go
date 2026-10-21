package main

import (
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"strings"
	"testing"
)

// TestFilterCommandsSubsequence: filterCommands matches command
// names whose runes contain the query as a subsequence — VSCode's
// "find action" semantics. "fb" matches both "Build" and "Find in
// Files" because both contain f…b in order.
func TestFilterCommandsSubsequence(t *testing.T) {
	cmds := []paletteCommand{
		{Name: "Build"},
		{Name: "Run"},
		{Name: "Find in Files"},
		{Name: "Refresh"},
	}

	cases := []struct {
		query string
		want  []string
	}{
		{"", []string{"Build", "Run", "Find in Files", "Refresh"}}, // empty query passes all
		{"build", []string{"Build"}},
		{"run", []string{"Run"}},
		{"fnf", []string{"Find in Files"}}, // f→n→f subsequence in "find in files"
		{"rfh", []string{"Refresh"}},       // r→f→h subsequence in "refresh"
		{"un", []string{"Run"}},            // shorter first when both Run+Find match? Run only
		{"xyz", nil},                       // no match
	}
	for _, c := range cases {
		got := filterCommands(cmds, c.query)
		if len(got) != len(c.want) {
			t.Errorf("filterCommands(%q) returned %d items, want %d:\n got %v\nwant %v",
				c.query, len(got), len(c.want), names(got), c.want)
			continue
		}
		for i, g := range got {
			if g.Name != c.want[i] {
				t.Errorf("filterCommands(%q)[%d] = %q, want %q",
					c.query, i, g.Name, c.want[i])
			}
		}
	}
}

// TestFilterCommandsCaseInsensitive: query and command names
// shouldn't have to match case. The filter lower-cases both before
// running the subsequence check.
func TestFilterCommandsCaseInsensitive(t *testing.T) {
	cmds := []paletteCommand{{Name: "Build"}}
	for _, q := range []string{"build", "BUILD", "BuIlD", "uIl"} {
		got := filterCommands(cmds, q)
		if len(got) != 1 {
			t.Errorf("filterCommands(%q) = %v, want 1 hit", q, names(got))
		}
	}
}

// TestSubsequenceMatchEdgeCases: empty query always matches; empty
// text fails any non-empty query.
func TestSubsequenceMatchEdgeCases(t *testing.T) {
	if !subsequenceMatch("anything", "") {
		t.Error("empty query should match any text")
	}
	if !subsequenceMatch("", "") {
		t.Error("empty query against empty text should still match")
	}
	if subsequenceMatch("", "x") {
		t.Error("empty text shouldn't match a non-empty query")
	}
	// Order-sensitive: "ba" must NOT match "abracadabra" the way
	// "ab" does — query order matters.
	if subsequenceMatch("abracadabra", "zz") {
		t.Error("subsequence with no occurrence should fail")
	}
}

// TestRegisterPaletteCommandsPopulatesList: registerPaletteCommands
// pushes every documented action into paletteCommands. Locks the
// minimum count so a regression that drops actions surfaces as a
// failing test.
func TestRegisterPaletteCommandsPopulatesList(t *testing.T) {
	saved := paletteCommands
	defer func() { paletteCommands = saved }()
	paletteCommands = nil

	registerPaletteCommands(nil, nil)

	if len(paletteCommands) < 10 {
		t.Errorf("paletteCommands has %d entries, want ≥10", len(paletteCommands))
	}
	// Every entry needs a non-empty name (the search target).
	for i, c := range paletteCommands {
		if strings.TrimSpace(c.Name) == "" {
			t.Errorf("paletteCommands[%d] has empty Name", i)
		}
		if c.Fn == nil {
			t.Errorf("paletteCommands[%d] (%q) has nil Fn", i, c.Name)
		}
	}
}

func names(cmds []paletteCommand) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.Name
	}
	return out
}

func TestExplorerPaletteCommandsAndHiddenToggle(t *testing.T) {
	saved, tree := paletteCommands, globalFileExplorer
	t.Cleanup(func() { paletteCommands = saved; globalFileExplorer = tree })
	globalFileExplorer = ged.NewFileExplorer()
	registerPaletteCommands(nil, nil)
	for _, name := range []string{"Show Files", "Filter Project Files", "Refresh Project Files", "Toggle Hidden Files", "Reveal Active File in Explorer"} {
		found := false
		for _, cmd := range paletteCommands {
			if cmd.Name == i18n.T(name) {
				found = true
				if name == "Toggle Hidden Files" {
					cmd.Fn()
					if !globalFileExplorer.ShowHidden() {
						t.Fatal("toggle did not enable")
					}
					cmd.Fn()
					if globalFileExplorer.ShowHidden() {
						t.Fatal("toggle did not disable")
					}
				}
			}
		}
		if !found {
			t.Fatal("missing command", name)
		}
	}
}

func TestPaletteSubmitsOnlyAfterModalUnwind(t *testing.T) {
	old := paletteCommands
	t.Cleanup(func() { paletteCommands = old })
	for _, from := range []string{"input", "list", "cancel"} {
		t.Run(from, func(t *testing.T) {
			first, second := 0, 0
			paletteCommands = []paletteCommand{{Name: "first", Fn: func() { first++ }}, {Name: "second", Fn: func() { second++ }}}
			dlg, selection := newCommandPalette(gui.NewFrame())
			var input *gui.Edit
			var list *gui.ListWidget
			var visit func(gui.IWidget)
			visit = func(w gui.IWidget) {
				if e, ok := w.(*gui.Edit); ok {
					input = e
				}
				if l, ok := w.(*gui.ListWidget); ok {
					list = l
				}
				for _, c := range w.Children() {
					visit(c)
				}
			}
			visit(dlg)
			if input == nil || list == nil {
				t.Fatal("palette input/list missing")
			}
			if list.ActiveIndex() != 0 {
				t.Fatal("palette initial selection missing")
			}
			input.SetText("sec")
			if list.Count() != 1 || list.ActiveIndex() != 0 || list.Item(0).Data.(paletteCommand).Name != "second" {
				t.Fatal("filter lost selection/payload")
			}
			input.SetText("")
			if from == "list" {
				list.SetActiveIndex(1)
				list.OnKeyDown(gui.KeyEnter, false)
			} else {
				input.OnKeyDown(gui.KeyEnter, false)
			}
			if first != 0 || second != 0 || selection.command == nil {
				t.Fatal("command ran before native modal teardown")
			}
			// Repeated submit events before the loop exits must not execute twice.
			input.OnKeyDown(gui.KeyEnter, false)
			result := gui.DialogOK
			if from == "cancel" {
				result = gui.DialogCancel
			}
			selection.run(result)
			selection.run(gui.DialogOK)
			if from == "input" && (first != 1 || second != 0) || from == "list" && (first != 0 || second != 1) || from == "cancel" && (first != 0 || second != 0) {
				t.Fatalf("wrong command / duplicate: %d %d", first, second)
			}
		})
	}
}

func TestCommandPaletteHasVisibleResultsViewport(t *testing.T) {
	old := paletteCommands
	defer func() { paletteCommands = old }()
	paletteCommands = []paletteCommand{{Name: "Only result", Fn: func() {}}}
	dlg, _ := newCommandPalette(nil)
	dlg.Layout()
	box := dlg.Content().(*gui.VBox)
	box.Layout()
	var list *gui.ListWidget
	for _, c := range box.Children() {
		if v, ok := c.(*gui.ListWidget); ok {
			list = v
		}
	}
	if list == nil || !list.FillAvailable() || list.Height() < 200 || list.Width() != box.Width() {
		t.Fatal("palette results are collapsed or clipped")
	}
	if list.Y()+list.Height() > box.Height() {
		t.Fatal("palette list covers dialog footer")
	}
	fillFinderList(list, nil)
	box.Layout()
	if list.Height() < 200 {
		t.Fatal("empty query collapsed list before async refill")
	}
}

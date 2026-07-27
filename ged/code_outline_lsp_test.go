package ged

import (
	"fmt"
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
)

// GL-free: every test here drives the outline panel's model only —
// SetSymbols / filter / sort / revision plumbing plus the flat row list.
// Nothing constructs a Frame, calls Draw or measures a font.

// sampleOutline is a representative semantic outline as a host would hand
// it over after converting a documentSymbol response. It deliberately mixes
// three nesting levels, exported and unexported members, and a declaration
// order that differs from alphabetical order so the sort modes are
// distinguishable:
//
//	Server   (10)  exported struct
//	  Handler   (12)  exported field
//	    ServeHTTP (13)  exported method
//	  addr      (30)  unexported field
//	Start    (50)  exported method
//	config   (70)  unexported struct
//	  Debug     (71)  exported field
//	helper   (90)  unexported func
func sampleOutline() []OutlineSymbol {
	return []OutlineSymbol{
		{Name: "Server", Kind: "Struct", Detail: "struct{...}", Line: 10, EndLine: 40, Exported: true, Children: []OutlineSymbol{
			{Name: "Handler", Kind: "Field", Line: 12, EndLine: 12, Exported: true, Children: []OutlineSymbol{
				{Name: "ServeHTTP", Kind: "Method", Line: 13, EndLine: 20, Exported: true},
			}},
			{Name: "addr", Kind: "Field", Line: 30, EndLine: 30},
		}},
		{Name: "Start", Kind: "Method", Line: 50, EndLine: 60, Exported: true},
		{Name: "config", Kind: "Struct", Line: 70, EndLine: 75, Children: []OutlineSymbol{
			{Name: "Debug", Kind: "Field", Line: 71, EndLine: 71, Exported: true},
		}},
		{Name: "helper", Kind: "Function", Line: 90, EndLine: 95},
	}
}

// outlineRows renders the visible flat list as "depth:Name" strings, which
// captures both the row order and the tree shape in one comparable value.
func outlineRows(p *CodeOutlinePanel) []string {
	out := make([]string, 0, len(p.flatList))
	for _, node := range p.flatList {
		out = append(out, fmt.Sprintf("%d:%s", node.depth, node.symbol.Name))
	}
	return out
}

func wantOutlineRows(t *testing.T, p *CodeOutlinePanel, want []string) {
	t.Helper()
	got := outlineRows(p)
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Errorf("rows =\n  %s\nwant\n  %s", strings.Join(got, " | "), strings.Join(want, " | "))
	}
}

// TestOutlineSetSymbolsBuildsNestedTree checks the semantic hierarchy is
// kept at every depth (not flattened to types + methods) and that the
// provider-agnostic kind names land on the right gui.Sym* enum.
func TestOutlineSetSymbolsBuildsNestedTree(t *testing.T) {
	p := NewCodeOutlinePanel()
	p.SetSymbols(sampleOutline())

	wantOutlineRows(t, p, []string{
		"0:Server", "1:Handler", "2:ServeHTTP", "1:addr",
		"0:Start", "0:config", "1:Debug", "0:helper",
	})

	kinds := map[string]int{}
	ends := map[string]int{}
	for _, node := range p.flatList {
		kinds[node.symbol.Name] = node.symbol.Kind
		ends[node.symbol.Name] = node.endLine
	}
	for name, want := range map[string]int{
		"Server":    gui.SymType,
		"Handler":   gui.SymVar,
		"ServeHTTP": gui.SymMethod,
		"Start":     gui.SymMethod,
		"helper":    gui.SymFunc,
	} {
		if kinds[name] != want {
			t.Errorf("%s kind = %d, want %d", name, kinds[name], want)
		}
	}
	if ends["ServeHTTP"] != 20 {
		t.Errorf("ServeHTTP endLine = %d, want 20 (range dropped)", ends["ServeHTTP"])
	}
	if got := p.flatList[0].symbol.Detail; got != "struct{...}" {
		t.Errorf("Server detail = %q, want the provider's detail", got)
	}
}

// TestOutlineKindMapping pins the kind-name table, including the LSP
// spellings and the unknown-kind default.
func TestOutlineKindMapping(t *testing.T) {
	cases := []struct {
		kind string
		want int
	}{
		{"func", gui.SymFunc},
		{"Function", gui.SymFunc},
		{"", gui.SymFunc},
		{"nonsense", gui.SymFunc},
		{"type", gui.SymType},
		{"Interface", gui.SymType},
		{" class ", gui.SymType},
		{"Method", gui.SymMethod},
		{"constructor", gui.SymMethod},
		{"var", gui.SymVar},
		{"Field", gui.SymVar},
		{"const", gui.SymConst},
		{"EnumMember", gui.SymConst},
	}
	for _, c := range cases {
		if got := outlineKindOf(c.kind); got != c.want {
			t.Errorf("outlineKindOf(%q) = %d, want %d", c.kind, got, c.want)
		}
	}
}

// TestOutlineSetSymbolsCopySemantics verifies the panel owns its input:
// mutating the slice (or a nested child) after SetSymbols must not change
// what is displayed.
func TestOutlineSetSymbolsCopySemantics(t *testing.T) {
	p := NewCodeOutlinePanel()
	in := sampleOutline()
	p.SetSymbols(in)

	in[0].Name = "MUTATED"
	in[0].Children[0].Children[0].Name = "MUTATED-DEEP"

	wantOutlineRows(t, p, []string{
		"0:Server", "1:Handler", "2:ServeHTTP", "1:addr",
		"0:Start", "0:config", "1:Debug", "0:helper",
	})
}

// TestOutlineFilterNarrows checks the name filter keeps matches plus their
// ancestors as context, and that clearing it restores the full tree.
func TestOutlineFilterNarrows(t *testing.T) {
	p := NewCodeOutlinePanel()
	p.SetSymbols(sampleOutline())

	// A hit two levels deep drags its parents along.
	p.SetFilter("http")
	wantOutlineRows(t, p, []string{"0:Server", "1:Handler", "2:ServeHTTP"})
	if p.Filter() != "http" {
		t.Errorf("Filter() = %q, want %q", p.Filter(), "http")
	}

	// Case-insensitive, and a top-level-only match drops everything else.
	p.SetFilter("START")
	wantOutlineRows(t, p, []string{"0:Start"})

	// A parent match does not drag unmatched children along.
	p.SetFilter("config")
	wantOutlineRows(t, p, []string{"0:config"})

	// No match at all: empty outline, no panic.
	p.SetFilter("zzz-nothing")
	wantOutlineRows(t, p, nil)

	p.SetFilter("")
	wantOutlineRows(t, p, []string{
		"0:Server", "1:Handler", "2:ServeHTTP", "1:addr",
		"0:Start", "0:config", "1:Debug", "0:helper",
	})
}

// TestOutlineSortModes checks byPosition is declaration order and byName is
// alphabetical at every level, not just the top one.
func TestOutlineSortModes(t *testing.T) {
	p := NewCodeOutlinePanel()
	p.SetSymbols(sampleOutline())

	if p.SortMode() != OutlineSortByPosition {
		t.Errorf("default SortMode() = %d, want OutlineSortByPosition", p.SortMode())
	}
	wantOutlineRows(t, p, []string{
		"0:Server", "1:Handler", "2:ServeHTTP", "1:addr",
		"0:Start", "0:config", "1:Debug", "0:helper",
	})

	p.SetSortMode(OutlineSortByName)
	if p.SortMode() != OutlineSortByName {
		t.Errorf("SortMode() = %d, want OutlineSortByName", p.SortMode())
	}
	// Top level: config < helper < Server < Start (case-insensitive).
	// Server's children flip to addr < Handler, proving the recursion.
	wantOutlineRows(t, p, []string{
		"0:config", "1:Debug", "0:helper",
		"0:Server", "1:addr", "1:Handler", "2:ServeHTTP",
		"0:Start",
	})

	p.SetSortMode(OutlineSortByPosition)
	wantOutlineRows(t, p, []string{
		"0:Server", "1:Handler", "2:ServeHTTP", "1:addr",
		"0:Start", "0:config", "1:Debug", "0:helper",
	})
}

// TestOutlineExportedOnly checks the visibility toggle hides unexported
// symbols but keeps an unexported parent that owns an exported child.
func TestOutlineExportedOnly(t *testing.T) {
	p := NewCodeOutlinePanel()
	p.SetSymbols(sampleOutline())

	p.SetExportedOnly(true)
	if !p.ExportedOnly() {
		t.Error("ExportedOnly() = false after SetExportedOnly(true)")
	}
	// addr and helper go; config stays only as Debug's parent.
	wantOutlineRows(t, p, []string{
		"0:Server", "1:Handler", "2:ServeHTTP",
		"0:Start", "0:config", "1:Debug",
	})

	// Composes with the filter.
	p.SetFilter("e")
	wantOutlineRows(t, p, []string{"0:Server", "1:Handler", "2:ServeHTTP", "0:config", "1:Debug"})
	p.SetFilter("")

	p.SetExportedOnly(false)
	wantOutlineRows(t, p, []string{
		"0:Server", "1:Handler", "2:ServeHTTP", "1:addr",
		"0:Start", "0:config", "1:Debug", "0:helper",
	})
}

// TestOutlineFallbackToHeuristicParse checks that with no semantic symbols
// the panel still parses the bound editor (types owning their receiver
// methods), and that dropping the semantic outline returns to that path.
func TestOutlineFallbackToHeuristicParse(t *testing.T) {
	ed := gui.NewCodeEditor()
	ed.SetText(`package p

type Server struct{}

func (s *Server) Start() {}

func (s *Server) stop() {}

func main() {}
`)

	p := NewCodeOutlinePanel()
	p.SetEditor(ed)
	heuristic := []string{"0:Server", "1:Start", "1:stop", "0:main"}
	wantOutlineRows(t, p, heuristic)

	// Exported-only works on the heuristic path too: the Go rule is applied
	// to parsed names, and Server survives for Start's sake.
	p.SetExportedOnly(true)
	wantOutlineRows(t, p, []string{"0:Server", "1:Start"})
	p.SetExportedOnly(false)

	// Semantic symbols take over ...
	p.SetSymbols(sampleOutline())
	if got := outlineRows(p); len(got) != 8 || got[0] != "0:Server" || got[2] != "2:ServeHTTP" {
		t.Fatalf("semantic outline did not take over: %v", got)
	}

	// ... and handing back nil returns to the heuristic scan.
	p.SetSymbols(nil)
	wantOutlineRows(t, p, heuristic)
}

// TestOutlineRevisionInvalidation is the regression: a same-length,
// same-line-count edit leaves the size fingerprint untouched, so nothing
// short of a revision bump can invalidate the outline.
func TestOutlineRevisionInvalidation(t *testing.T) {
	ed := gui.NewCodeEditor()
	ed.SetText("package p\n\nfunc aaa() {}\n")

	p := NewCodeOutlinePanel()
	p.SetEditor(ed)
	wantOutlineRows(t, p, []string{"0:aaa"})

	if p.RefreshIfStale() {
		t.Error("RefreshIfStale re-parsed an unchanged buffer")
	}

	// Same byte count, same line count, different symbol.
	ed.SetText("package p\n\nfunc bbb() {}\n")
	if p.RefreshIfStale() {
		t.Error("RefreshIfStale claimed a refresh without a revision change")
	}
	wantOutlineRows(t, p, []string{"0:aaa"}) // stale, exactly the bug being closed

	p.SetRevision(7)
	if !p.RefreshIfStale() {
		t.Fatal("revision change did not invalidate the outline")
	}
	wantOutlineRows(t, p, []string{"0:bbb"})

	// The refresh consumed the revision: no repeat work next frame.
	if p.RefreshIfStale() {
		t.Error("RefreshIfStale refreshed twice for one revision")
	}
	// Re-setting the same revision is not a change.
	p.SetRevision(7)
	if p.RefreshIfStale() {
		t.Error("re-setting the same revision invalidated the outline")
	}

	// The old length fingerprint still works on its own.
	ed.SetText("package p\n\nfunc bbb() {}\nfunc ccc() {}\n")
	if !p.RefreshIfStale() {
		t.Fatal("length change did not invalidate the outline")
	}
	wantOutlineRows(t, p, []string{"0:bbb", "0:ccc"})
}

// TestOutlineNestedExpandCollapse checks a click on a nested row toggles
// that row (not just top-level ones) and navigates to its line.
func TestOutlineNestedExpandCollapse(t *testing.T) {
	p := NewCodeOutlinePanel()
	p.SetSymbols(sampleOutline())

	var navigated []int
	p.SetNavigateCallback(func(line int) { navigated = append(navigated, line) })

	// Row 1 is Handler (depth 1, owns ServeHTTP): header 22 + row 22 + 5.
	p.OnLeftDown(4, 22+22+5)
	if len(navigated) != 1 || navigated[0] != 12 {
		t.Fatalf("navigate = %v, want [12] (Handler's line)", navigated)
	}
	wantOutlineRows(t, p, []string{
		"0:Server", "1:Handler", "1:addr",
		"0:Start", "0:config", "1:Debug", "0:helper",
	})

	// Clicking again re-expands it.
	p.OnLeftDown(4, 22+22+5)
	wantOutlineRows(t, p, []string{
		"0:Server", "1:Handler", "2:ServeHTTP", "1:addr",
		"0:Start", "0:config", "1:Debug", "0:helper",
	})
	if len(navigated) != 2 {
		t.Errorf("navigate = %v, want two entries", navigated)
	}

	// A leaf click only navigates. Row 2 is ServeHTTP.
	p.OnLeftDown(4, 22+2*22+5)
	if len(navigated) != 3 || navigated[2] != 13 {
		t.Fatalf("navigate = %v, want ServeHTTP's line 13 last", navigated)
	}
	if len(p.flatList) != 8 {
		t.Errorf("leaf click changed the row count: %v", outlineRows(p))
	}
}

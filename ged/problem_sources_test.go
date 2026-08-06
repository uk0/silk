package ged

import (
	"testing"
)

// locators is the list as a reader sees it: one entry per row, in pane order.
// Comparing locators rather than counts is what makes a merge that keeps the
// right number of the wrong rows fail.
func locators(list []Problem) []string {
	out := make([]string, 0, len(list))
	for _, p := range list {
		out = append(out, p.File)
	}
	return out
}

func sameLocators(got []Problem, want ...string) bool {
	g := locators(got)
	if len(g) != len(want) {
		return false
	}
	for i := range want {
		if g[i] != want[i] {
			return false
		}
	}
	return true
}

// TestLoadRowsSurviveABuild is the defect in one call: the 问题 pane had a
// single list every writer replaced, so a build — or any other failure routed
// through the same setter — deleted the warning that a design still open had
// already lost a widget on load.
func TestLoadRowsSurviveABuild(t *testing.T) {
	list := MergeProblems(nil, LoadSource("A.silkui"), LoadProblems([]string{"gui.Ghost"}))
	list = MergeProblems(list, BuildSource, ParseProblems("main.go:10:6: undefined: Foo\n"))

	if !sameLocators(list, "gui.Ghost", "main.go") {
		t.Fatalf("after the build the pane reads %v, want the load warning kept and the build row added", locators(list))
	}
}

// TestBuildRowsReplaceThePreviousBuildsAndNothingElse: a build's rows are a
// statement about this build, so the next one takes them away — and only them.
func TestBuildRowsReplaceThePreviousBuildsAndNothingElse(t *testing.T) {
	list := MergeProblems(nil, LoadSource("A.silkui"), LoadProblems([]string{"gui.Ghost"}))
	list = MergeProblems(list, BuildSource, ParseProblems("first.go:1:1: undefined: Foo\n"))
	list = MergeProblems(list, BuildSource, ParseProblems("second.go:2:2: undefined: Bar\n"))

	if !sameLocators(list, "gui.Ghost", "second.go") {
		t.Fatalf("pane reads %v, want the previous build's row gone and the load warning kept", locators(list))
	}

	// A build that passes files no rows at all, which must not reach further
	// than the build's own.
	list = MergeProblems(list, BuildSource, nil)
	if !sameLocators(list, "gui.Ghost") {
		t.Fatalf("after a clean build the pane reads %v, want only the load warning", locators(list))
	}
}

// TestClosingADocumentDropsItsRowsAndOnlyItsRows: closing a design retires
// everything filed against it — the widgets its load dropped and the ones it
// could not generate — while another document's rows and the build's stay.
func TestClosingADocumentDropsItsRowsAndOnlyItsRows(t *testing.T) {
	list := MergeProblems(nil, LoadSource("A.silkui"), LoadProblems([]string{"gui.Ghost"}))
	list = MergeProblems(list, DesignSource("A.silkui"), []Problem{{File: "ghost (未知类型)"}})
	list = MergeProblems(list, LoadSource("B.silkui"), LoadProblems([]string{"gui.Phantom"}))
	list = MergeProblems(list, BuildSource, ParseProblems("main.go:10:6: undefined: Foo\n"))

	list = DropDocProblems(list, "A.silkui")
	if !sameLocators(list, "gui.Phantom", "main.go") {
		t.Fatalf("after closing A the pane reads %v, want B's warning and the build row", locators(list))
	}
}

// TestDroppingAnUnsavedDocumentKeepsTheBuildRows: a design that has never been
// saved is filed under the empty key, which is also what a build row carries
// in Doc. Closing that design must not take the compiler's diagnostics with
// it — they are about .go files and nothing has changed about them.
func TestDroppingAnUnsavedDocumentKeepsTheBuildRows(t *testing.T) {
	list := MergeProblems(nil, DesignSource(""), []Problem{{File: "ghost (未知类型)"}})
	list = MergeProblems(list, BuildSource, ParseProblems("main.go:10:6: undefined: Foo\n"))

	list = DropDocProblems(list, "")
	if !sameLocators(list, "main.go") {
		t.Fatalf("closing an unsaved design left %v, want the build row alone", locators(list))
	}
}

// TestReopeningADocumentReplacesItsLoadRows: the rows are keyed by filename so
// a second open of the same file replaces the first open's, rather than
// stacking a duplicate warning beside it — and an open that finds nothing
// missing clears them.
func TestReopeningADocumentReplacesItsLoadRows(t *testing.T) {
	list := MergeProblems(nil, LoadSource("A.silkui"), LoadProblems([]string{"gui.Ghost"}))
	list = MergeProblems(list, LoadSource("A.silkui"), LoadProblems([]string{"gui.Ghost"}))
	if !sameLocators(list, "gui.Ghost") {
		t.Fatalf("reopening A left %v, want the one warning", locators(list))
	}

	list = MergeProblems(list, LoadSource("A.silkui"), LoadProblems(nil))
	if len(list) != 0 {
		t.Fatalf("reopening a design whose widget is now registered left %v, want nothing", locators(list))
	}
}

// TestLoadAndDesignRowsOfOneDocumentCoexist: the two describe different
// widgets — one never entered the scene, the other is in it and cannot be
// generated — so filing either must not take the other away. They share a
// document, which is why they cannot share a source.
func TestLoadAndDesignRowsOfOneDocumentCoexist(t *testing.T) {
	list := MergeProblems(nil, LoadSource("A.silkui"), LoadProblems([]string{"gui.Ghost"}))
	list = MergeProblems(list, DesignSource("A.silkui"), []Problem{{File: "broken (未知类型)"}})

	if !sameLocators(list, "gui.Ghost", "broken (未知类型)") {
		t.Fatalf("pane reads %v, want both of the document's rows", locators(list))
	}
}

// TestMergeStampsTheSourceOntoTheRows pins the invariant the rest rests on:
// the rows come back carrying the source they were filed under. A writer that
// hands over unstamped rows would never be able to find them again, and every
// "replacement" would append instead — the list growing without bound while
// each build's stale rows stayed on screen.
func TestMergeStampsTheSourceOntoTheRows(t *testing.T) {
	src := LoadSource("A.silkui")
	list := MergeProblems(nil, src, LoadProblems([]string{"gui.Ghost"}))
	if len(list) != 1 {
		t.Fatalf("MergeProblems returned %d rows, want 1", len(list))
	}
	if list[0].Source != src {
		t.Errorf("row filed under %+v came back stamped %+v", src, list[0].Source)
	}

	// The caller's own slice must not be stamped behind its back: LoadProblems
	// and DesignProblems build rows that callers reuse.
	rows := LoadProblems([]string{"gui.Ghost"})
	MergeProblems(nil, src, rows)
	if rows[0].Source != (ProblemSource{}) {
		t.Errorf("MergeProblems wrote %+v into the caller's rows", rows[0].Source)
	}
}

// TestAnUnfiledRowIsNotABuildRow: SetProblems still takes a raw list, so rows
// can reach the pane carrying no source at all. Had ProblemBuild been the zero
// kind, every one of them would have looked like the build's own and the next
// build would have deleted it — this file's own defect, reintroduced by a
// default value.
func TestAnUnfiledRowIsNotABuildRow(t *testing.T) {
	list := []Problem{{File: "hand-made.go"}}
	if got := MergeProblems(list, BuildSource, nil); !sameLocators(got, "hand-made.go") {
		t.Errorf("a build deleted a row nobody filed: %v", locators(got))
	}
}

// TestMergedRowsKeepTheirPlace: a rebuild replaces its rows where they were,
// so the pane does not reorder itself under a reader who is working through
// it. Appending instead would walk the build's rows to the bottom on every
// rebuild.
func TestMergedRowsKeepTheirPlace(t *testing.T) {
	list := MergeProblems(nil, BuildSource, ParseProblems("first.go:1:1: undefined: Foo\n"))
	list = MergeProblems(list, LoadSource("A.silkui"), LoadProblems([]string{"gui.Ghost"}))
	list = MergeProblems(list, BuildSource, ParseProblems("second.go:2:2: undefined: Bar\n"))

	if !sameLocators(list, "second.go", "gui.Ghost") {
		t.Fatalf("pane reads %v, want the rebuilt rows back in the build's old place", locators(list))
	}
}

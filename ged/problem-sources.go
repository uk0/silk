package ged

// Every row in the 问题 pane belongs to whoever filed it, and replacing rows is
// scoped to that one writer. The pane used to be a single list each writer
// replaced wholesale, so a failure with nothing to do with the open design — a
// file that would not open, a program that would not start — parsed to zero
// rows and took away the warning that a design still on screen had already
// lost a widget. The two statements have different lifetimes: a build
// describes the code the generator produced just now and the next build
// replaces it, while a load warning describes a document that is open and
// stays true until that document is closed or loaded again.

// ProblemKind is what a row is a statement about, and so what may replace it.
type ProblemKind int

const (
	// ProblemUnfiled is the zero value: a row that reached the pane without a
	// source, which SetProblems still allows. It is deliberately not a real
	// kind. Were ProblemBuild the zero instead, an unfiled row would be
	// indistinguishable from the build's own and the next build would delete
	// it — the very failure this file exists to end.
	ProblemUnfiled ProblemKind = iota
	// ProblemBuild is a compiler diagnostic. The next build replaces the lot.
	ProblemBuild
	// ProblemLoad is a widget the loader could not build and dropped. It
	// outlives any number of builds: the design in memory is still short that
	// widget, and a 保存 still makes the loss permanent.
	ProblemLoad
	// ProblemDesign is a widget in the scene the generator cannot build,
	// recomputed each time 预览/导出/运行 asks.
	ProblemDesign
)

// ProblemSource identifies the rows one writer owns. It is comparable, so a
// row is matched to its writer with ==. Doc is the design file the rows are
// about and is empty for a build, whose diagnostics name .go files rather than
// a design.
type ProblemSource struct {
	Kind ProblemKind
	Doc  string
}

// BuildSource is where compiler output is filed. Only one build runs at a
// time, so it needs no further identity.
var BuildSource = ProblemSource{Kind: ProblemBuild}

// LoadSource is where the widgets a load had to drop are filed. Keyed by
// filename rather than by scene, so opening the same design again replaces the
// rows the previous open left instead of stacking a second copy beside them.
func LoadSource(filename string) ProblemSource {
	return ProblemSource{Kind: ProblemLoad, Doc: filename}
}

// DesignSource is where the widgets a generate would drop are filed. A
// separate kind from LoadSource on the same document because the two describe
// different widgets — one never entered the scene, the other is in it — so
// reporting either must not take the other away.
func DesignSource(filename string) ProblemSource {
	return ProblemSource{Kind: ProblemDesign, Doc: filename}
}

// MergeProblems replaces exactly the rows filed under src, leaving every other
// source's rows where they were. The rows come back stamped with src, which is
// what lets the next call find them: an unstamped row can never be replaced
// and the list would only ever grow.
//
// The replacement lands where the source's first row was, so a rebuild does
// not shuffle the pane under the reader.
func MergeProblems(existing []Problem, src ProblemSource, rows []Problem) []Problem {
	filed := make([]Problem, len(rows))
	copy(filed, rows)
	for i := range filed {
		filed[i].Source = src
	}
	out := make([]Problem, 0, len(existing)+len(filed))
	placed := false
	for _, p := range existing {
		if p.Source == src {
			if !placed {
				out = append(out, filed...)
				placed = true
			}
			continue
		}
		out = append(out, p)
	}
	if !placed {
		out = append(out, filed...)
	}
	return out
}

// DropDocProblems removes every row about doc, whichever writer filed it: the
// document is closed, so neither the widgets its load dropped nor the ones it
// could not generate describe anything the user can still act on.
//
// Build rows survive whatever doc is. They carry no document, and an unsaved
// design's empty filename would otherwise sweep them up with it.
func DropDocProblems(existing []Problem, doc string) []Problem {
	var out []Problem
	for _, p := range existing {
		if p.Source.Kind != ProblemBuild && p.Source.Doc == doc {
			continue
		}
		out = append(out, p)
	}
	return out
}

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// commandHandlers names every function in design.go that a menu item, a
// toolbar button or a registered shortcut invokes directly. They are the
// functions a user can reach, so they are the ones that must never decline in
// silence.
var commandHandlers = []string{
	"onFileNew", "onFileOpen", "openDesignFile", "onFileClose",
	"onFileSave", "onFileSaveAs", "onSaveAsTemplate",
	"onUndo", "onRedo", "onDelete", "onSelectAll",
	"onCut", "onCopy", "onPaste", "onDuplicate",
	"alignSelection", "sameSizeSelection", "bringToFront", "sendToBack",
	"applyContainerLayout", "breakLayout",
	"onPreview", "onExportCode", "onRun",
	"onFormSettings", "onTagDictionary", "onToggleGuides",
}

// reporters are the calls that put a reason where the user can read it: the
// status bar for the ordinary "nothing to act on" cases, a dialog for the ones
// that lose work. core.Warn and core.Error are deliberately absent — a log line
// nobody has open is the silence this test exists to catch.
//
// requireDesign / requireSelection are here because they report on the path
// that returns nil; TestGuardHelpersReport pins that so the delegation below
// cannot become a way to smuggle a silent guard past this test.
var reporters = map[string]bool{
	"reportDeclined":       true,
	"reportFailure":        true,
	"reportBuildFailure":   true, // files the build's 问题 rows, then reportFailure
	"reportLostWork":       true,
	"reportDesignProblems": true,
	"setStatus":            true,
	"requireDesign":        true,
	"requireSelection":     true,
	"ShowMessage":          true,
	"ShowMessageDialog":    true,
	"ShowConfirmDialog":    true,
}

// guardHelpers must each decline out loud, since the guards that call them
// return in silence on the strength of it.
var guardHelpers = []string{"requireDesign", "requireSelection"}

// cancelGuards are the conditions a command is allowed to return from without
// a word, because the user just dismissed a modal and already knows nothing
// happened. The list is deliberately exhaustive and literal: a new silent
// guard has to be added here on purpose, which is the point.
var cancelGuards = map[string]bool{
	`filename == ""`:    true, // OpenFileDialog / SaveFileDialog cancelled
	`!ok || name == ""`: true, // ShowInputBox cancelled
	`!ok`:               true, // ShowInputBox cancelled
}

// TestNoCommandDeclinesInSilence walks the command handlers and fails on a
// guard that returns without saying anything. "Nothing happened and nothing
// was said" is the defect: a menu item that looks live, does nothing when
// clicked and leaves no trace anywhere the user can see.
func TestNoCommandDeclinesInSilence(t *testing.T) {
	src, err := os.ReadFile("design.go")
	if err != nil {
		t.Fatalf("cannot read design.go: %v", err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "design.go", src, 0)
	if err != nil {
		t.Fatalf("cannot parse design.go: %v", err)
	}

	want := map[string]bool{}
	for _, name := range commandHandlers {
		want[name] = true
	}

	var silent []string
	seen := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !want[fn.Name.Name] {
			continue
		}
		seen[fn.Name.Name] = true
		for _, g := range silentGuards(fset, src, fn.Body) {
			silent = append(silent, fn.Name.Name+" "+g)
		}
		// A handler whose entire body hangs off one conditional has no guard
		// to inspect at all: when the condition is false the command simply
		// does not run, which is the same silence wearing a different shape.
		if len(fn.Body.List) == 1 {
			if ifs, ok := fn.Body.List[0].(*ast.IfStmt); ok && ifs.Else == nil {
				pos := fset.Position(ifs.Pos())
				silent = append(silent, fn.Name.Name+" design.go:"+strconv.Itoa(pos.Line)+
					" whole body wrapped in if "+exprText(fset, src, ifs.Cond))
			}
		}
	}

	for _, name := range commandHandlers {
		if !seen[name] {
			t.Errorf("design.go has no func %s; update commandHandlers to match the menus", name)
		}
	}

	if len(silent) > 0 {
		sort.Strings(silent)
		t.Errorf("%d command guard(s) return without telling the user why:\n  %s",
			len(silent), strings.Join(silent, "\n  "))
	}
}

// silentGuards returns "design.go:LINE if COND" for every guard in body whose
// last statement is a bare return and which says nothing anywhere the user can
// see: not in the guard, not in the guard's own condition, and not in the
// helper the value under test came from.
func silentGuards(fset *token.FileSet, src []byte, body *ast.BlockStmt) []string {
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		// Names bound from a helper that reports on its own failing path, so
		// `gv, _ := requireDesign(); if gv == nil { return }` is not silent.
		delegated := map[string]bool{}
		for _, stmt := range block.List {
			if as, ok := stmt.(*ast.AssignStmt); ok {
				if reportsSomething(as) {
					for _, lhs := range as.Lhs {
						if id, ok := lhs.(*ast.Ident); ok {
							delegated[id.Name] = true
						}
					}
				}
				continue
			}
			ifs, ok := stmt.(*ast.IfStmt)
			if !ok || len(ifs.Body.List) == 0 {
				continue
			}
			ret, ok := ifs.Body.List[len(ifs.Body.List)-1].(*ast.ReturnStmt)
			if !ok || len(ret.Results) > 0 {
				continue
			}
			cond := exprText(fset, src, ifs.Cond)
			if cancelGuards[cond] || reportsSomething(ifs) || namesDelegated(ifs.Cond, delegated) {
				continue
			}
			pos := fset.Position(ifs.Pos())
			out = append(out, "design.go:"+strconv.Itoa(pos.Line)+" if "+cond)
		}
		return true
	})
	return out
}

// reportsSomething reports whether n calls one of the reporters.
func reportsSomething(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch f := call.Fun.(type) {
		case *ast.Ident:
			found = found || reporters[f.Name]
		case *ast.SelectorExpr:
			found = found || reporters[f.Sel.Name]
		}
		return !found
	})
	return found
}

// namesDelegated reports whether cond tests a value that came from a helper
// which already reported.
func namesDelegated(cond ast.Expr, delegated map[string]bool) bool {
	found := false
	ast.Inspect(cond, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && delegated[id.Name] {
			found = true
		}
		return !found
	})
	return found
}

// TestGuardHelpersReport keeps the delegation above honest: a helper the
// guards trust to explain their silence must itself call a reporter.
func TestGuardHelpersReport(t *testing.T) {
	src, err := os.ReadFile("design.go")
	if err != nil {
		t.Fatalf("cannot read design.go: %v", err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "design.go", src, 0)
	if err != nil {
		t.Fatalf("cannot parse design.go: %v", err)
	}
	found := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		for _, name := range guardHelpers {
			if fn.Name.Name != name {
				continue
			}
			found[name] = true
			if !callsReportDeclined(fn.Body) {
				t.Errorf("%s returns nil without calling reportDeclined; every guard that trusts it then declines in silence", name)
			}
		}
	}
	for _, name := range guardHelpers {
		if !found[name] {
			t.Errorf("design.go has no func %s", name)
		}
	}
}

func callsReportDeclined(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "reportDeclined" {
			found = true
		}
		return !found
	})
	return found
}

// exprText slices the original source an expression was written as, which is
// what makes cancelGuards readable against the file.
func exprText(fset *token.FileSet, src []byte, e ast.Expr) string {
	return string(src[fset.Position(e.Pos()).Offset:fset.Position(e.End()).Offset])
}

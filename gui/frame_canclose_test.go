package gui

import (
	"go/ast"
	"go/parser"
	gotoken "go/token" // the package's own `token` is a lexer type (codeeditor.go)
	"os"
	"strings"
	"testing"
)

// TestFrameCanCloseVetoesTheWindowManagerClose: the close button is the one
// close an application cannot intercept anywhere else. Close() runs
// CloseAllViews before either of its callbacks fires, so "unsaved work — really
// quit?" answered from those is answered over a dismantled frame; CanClose is
// where the answer still means something.
func TestFrameCanCloseVetoesTheWindowManagerClose(t *testing.T) {
	frame := NewFrame()
	if !frame.CanClose() {
		t.Error("a frame with no guard refused to close")
	}

	asked := 0
	frame.SetCanCloseCallback(func(f *Frame) bool {
		asked++
		if f != frame {
			t.Errorf("guard was handed %v, want the frame it was installed on", f)
		}
		return false
	})
	if frame.CanClose() {
		t.Error("CanClose ignored a guard that said no")
	}
	if asked != 1 {
		t.Errorf("guard consulted %d times, want 1", asked)
	}

	frame.SetCanCloseCallback(func(*Frame) bool { return true })
	if !frame.CanClose() {
		t.Error("CanClose ignored a guard that said yes")
	}
}

// TestWindowCloseAsksTheFrameFirst: the guard only protects anything if the
// close paths consult it, and consult it before they start closing. Both
// backends run inside a window procedure the OS drives, so this reads the
// source — crude, but it covers the Win32 half, which no test here can run and
// which is verified on a real machine long after this does. The GLFW half is
// driven for real in window_close_guard_glfw_test.go; it is kept here so the
// two backends are still read against one rule.
func TestWindowCloseAsksTheFrameFirst(t *testing.T) {
	cases := []struct {
		file  string
		start string
		end   string
	}{
		{"window_glfw.go", "func onWindowClose(", "\nfunc onMouseButton("},
		{"window_windows.go", "case win32.WM_CLOSE:", "\n\tdefault:"},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			body := sourceBlock(t, c.file, c.start, c.end)
			guard := strings.Index(body, "CanClose()")
			teardown := strings.Index(body, "PromptSaveClose(")
			if guard < 0 {
				t.Fatal("the close path does not consult Frame.CanClose; unsaved work is quit over silently here")
			}
			if teardown < 0 {
				t.Fatal("the close path no longer calls PromptSaveClose; update this test to match")
			}
			if guard > teardown {
				t.Error("CanClose is consulted after the close has started; a veto arrives too late")
			}
		})
	}
}

// TestWin32CloseGuardHasTheRightPolarity: the test above catches a guard that
// was deleted, not one that was turned round. Reading WM_CLOSE as
// `if f.CanClose()` swallows the message for every frame that wants to stay and
// tears down every frame that agreed to go — the call is still there, still
// ahead of PromptSaveClose, and the user loses the work he just clicked Cancel
// over. The GLFW backend answers this by being driven
// (TestOnWindowCloseObeysTheFrameVeto); the Win32 window procedure needs a
// message pump and an HWND this host has neither of, so its polarity is read
// out of the syntax tree instead.
func TestWin32CloseGuardHasTheRightPolarity(t *testing.T) {
	const file = "window_windows.go"
	fset := gotoken.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("cannot parse %s: %v", file, err)
	}

	var clause *ast.CaseClause
	ast.Inspect(parsed, func(n ast.Node) bool {
		cc, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, e := range cc.List {
			if sel, ok := e.(*ast.SelectorExpr); ok && sel.Sel.Name == "WM_CLOSE" {
				clause = cc
				return false
			}
		}
		return true
	})
	if clause == nil {
		t.Fatalf("%s no longer handles WM_CLOSE; update this test to match", file)
	}

	var guard *ast.IfStmt
	for _, stmt := range clause.Body {
		if is, ok := stmt.(*ast.IfStmt); ok && callsCanClose(is.Cond) {
			guard = is
			break
		}
	}
	if guard == nil {
		t.Fatal("WM_CLOSE does not consult Frame.CanClose; unsaved work is quit over silently on Windows")
	}

	// The veto has to be the NEGATED reading, and the branch it guards has to
	// leave without going on to the teardown — swallowing WM_CLOSE is what
	// keeps the window standing.
	negated := false
	ast.Inspect(guard.Cond, func(n ast.Node) bool {
		if u, ok := n.(*ast.UnaryExpr); ok && u.Op == gotoken.NOT && callsCanClose(u.X) {
			negated = true
			return false
		}
		return true
	})
	if !negated {
		t.Error("WM_CLOSE closes the frames that refuse and keeps the ones that agree: the CanClose test is not negated")
	}
	// "Returns" is not enough, and the difference is the whole guard: six
	// neighbouring case arms end in `return win32.DefWindowProc(...)`, so
	// harmonising this one with them is a natural edit that reads as tidying.
	// It also hands the message straight to Windows, which destroys the window
	// — the exact outcome the veto exists to prevent. Swallowing WM_CLOSE means
	// returning 0 and nothing else.
	swallows, defers2Windows, tearsDown := false, false, false
	ast.Inspect(guard.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.ReturnStmt:
			if len(v.Results) == 1 {
				if lit, ok := v.Results[0].(*ast.BasicLit); ok && lit.Value == "0" {
					swallows = true
				}
			}
		case *ast.CallExpr:
			if id, ok := v.Fun.(*ast.Ident); ok && id.Name == "PromptSaveClose" {
				tearsDown = true
			}
			if sel, ok := v.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "DefWindowProc" {
				defers2Windows = true
			}
		}
		return true
	})
	if defers2Windows {
		t.Error("the vetoed branch hands WM_CLOSE to DefWindowProc; Windows destroys the window despite the veto")
	}
	if !swallows {
		t.Error("the vetoed branch does not return 0; WM_CLOSE is not swallowed and the window goes down anyway")
	}
	if tearsDown {
		t.Error("the vetoed branch still runs PromptSaveClose; the veto changes nothing")
	}
}

// callsCanClose reports whether e calls a CanClose method anywhere inside it.
func callsCanClose(e ast.Expr) (found bool) {
	ast.Inspect(e, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "CanClose" {
			found = true
			return false
		}
		return true
	})
	return
}

// sourceBlock returns the text of file between the start marker and the first
// end marker after it.
func sourceBlock(t *testing.T, file, start, end string) string {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("cannot read %s: %v", file, err)
	}
	src := string(raw)
	from := strings.Index(src, start)
	if from < 0 {
		t.Fatalf("%s has no %q", file, start)
	}
	rest := src[from:]
	to := strings.Index(rest, end)
	if to < 0 {
		t.Fatalf("%s: %q is not terminated by %q", file, start, end)
	}
	return rest[:to]
}

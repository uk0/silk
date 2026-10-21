package gui

import (
	"errors"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"strings"
	"testing"

	"github.com/uk0/silk/diagnostic"
)

func TestModalPanicDiagnosticPreservesCause(t *testing.T) {
	cause := errors.New("initial focus callback")
	err := modalPanicDiagnostic(cause)
	if diagnostic.CodeOf(err) != diagnostic.GUIModalPanic || !errors.Is(err, cause) || err.Hint == "" {
		t.Fatal(err)
	}
	if got := modalPanicDiagnostic("plain panic"); !strings.Contains(got.Error(), "plain panic") {
		t.Fatal(got)
	}
	if NewDialog("Normal", nil).LastError() != nil {
		t.Fatal("fresh dialog reports an error")
	}
}

// The native Windows behavioral test covers the failure and successful reuse.
// Pin both platform entrypoints too: no platform may reintroduce the old order
// (disable owner / invoke cbOnShow / only then register cleanup).
func TestModalRollbackPrecedesInitializationOnBothBackends(t *testing.T) {
	for _, file := range []string{"window_windows.go", "window_glfw.go"} {
		t.Run(file, func(t *testing.T) {
			f, err := parser.ParseFile(gotoken.NewFileSet(), file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Name.Name != "ShowModal" {
					continue
				}
				found = true
				var rollback gotoken.Pos
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if d, ok := n.(*ast.DeferStmt); ok && rollback == 0 {
						rollback = d.Pos()
					}
					if c, ok := n.(*ast.CallExpr); ok {
						sensitive := false
						if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "cbOnShow" {
							sensitive = true
						}
						if sel, ok := c.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetEnabled" && len(c.Args) == 1 {
							if id, ok := c.Args[0].(*ast.Ident); ok && id.Name == "false" {
								sensitive = true
							}
						}
						if sensitive && (rollback == 0 || rollback > c.Pos()) {
							t.Error("modal mutates/initializes before rollback")
						}
					}
					return true
				})
				if rollback == 0 {
					t.Fatal("missing modal rollback")
				}
			}
			if !found {
				t.Fatal("ShowModal not found")
			}
		})
	}
}

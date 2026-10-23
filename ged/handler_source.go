package ged

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

const handlerPackagePrefix = "package p;"

func parseHandlerFile(code string) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "handler.go", handlerPackagePrefix+code, parser.ParseComments|parser.SkipObjectResolution)
	return fset, f, err
}

// handlerBlocks splits independent handlers without dropping helper declarations
// or comments. Unbound free helpers stay with their preceding owning handler.
func handlerBlocks(code string, events map[string]string) []handlerStub {
	fset, file, err := parseHandlerFile(code)
	if err != nil {
		return nil
	}
	bound := map[string]bool{}
	for _, name := range events {
		bound[name] = true
	}
	var names []string
	var starts []int
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || (len(names) > 0 && fn.Recv == nil && !bound[fn.Name.Name]) {
			continue
		}
		start := fset.Position(fn.Pos()).Offset - len(handlerPackagePrefix)
		if fn.Doc != nil {
			start = fset.Position(fn.Doc.Pos()).Offset - len(handlerPackagePrefix)
		}
		if len(names) == 0 {
			start = 0
		}
		names = append(names, fn.Name.Name)
		starts = append(starts, start)
	}
	var out []handlerStub
	for i, name := range names {
		end := len(code)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		out = append(out, handlerStub{name: name, body: strings.TrimSpace(code[starts[i]:end])})
	}
	return out
}

// rebindHandlerMethods updates every receiver in a multi-event snippet, while
// retaining each receiver identifier and leaving free helper functions alone.
func rebindHandlerMethods(code, recv string) string {
	fset, file, err := parseHandlerFile(code)
	if err != nil {
		return code
	}
	for i := len(file.Decls) - 1; i >= 0; i-- {
		fn, ok := file.Decls[i].(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			continue
		}
		header := recv
		if r := receiverIdent(fn); r != "" {
			header = strings.Replace(header, "(ui *", "("+r+" *", 1)
		}
		start := fset.Position(fn.Pos()).Offset - len(handlerPackagePrefix)
		end := fset.Position(fn.Name.Pos()).Offset - len(handlerPackagePrefix)
		code = code[:start] + header + code[end:]
	}
	return code
}

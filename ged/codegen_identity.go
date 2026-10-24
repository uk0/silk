package ged

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// SplitCodeType reads the UI type from a generated split machine half. It rejects
// unknown ownership, ambiguous constructors and standalone output containing
// user handlers or main. Integrators can preserve the constructor/receiver API
// across form-title changes instead of deriving a different name on each save.
// This is a structural check, not a full Go type check or authenticity proof.
func SplitCodeType(source string) (string, error) {
	line, _, _ := strings.Cut(source, "\n")
	if strings.TrimSuffix(line, "\r") != GeneratedCodeHeader {
		return "", fmt.Errorf("refusing to replace a file not owned by the designer exporter")
	}
	file, err := parser.ParseFile(token.NewFileSet(), "", source, parser.SkipObjectResolution)
	if err != nil {
		return "", err
	}
	candidates := map[string]bool{}
	for _, declaration := range file.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typ := spec.(*ast.TypeSpec)
			structure, ok := typ.Type.(*ast.StructType)
			if !ok || typ.Assign.IsValid() {
				continue
			}
			for _, field := range structure.Fields.List {
				if len(field.Names) != 1 || field.Names[0].Name != "Form" {
					continue
				}
				star, ok := field.Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				selector, ok := star.X.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "Form" {
					continue
				}
				pkg, ok := selector.X.(*ast.Ident)
				if ok && pkg.Name == "gui" {
					candidates[typ.Name.Name] = true
				}
			}
		}
	}
	var found []string
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		// A standalone export can use the same header. Only the generated
		// constructor and BindServices belong in a split machine half; reject
		// all other functions, including free-function event handlers.
		if fn.Recv != nil && fn.Name.Name == "BindServices" {
			continue
		}
		constructor := ""
		if fn.Recv == nil && fn.Type.Results != nil && len(fn.Type.Results.List) == 1 {
			if star, ok := fn.Type.Results.List[0].Type.(*ast.StarExpr); ok {
				if name, ok := star.X.(*ast.Ident); ok && candidates[name.Name] && fn.Name.Name == "New"+name.Name {
					constructor = name.Name
				}
			}
		}
		if constructor == "" {
			return "", fmt.Errorf("not a split machine file; use an explicit split export to a new path")
		}
		found = append(found, constructor)
	}
	if len(found) != 1 {
		return "", fmt.Errorf("expected one generated UI type/constructor, found %d", len(found))
	}
	return found[0], nil
}

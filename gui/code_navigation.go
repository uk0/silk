package gui

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// isActionModifier returns true if the platform's primary action modifier is held.
// macOS: Command key (Super/LWin)
// Windows/Linux: Ctrl key
func isActionModifier() bool {
	if runtime.GOOS == "darwin" {
		return IsKeyDown(KeyLWin) || IsKeyDown(KeyRWin)
	}
	return IsKeyDown(KeyCtrl)
}

// NavigationTarget represents a found definition location.
type NavigationTarget struct {
	FilePath string
	Line     int
	Column   int
	Name     string
	Kind     string // "func", "type", "var", "const", "method", "param", "local"
}

// NavPosition records a cursor position for back/forward navigation.
type NavPosition struct {
	FilePath string
	Line     int
	Column   int
	ScrollY  float64
}

// NavigationStack provides back/forward navigation history.
type NavigationStack struct {
	stack   []NavPosition
	current int // points to the current position (-1 = empty)
}

// Push adds a new position to the stack, discarding any forward history.
func (ns *NavigationStack) Push(pos NavPosition) {
	if ns.current < len(ns.stack)-1 {
		ns.stack = ns.stack[:ns.current+1]
	}
	ns.stack = append(ns.stack, pos)
	ns.current = len(ns.stack) - 1
}

// CanGoBack returns true if there is a previous position in history.
func (ns *NavigationStack) CanGoBack() bool {
	return ns.current > 0
}

// GoBack moves to the previous position and returns it.
func (ns *NavigationStack) GoBack() (NavPosition, bool) {
	if ns.current <= 0 {
		return NavPosition{}, false
	}
	ns.current--
	return ns.stack[ns.current], true
}

// CanGoForward returns true if there is a next position in history.
func (ns *NavigationStack) CanGoForward() bool {
	return ns.current < len(ns.stack)-1
}

// GoForward moves to the next position and returns it.
func (ns *NavigationStack) GoForward() (NavPosition, bool) {
	if ns.current >= len(ns.stack)-1 {
		return NavPosition{}, false
	}
	ns.current++
	return ns.stack[ns.current], true
}

// FindDefinition searches for the definition of an identifier.
// First searches the current file content, then other .go files in the same
// directory, then walks up to find a gui/ package directory.
//
// Resolution within a single file is AST-based (go/parser): it understands
// func/type/var/const declarations, methods, and function-local declarations
// (params, named results, and short variable declarations). If the source
// fails to parse (e.g. mid-edit), it falls back to a tolerant line scanner so
// navigation keeps working on partial code.
func FindDefinition(word string, currentFile string, currentContent string) *NavigationTarget {
	if word == "" {
		return nil
	}

	// 1. Search current file content first
	target := findInContent(word, currentFile, currentContent)
	if target != nil {
		return target
	}

	// 2. Search other .go files in the same directory
	dir := filepath.Dir(currentFile)
	if dir != "" && dir != "." {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
					continue
				}
				path := filepath.Join(dir, entry.Name())
				if path == currentFile {
					continue
				}
				content, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				target := findInContent(word, path, string(content))
				if target != nil {
					return target
				}
			}
		}
	}

	// 3. Search gui/ package directory (for framework API navigation)
	guiDir := findGuiPackageDir(currentFile)
	if guiDir != "" && guiDir != dir {
		entries, err := os.ReadDir(guiDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
					continue
				}
				path := filepath.Join(guiDir, entry.Name())
				content, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				target := findInContent(word, path, string(content))
				if target != nil {
					return target
				}
			}
		}
	}

	return nil
}

// findInContent searches for a definition of word within the given file content.
// It parses the source with go/ast for accurate results and falls back to a
// line scanner when the source does not parse.
func findInContent(word, filePath, content string) *NavigationTarget {
	if target := findDefinitionAST(word, filePath, content); target != nil {
		return target
	}
	return findInContentRegex(word, filePath, content)
}

// findDefinitionAST resolves a definition using the Go AST. It returns nil if
// the source cannot be parsed or the name has no declaration in this file.
//
// Top-level declarations (func/type/var/const/method) are preferred, matching
// click-to-definition on an exported API symbol. If the name is only declared
// locally (a parameter, named result, or short variable declaration), the
// innermost such declaration is returned.
func findDefinitionAST(word, filePath, content string) *NavigationTarget {
	fset := gotoken.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, content, parser.SkipObjectResolution)
	if err != nil || file == nil {
		return nil
	}

	// Top-level declarations win: this is what a user clicking an API symbol
	// almost always wants to reach.
	if t := topLevelDecl(word, filePath, file, fset); t != nil {
		return t
	}

	// Otherwise resolve a function-local declaration, preferring the innermost
	// (deepest) enclosing scope.
	return localDecl(word, filePath, file, fset)
}

// topLevelDecl returns the package-level declaration of word, if any.
func topLevelDecl(word, filePath string, file *ast.File, fset *gotoken.FileSet) *NavigationTarget {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil || d.Name.Name != word {
				continue
			}
			kind := "func"
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = "method"
			}
			return navTarget(filePath, d.Name.Pos(), fset, word, kind)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name != nil && s.Name.Name == word {
						return navTarget(filePath, s.Name.Pos(), fset, word, "type")
					}
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == gotoken.CONST {
						kind = "const"
					}
					for _, name := range s.Names {
						if name.Name == word {
							return navTarget(filePath, name.Pos(), fset, word, kind)
						}
					}
				}
			}
		}
	}
	return nil
}

// localDecl returns the innermost function-local declaration of word: a
// parameter, named result, receiver, or short variable / var declaration.
func localDecl(word, filePath string, file *ast.File, fset *gotoken.FileSet) *NavigationTarget {
	var best *ast.Ident
	bestDepth := -1
	var bestKind string

	// consider records a candidate at the given nesting depth, keeping the
	// deepest (innermost) one seen so far.
	consider := func(id *ast.Ident, depth int, kind string) {
		if id == nil || id.Name != word {
			return
		}
		if depth >= bestDepth {
			best = id
			bestDepth = depth
			bestKind = kind
		}
	}

	// fieldNames walks a parameter/result/receiver field list.
	fieldNames := func(fl *ast.FieldList, depth int, kind string) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, n := range f.Names {
				consider(n, depth, kind)
			}
		}
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		// Function signature declarations live at depth 0 of the body.
		fieldNames(fn.Recv, 0, "param")
		if fn.Type != nil {
			fieldNames(fn.Type.Params, 0, "param")
			fieldNames(fn.Type.Results, 0, "param")
		}
		// Walk the body, tracking block-nesting depth so the innermost
		// declaration of word wins.
		depth := 0
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BlockStmt:
				depth++
			case *ast.AssignStmt:
				if x.Tok == gotoken.DEFINE {
					for _, lhs := range x.Lhs {
						if id, ok := lhs.(*ast.Ident); ok {
							consider(id, depth, "local")
						}
					}
				}
			case *ast.ValueSpec:
				for _, n := range x.Names {
					consider(n, depth, "local")
				}
			case *ast.RangeStmt:
				if x.Tok == gotoken.DEFINE {
					if id, ok := x.Key.(*ast.Ident); ok {
						consider(id, depth, "local")
					}
					if id, ok := x.Value.(*ast.Ident); ok {
						consider(id, depth, "local")
					}
				}
			}
			return true
		})
	}

	if best == nil {
		return nil
	}
	return navTarget(filePath, best.Pos(), fset, word, bestKind)
}

// ReferenceMatch is a single use site of an identifier within a file.
type ReferenceMatch struct {
	Line   int // 0-based
	Column int // 0-based
}

// FindReferences returns every position in content where an identifier named
// word appears (declarations and uses alike), using the Go AST.
//
// Resolution is name-based and scoped to this single file: cross-package
// resolution and shadowing analysis are out of scope. The source is parsed
// with go/parser; if it does not parse, an empty slice is returned.
func FindReferences(word, content string) []ReferenceMatch {
	if word == "" {
		return nil
	}
	fset := gotoken.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, parser.SkipObjectResolution)
	if err != nil || file == nil {
		return nil
	}

	var matches []ReferenceMatch
	ast.Inspect(file, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Name != word {
			return true
		}
		// Skip the package clause identifier: it is not a symbol reference.
		if id == file.Name {
			return true
		}
		pos := fset.Position(id.Pos())
		matches = append(matches, ReferenceMatch{Line: pos.Line - 1, Column: pos.Column - 1})
		return true
	})
	return matches
}

// OutlineSymbol describes a top-level declaration for an outline / symbol list.
type OutlineSymbol struct {
	Name     string
	Kind     string // "func", "type", "var", "const", "method"
	Receiver string // method receiver type, empty for plain funcs
	Line     int    // 0-based
	Column   int    // 0-based
}

// OutlineSymbols returns the top-level functions, methods, types, vars and
// consts declared in content, in source order, using the Go AST. It returns
// nil if the source does not parse.
func OutlineSymbols(content string) []OutlineSymbol {
	fset := gotoken.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, parser.SkipObjectResolution)
	if err != nil || file == nil {
		return nil
	}

	var out []OutlineSymbol
	add := func(name, kind, recv string, pos gotoken.Pos) {
		p := fset.Position(pos)
		out = append(out, OutlineSymbol{
			Name:     name,
			Kind:     kind,
			Receiver: recv,
			Line:     p.Line - 1,
			Column:   p.Column - 1,
		})
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil {
				continue
			}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				add(d.Name.Name, "method", receiverType(d.Recv), d.Name.Pos())
			} else {
				add(d.Name.Name, "func", "", d.Name.Pos())
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name != nil {
						add(s.Name.Name, "type", "", s.Name.Pos())
					}
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == gotoken.CONST {
						kind = "const"
					}
					for _, name := range s.Names {
						add(name.Name, kind, "", name.Pos())
					}
				}
			}
		}
	}
	return out
}

// receiverType returns the (possibly pointer) receiver type name as it appears
// in source, e.g. "*Server" or "Server".
func receiverType(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	switch t := recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return "*" + id.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// navTarget builds a NavigationTarget from a token position. Line and Column
// are 0-based to match the editor's coordinate system.
func navTarget(filePath string, pos gotoken.Pos, fset *gotoken.FileSet, name, kind string) *NavigationTarget {
	p := fset.Position(pos)
	return &NavigationTarget{
		FilePath: filePath,
		Line:     p.Line - 1,
		Column:   p.Column - 1,
		Name:     name,
		Kind:     kind,
	}
}

// findInContentRegex is the tolerant fallback used when source does not parse.
// It scans for a definition of word line by line.
func findInContentRegex(word, filePath, content string) *NavigationTarget {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// func FuncName( ... or func (receiver) MethodName( ...
		if strings.HasPrefix(trimmed, "func ") {
			rest := trimmed[5:]
			kind := "func"
			// Check for method: func (r *Type) Name(
			if strings.HasPrefix(rest, "(") {
				closeP := strings.Index(rest, ")")
				if closeP >= 0 {
					rest = strings.TrimSpace(rest[closeP+1:])
					kind = "method"
				}
			}
			// rest now starts with the function/method name
			nameEnd := strings.IndexAny(rest, "( \t")
			if nameEnd > 0 {
				name := rest[:nameEnd]
				if name == word {
					return &NavigationTarget{FilePath: filePath, Line: i, Name: name, Kind: kind}
				}
			}
		}

		// type TypeName struct/interface/...
		if strings.HasPrefix(trimmed, "type ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 && parts[1] == word {
				return &NavigationTarget{FilePath: filePath, Line: i, Name: word, Kind: "type"}
			}
		}

		// var VarName ... or const ConstName ...
		if strings.HasPrefix(trimmed, "var ") || strings.HasPrefix(trimmed, "const ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				// Strip trailing type annotation or '='
				name := strings.TrimRight(parts[1], "=")
				if name == word {
					return &NavigationTarget{FilePath: filePath, Line: i, Name: word, Kind: "var"}
				}
			}
		}
	}
	return nil
}

// findGuiPackageDir walks up from the current file's directory looking for
// a gui/ sibling directory (the framework's GUI package).
func findGuiPackageDir(currentFile string) string {
	dir := filepath.Dir(currentFile)
	for i := 0; i < 5; i++ {
		guiDir := filepath.Join(dir, "gui")
		if info, err := os.Stat(guiDir); err == nil && info.IsDir() {
			return guiDir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

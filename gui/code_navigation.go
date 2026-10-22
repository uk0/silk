package gui

import (
	"go/ast"
	"go/scanner"
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

// IsActionModifier reports whether the platform's primary shortcut modifier is
// held (Command on macOS, Ctrl elsewhere). Call on the UI thread during input.
func IsActionModifier() bool { return isActionModifier() }

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
// is incomplete (e.g. mid-edit), valid recovered AST nodes remain navigable;
// comments and strings never turn into fake declarations.
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

// findInContent searches the recoverable AST of the actual buffer. Lexical
// comments/strings never become fallback declarations, including while typing.
func findInContent(word, filePath, content string) *NavigationTarget {
	if target := findDefinitionAST(word, filePath, content); target != nil {
		return target
	}
	// A named func declaration cannot be nested in another function. Recover
	// its token header even when an earlier unfinished body swallowed its AST.
	// Unlike regex, scanner never sees declarations inside comments/raw strings.
	var scan scanner.Scanner
	fset := gotoken.NewFileSet()
	f := fset.AddFile(filePath, -1, len(content))
	scan.Init(f, []byte(content), nil, 0)
	for {
		_, tok, _ := scan.Scan()
		if tok == gotoken.EOF {
			return nil
		}
		if tok != gotoken.FUNC {
			continue
		}
		pos, tok, name := scan.Scan()
		kind := "func"
		if tok == gotoken.LPAREN {
			depth := 1
			for depth > 0 {
				_, tok, _ = scan.Scan()
				if tok == gotoken.EOF {
					return nil
				}
				if tok == gotoken.LPAREN {
					depth++
				}
				if tok == gotoken.RPAREN {
					depth--
				}
			}
			pos, tok, name = scan.Scan()
			kind = "method"
		}
		if tok != gotoken.IDENT || name != word {
			continue
		}
		_, next, _ := scan.Scan()
		if next == gotoken.LPAREN || next == gotoken.LBRACK {
			return navTarget(filePath, pos, fset, word, kind)
		}
	}
}

// findDefinitionAST remains name-based, not cursor/scope-aware. Package-level
// declarations take precedence; local candidates use the existing depth policy.
// An incomplete function or package-less snippet does not erase prior targets.
func findDefinitionAST(word, filePath, content string) *NavigationTarget {
	if word == "" || word == "_" {
		return nil
	}
	_, fset, file, _, prefixLines := parseRecoverableGo(content, false)
	if file == nil {
		return nil
	}
	target := topLevelDecl(word, filePath, file, fset)
	if target == nil {
		target = localDecl(word, filePath, file, fset)
	}
	if target != nil {
		target.Line -= prefixLines
	}
	return target
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
// with recoverable go/parser syntax; incomplete source retains valid identifiers.
func FindReferences(word, content string) []ReferenceMatch {
	if word == "" {
		return nil
	}
	_, fset, file, prefix, prefixLines := parseRecoverableGo(content, false)
	if file == nil {
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
		pos := fset.PositionFor(id.Pos(), false)
		offset := pos.Offset - prefix
		if offset < 0 || offset+len(word) > len(content) || content[offset:offset+len(word)] != word {
			return true
		}
		matches = append(matches, ReferenceMatch{Line: pos.Line - 1 - prefixLines, Column: pos.Column - 1})
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

// OutlineSymbols returns recoverable top-level declarations in source order.
// It shares the editor's Go declaration model and accepts incomplete source and
// package-less snippets. Columns are original UTF-8 byte offsets, not //line
// remappings, runes or UTF-16. Invalid/blank placeholder names are omitted.
func OutlineSymbols(content string) []OutlineSymbol {
	var out []OutlineSymbol
	for _, d := range goDeclarations(content) {
		out = append(out, OutlineSymbol{Name: d.symbol.Name, Kind: declarationKind(d.symbol.Kind),
			Receiver: d.receiver, Line: d.symbol.Line, Column: d.column})
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
	p := fset.PositionFor(pos, false)
	return &NavigationTarget{
		FilePath: filePath,
		Line:     p.Line - 1,
		Column:   p.Column - 1,
		Name:     name,
		Kind:     kind,
	}
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

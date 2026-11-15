package gui

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	gotoken "go/token"
	"strings"
	"unicode/utf8"
)

// goDeclaration is shared by the editor outline and the SDK symbol finder.
// AST recovery is intentional: an unfinished function must not erase earlier
// declarations. No regexp fallback may invent code inside comments or strings.
type goDeclaration struct {
	symbol   CodeSymbol
	column   int    // original UTF-8 byte column, zero based
	receiver string // SDK spelling keeps a pointer, unlike the editor group label
}

func parseRecoverableGo(content string, outline bool) (source []byte, fset *gotoken.FileSet, file *ast.File, prefix, prefixLines int) {
	return parseGoSource(content, outline, nil)
}

func parseGoSource(content string, outline bool, metadata *goOutlineMetadata) (source []byte, fset *gotoken.FileSet, file *ast.File, prefix, prefixLines int) {
	source = []byte(content)
	// The editor also supports package-less snippets. Insert only a synthetic
	// package clause, keeping an explicit offset/line correction for all results.
	var peek scanner.Scanner
	peek.Init(gotoken.NewFileSet().AddFile("", -1, len(source)), source, nil, 0)
	_, first, _ := peek.Scan()
	prefix, prefixLines = 0, 0
	if first != gotoken.PACKAGE {
		const header = "package _\n"
		source = append([]byte(header), source...)
		prefix, prefixLines = len(header), 1
	}
	if outline {
		blankGoOutlineBodies(source, metadata, prefix)
	}
	fset = gotoken.NewFileSet()
	var err error
	file, err = parser.ParseFile(fset, "", source, parser.AllErrors|parser.SkipObjectResolution)
	if metadata != nil {
		metadata.valid = file != nil && err == nil
	}
	return
}

func goDeclarations(content string) []goDeclaration {
	return parseGoDeclarations(content, nil)
}

func parseGoDeclarations(content string, metadata *goOutlineMetadata) []goDeclaration {
	if content == "" {
		return nil
	}
	source, fset, file, prefix, prefixLines := parseGoSource(content, true, metadata)
	if file == nil {
		return nil
	}
	var result []goDeclaration
	if len(file.Decls) > 0 {
		result = make([]goDeclaration, 0, len(file.Decls))
	}
	add := func(id *ast.Ident, kind int, detail, receiver string) {
		if id == nil || id.Name == "_" || !gotoken.IsIdentifier(id.Name) || !id.Pos().IsValid() {
			return
		}
		// Ignore //line remapping: these locations address the actual editor buffer,
		// not the compiler's virtual source filename/line number.
		pos := fset.PositionFor(id.Pos(), false)
		offset := pos.Offset - prefix
		if offset < 0 || offset+len(id.Name) > len(content) || content[offset:offset+len(id.Name)] != id.Name {
			return
		}
		result = append(result, goDeclaration{symbol: CodeSymbol{
			Name: id.Name, Kind: kind, Line: pos.Line - 1 - prefixLines, Detail: detail,
			Receiver: strings.TrimPrefix(receiver, "*"),
		}, column: pos.Column - 1, receiver: receiver})
	}
	slice := func(start, end gotoken.Pos) string {
		if !start.IsValid() || !end.IsValid() || end < start {
			return ""
		}
		a, b := fset.PositionFor(start, false).Offset, fset.PositionFor(end, false).Offset
		if a < 0 || b < a || b > len(source) {
			return ""
		}
		// Display details are bounded independently of source/body size. Never split
		// UTF-8 while clipping a pathological generic constraint or struct result.
		if b-a > 2048 {
			b = a + 2048
			for b > a && !utf8.RuneStart(source[b]) {
				b--
			}
		}
		return compactDeclarationDetail(string(source[a:b]))
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil || d.Type == nil {
				continue
			}
			if metadata != nil && metadata.valid {
				metadata.headers = append(metadata.headers, goOutlineSpan{
					fset.PositionFor(d.Pos(), false).Offset - prefix,
					fset.PositionFor(d.Type.End(), false).Offset - prefix,
				})
			}
			kind, recv := SymFunc, ""
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = SymMethod
				if field := d.Recv.List[0]; field != nil && field.Type != nil {
					recv = slice(field.Type.Pos(), field.Type.End())
				}
			}
			add(d.Name, kind, "func"+slice(d.Name.End(), d.Type.End()), recv)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					detail := "type"
					switch s.Type.(type) {
					case *ast.StructType:
						detail = "type struct"
					case *ast.InterfaceType:
						detail = "type interface"
					}
					add(s.Name, SymType, detail, "")
				case *ast.ValueSpec:
					if d.Tok != gotoken.VAR && d.Tok != gotoken.CONST {
						continue
					}
					kind, detail := SymVar, "var"
					if d.Tok == gotoken.CONST {
						kind, detail = SymConst, "const"
					}
					for _, name := range s.Names {
						add(name, kind, detail, "")
					}
				}
			}
		}
	}
	if metadata != nil {
		metadata.finish()
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func compactDeclarationDetail(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	count := 0
	for offset := range s {
		if count == 240 {
			return s[:offset] + "…"
		}
		count++
	}
	return s
}

func declarationKind(kind int) string {
	switch kind {
	case SymFunc:
		return "func"
	case SymMethod:
		return "method"
	case SymType:
		return "type"
	case SymConst:
		return "const"
	default:
		return "var"
	}
}

// Outline consumers need declaration signatures, never statement ASTs. A lexical
// pass blanks function bodies in-place (retaining byte lengths/newlines), so
// typing in a large function does not allocate an AST for every local expression.
// The scanner, not substring searches, distinguishes comments/strings; struct
// and interface result types and generic constraints remain intact.
func blankOutlineFunctionBodies(source []byte) {
	blankGoOutlineBodies(source, nil, 0)
}

func blankGoOutlineBodies(source []byte, metadata *goOutlineMetadata, prefix int) {
	var scan scanner.Scanner
	file := gotoken.NewFileSet().AddFile("", -1, len(source))
	mode := scanner.Mode(0)
	if metadata != nil {
		mode = scanner.ScanComments
	}
	scan.Init(file, source, nil, mode)
	braces, parens, brackets := 0, 0, 0
	header := false
	bodyStart := -1
	previous := gotoken.ILLEGAL
	type span struct{ start, end int }
	var bodies []span
	for {
		pos, tok, _ := scan.Scan()
		if tok == gotoken.EOF {
			break
		}
		offset := file.Offset(pos)
		if tok == gotoken.COMMENT {
			if metadata != nil && offset+1 < len(source) && source[offset+1] == '/' {
				end := offset
				for end < len(source) && source[end] != '\n' {
					end++
				}
				metadata.comments = append(metadata.comments, goOutlineSpan{offset + 2 - prefix, end - prefix})
			}
			continue // comments must not replace the preceding struct/interface token
		}
		if bodyStart >= 0 {
			if tok == gotoken.LBRACE {
				braces++
			}
			if tok == gotoken.RBRACE {
				braces--
				if braces == 0 {
					bodies = append(bodies, span{bodyStart, offset})
					if metadata != nil {
						metadata.bodies = append(metadata.bodies, goOutlineSpan{bodyStart - prefix, offset - prefix})
					}
					bodyStart = -1
					header = false
				}
			}
			continue
		}
		switch tok {
		case gotoken.FUNC:
			if braces == 0 && parens == 0 && brackets == 0 {
				header = true
			}
		case gotoken.LPAREN:
			parens++
		case gotoken.RPAREN:
			parens = max(0, parens-1)
		case gotoken.LBRACK:
			brackets++
		case gotoken.RBRACK:
			brackets = max(0, brackets-1)
		case gotoken.LBRACE:
			if header && braces == 0 && parens == 0 && brackets == 0 && previous != gotoken.STRUCT && previous != gotoken.INTERFACE {
				bodyStart = offset + 1
			}
			braces++
		case gotoken.RBRACE:
			braces = max(0, braces-1)
		case gotoken.SEMICOLON:
			if braces == 0 && parens == 0 && brackets == 0 {
				header = false
			}
		}
		previous = tok
	}
	if bodyStart >= 0 {
		bodies = append(bodies, span{bodyStart, len(source)})
	}
	for _, body := range bodies {
		for i := body.start; i < body.end; i++ {
			if source[i] != '\n' && source[i] != '\r' {
				source[i] = ' '
			}
		}
	}
}

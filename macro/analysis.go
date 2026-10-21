package macro

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"sort"
	"strings"

	"github.com/uk0/silk/diagnostic"
)

// Span is a half-open UTF-8 byte range in the original, unexpanded source.
type Span struct{ Start, End int }
type SymbolKind int

const (
	SymbolMacro SymbolKind = iota
	SymbolParameter
)

type Symbol struct {
	Name      string
	Kind      SymbolKind
	Selection Span
	// Range includes the definition body for macros; parameters use Selection.
	Range  Span
	Detail string
}
type Reference struct {
	Span
	Target int
}
type HighlightKind int

const (
	HighlightDirective HighlightKind = iota
	HighlightMacro
	HighlightParameter
)

type Highlight struct {
	Span
	Kind HighlightKind
}

// Analysis is an independent snapshot. It contains no UI objects, filesystem
// access or executed code. It is safe to compute off the UI thread.
type Analysis struct {
	Symbols     []Symbol
	References  []Reference
	Highlights  []Highlight
	Diagnostics []*diagnostic.Error
}

// Analyze understands the generator's lexical boundaries, including inert
// directive-looking text in raw strings/block comments. Valid symbols survive
// incomplete edits elsewhere. Diagnostics validate expansion without writing it.
func Analyze(filename string, src []byte) Analysis {
	var a Analysis
	if len(src) > MaxSourceBytes {
		a.Diagnostics = append(a.Diagnostics, diagnostic.New(diagnostic.MacroLimit, "macro source exceeds limit", nil).At(diagnostic.Location{File: filename}))
		return a
	}
	dirs, _ := scanDirectives(filename, src)
	macros := map[string]int{}
	open := -1
	params := map[string]int{}
	bodyStart := 0
	addParams := func(end int) {
		if open < 0 || end < bodyStart {
			return
		}
		for _, loc := range placeholder.FindAllIndex(src[bodyStart:end], -1) {
			lo, hi := bodyStart+loc[0], bodyStart+loc[1]
			a.Highlights = append(a.Highlights, Highlight{Span{lo, hi}, HighlightParameter})
			if target, ok := params[string(src[lo+2:hi-1])]; ok {
				a.References = append(a.References, Reference{Span{lo, hi}, target})
			}
		}
		a.Symbols[open].Range.End = end
	}
	// First collect declarations and their parameter scopes. References may precede
	// declarations, exactly as in Expand.
	for _, d := range dirs {
		a.Highlights = append(a.Highlights, Highlight{Span{d.pos.Offset, d.pos.Offset + len("//silk:") + len(d.kind)}, HighlightDirective})
		if d.kind == "end" {
			addParams(d.start)
			open = -1
			continue
		}
		if d.kind != "macro" && d.kind != "expand" {
			continue
		}
		expr, err := parser.ParseExpr(d.body)
		if err != nil {
			continue
		}
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			continue
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok {
			continue
		}
		bodyOffset := d.pos.Offset + strings.Index(string(src[d.pos.Offset:d.end]), d.body)
		selection := Span{bodyOffset + int(id.Pos()) - 1, bodyOffset + int(id.End()) - 1}
		a.Highlights = append(a.Highlights, Highlight{selection, HighlightMacro})
		if d.kind != "macro" {
			continue
		}
		if open >= 0 {
			addParams(d.start)
		}
		open = len(a.Symbols)
		a.Symbols = append(a.Symbols, Symbol{Name: id.Name, Kind: SymbolMacro, Selection: selection, Range: Span{d.start, len(src)}, Detail: d.body})
		if _, exists := macros[id.Name]; !exists {
			macros[id.Name] = open
		}
		params = map[string]int{}
		for _, arg := range call.Args {
			param, ok := arg.(*ast.Ident)
			if !ok {
				continue
			}
			r := Span{bodyOffset + int(param.Pos()) - 1, bodyOffset + int(param.End()) - 1}
			params[param.Name] = len(a.Symbols)
			a.Symbols = append(a.Symbols, Symbol{Name: param.Name, Kind: SymbolParameter, Selection: r, Range: r, Detail: "macro parameter"})
			a.Highlights = append(a.Highlights, Highlight{r, HighlightParameter})
		}
		bodyStart = d.end
	}
	addParams(len(src))
	for _, d := range dirs {
		if d.kind != "expand" {
			continue
		}
		expr, err := parser.ParseExpr(d.body)
		if err != nil {
			continue
		}
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			continue
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok {
			continue
		}
		if target, ok := macros[id.Name]; ok {
			off := d.pos.Offset + strings.Index(string(src[d.pos.Offset:d.end]), d.body)
			a.References = append(a.References, Reference{Span{off + int(id.Pos()) - 1, off + int(id.End()) - 1}, target})
		}
	}
	sort.SliceStable(a.Highlights, func(i, j int) bool { return a.Highlights[i].Start < a.Highlights[j].Start })
	if _, err := Expand(filename, src); err != nil {
		var d *diagnostic.Error
		if errors.As(err, &d) {
			a.Diagnostics = append(a.Diagnostics, d)
		} else {
			a.Diagnostics = append(a.Diagnostics, diagnostic.New(diagnostic.Internal, "macro analysis failed", err))
		}
	}
	return a
}

// DefinitionAt resolves invocation names and scoped ${Parameter} references.
// A caret immediately after a token is accepted, matching keyboard navigation.
func (a Analysis) DefinitionAt(offset int) (Symbol, bool) {
	for _, r := range a.References {
		if offset >= r.Start && offset <= r.End && r.Target >= 0 && r.Target < len(a.Symbols) {
			return a.Symbols[r.Target], true
		}
	}
	for _, s := range a.Symbols {
		if offset >= s.Selection.Start && offset <= s.Selection.End {
			return s, true
		}
	}
	return Symbol{}, false
}

func sourceLocation(filename string, src []byte, offset int) diagnostic.Location {
	offset = max(0, min(offset, len(src)))
	return diagnostic.Location{File: filename, Line: bytes.Count(src[:offset], []byte{'\n'}) + 1, Column: offset - bytes.LastIndexByte(src[:offset], '\n'), Offset: offset}
}

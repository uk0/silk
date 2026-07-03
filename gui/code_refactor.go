package gui

import (
	"errors"
	"github.com/uk0/silk/paint"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"sort"
	"strings"
	"unicode"
)

// RenameInFile renames all occurrences of oldName to newName in the given text.
// It only renames whole-word matches (not substrings), where word boundaries
// are defined by Go identifier rules.
func RenameInFile(text, oldName, newName string) string {
	if oldName == "" || oldName == newName {
		return text
	}
	oldRunes := []rune(oldName)
	newRuneStr := newName

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = renameInLine(line, oldRunes, newRuneStr)
	}
	return strings.Join(lines, "\n")
}

// renameInLine performs whole-word replacement within a single line.
func renameInLine(line string, oldRunes []rune, newName string) string {
	runes := []rune(line)
	oldLen := len(oldRunes)
	if oldLen == 0 || len(runes) < oldLen {
		return line
	}

	var result []rune
	i := 0
	for i < len(runes) {
		// Check if oldRunes matches starting at position i
		if i+oldLen <= len(runes) && runesEqual(runes[i:i+oldLen], oldRunes) {
			// Check left boundary: char before must not be an identifier char
			leftOK := i == 0 || !isIdentChar(runes[i-1])
			// Check right boundary: char after must not be an identifier char
			rightOK := i+oldLen >= len(runes) || !isIdentChar(runes[i+oldLen])

			if leftOK && rightOK {
				result = append(result, []rune(newName)...)
				i += oldLen
				continue
			}
		}
		result = append(result, runes[i])
		i++
	}
	return string(result)
}

// isIdentChar returns true if the rune is a valid Go identifier character.
func isIdentChar(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// runesEqual compares two rune slices for equality.
func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- Rename mode for CodeEditor ---

// activateRename enters rename mode, pre-filling the word at the cursor.
func (this *CodeEditor) activateRename() {
	word := this.wordAtCursor()
	if word == "" {
		return
	}
	this.renameActive = true
	this.renameWord = word
	this.renameText = word
	this.renameCursorPos = len([]rune(word))
	this.Self().Update()
}

// renameAccept performs the whole-word rename and pushes to undo stack.
func (this *CodeEditor) renameAccept() {
	if !this.renameActive {
		return
	}
	oldWord := this.renameWord
	newWord := this.renameText
	this.renameActive = false

	if oldWord == "" || newWord == "" || oldWord == newWord {
		this.renameText = ""
		this.renameWord = ""
		this.renameCursorPos = 0
		this.Self().Update()
		return
	}

	// Save full text for undo as kind 3 (full text replace)
	oldFullText := strings.Join(this.lines, "\n")
	newFullText := RenameInFile(oldFullText, oldWord, newWord)

	if oldFullText == newFullText {
		this.renameText = ""
		this.renameWord = ""
		this.renameCursorPos = 0
		this.Self().Update()
		return
	}

	this.pushUndo(editAction{
		kind:    3,
		line:    this.cursorLine,
		col:     this.cursorCol,
		text:    newFullText,
		oldText: oldFullText,
	})

	this.lines = strings.Split(newFullText, "\n")
	if len(this.lines) == 0 {
		this.lines = []string{""}
	}
	this.rebuildText()
	this.clampCursor()

	this.renameText = ""
	this.renameWord = ""
	this.renameCursorPos = 0
	this.ensureCursorVisible()
	this.Self().Update()
}

// renameCancel exits rename mode without making changes.
func (this *CodeEditor) renameCancel() {
	this.renameActive = false
	this.renameText = ""
	this.renameWord = ""
	this.renameCursorPos = 0
	this.Self().Update()
}

// drawRenameInput draws the inline rename text box at the cursor position.
func (this *CodeEditor) drawRenameInput(g paint.Painter, topOff, editorRight float64) {
	if !this.renameActive {
		return
	}

	fe := this.font.FontExtents()
	lh := fe.Height + 2

	// Calculate cursor screen position
	this.clampCursor()
	line := this.lines[this.cursorLine]
	lineRunes := []rune(line)
	prefixEnd := this.cursorCol
	if prefixEnd > len(lineRunes) {
		prefixEnd = len(lineRunes)
	}
	cx := this.gutterW + 10 - this.scrollX + this.measureText(string(lineRunes[:prefixEnd]))
	cy := float64(this.cursorLine)*lh - this.scrollY + topOff

	// Box dimensions
	inputW := 200.0
	inputH := lh + 8

	// Position the box near the cursor
	bx := cx - 4
	by := cy - 4
	if bx+inputW > editorRight {
		bx = editorRight - inputW - 4
	}
	if bx < this.gutterW+4 {
		bx = this.gutterW + 4
	}

	// Shadow
	g.SetBrush1(paint.Color{R: 0, G: 0, B: 0, A: 60})
	g.Rectangle(bx+2, by+2, inputW, inputH)
	g.Fill()

	// Background
	g.SetBrush1(paint.Color{R: 45, G: 45, B: 55, A: 250})
	g.Rectangle(bx, by, inputW, inputH)
	g.Fill()

	// Border (highlight color)
	g.SetPen1(paint.Color{R: 80, G: 140, B: 220, A: 255}, 1.5)
	g.Rectangle(bx, by, inputW, inputH)
	g.Stroke()

	// Label
	g.SetFont(this.font)
	g.SetBrush1(paint.Color{R: 100, G: 140, B: 200, A: 200})
	g.DrawText1(bx+4, by-2, "Rename:")

	// Input text
	g.SetBrush1(paint.Color{R: 220, G: 220, B: 230, A: 255})
	g.DrawText1(bx+4, by+fe.Ascent+4, this.renameText)

	// Text cursor
	curPrefix := string([]rune(this.renameText)[:this.renameCursorPos])
	tcx := bx + 4 + this.measureText(curPrefix)
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 230, A: 255})
	g.Rectangle(tcx, by+3, 1, inputH-6)
	g.Fill()
}

// --- AST-based single-file symbol rename ---
//
// RenameSymbol pairs with the AST-based go-to-definition / find-references in
// code_navigation.go: there we LOCATE identifiers; here we MUTATE them without
// disturbing comments or string literals (which a naive string replace would).

// goReservedWords is the set of Go reserved words rejected as identifier
// names. (codeeditor.go has its own syntax-highlighting keyword set;
// keeping a private list here avoids cross-coupling.)
var goReservedWords = map[string]struct{}{
	"break": {}, "case": {}, "chan": {}, "const": {}, "continue": {},
	"default": {}, "defer": {}, "else": {}, "fallthrough": {}, "for": {},
	"func": {}, "go": {}, "goto": {}, "if": {}, "import": {},
	"interface": {}, "map": {}, "package": {}, "range": {}, "return": {},
	"select": {}, "struct": {}, "switch": {}, "type": {}, "var": {},
}

// isGoIdent reports whether name is a syntactically valid Go identifier:
// a non-empty string that starts with a letter or '_' and consists of letters,
// digits, and '_'. Reserved words are rejected.
func isGoIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	if _, isKw := goReservedWords[name]; isKw {
		return false
	}
	return true
}

// RenameSymbol returns src with every occurrence of identifier oldName
// (declarations and uses alike) replaced by newName, using go/parser + go/ast
// so that comments, string literals, and import paths are left untouched.
//
// Validation:
//   - newName must be a valid Go identifier (see isGoIdent); otherwise an error
//     is returned and src is unchanged.
//   - oldName == newName is a no-op and returns src, nil.
//   - If src does not parse, the original src is returned along with the parse
//     error so the caller can surface it (no panic).
//   - If newName already exists as a top-level declaration in src and differs
//     from oldName, the rename is rejected to avoid silently shadowing or
//     duplicating package-level symbols.
//
// Known limitation (name-based, not scope-aware): this rewrites EVERY
// identifier whose Name == oldName in the file. It does not distinguish
// between two unrelated local variables in different functions that happen to
// share a name, nor between a field selector and a same-named variable. A
// fully scope-aware rename (using go/types) is intentionally out of scope here
// and would be a separate, larger change; tests below pin the current
// behaviour so any future upgrade surfaces as a deliberate breaking change.
func RenameSymbol(src, oldName, newName string) (string, error) {
	if oldName == newName {
		return src, nil
	}
	if !isGoIdent(oldName) {
		return src, errors.New("rename: oldName is not a valid Go identifier")
	}
	if !isGoIdent(newName) {
		return src, errors.New("rename: newName is not a valid Go identifier")
	}

	fset := gotoken.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil || file == nil {
		if err == nil {
			err = errors.New("rename: source did not parse")
		}
		return src, err
	}

	// Reject if newName is already taken by a different top-level declaration
	// in this file: blindly renaming would create a duplicate package-level
	// symbol and break the build.
	if topLevelNameTaken(file, newName) {
		return src, errors.New("rename: newName already declared at package level")
	}

	// Collect every *ast.Ident whose Name == oldName. We deliberately skip:
	//   - the package clause identifier (renaming the package is a different
	//     operation entirely);
	//   - identifiers that live inside an import path's quoted string (these
	//     are *ast.BasicLit, not *ast.Ident, so they're skipped naturally —
	//     but ImportSpec also has an optional Name *ast.Ident for the alias,
	//     which we DO rename since that's a real identifier).
	type span struct{ start, end int }
	var spans []span
	ast.Inspect(file, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id == nil || id.Name != oldName {
			return true
		}
		if id == file.Name { // package clause
			return true
		}
		startPos := fset.Position(id.Pos())
		endPos := fset.Position(id.End())
		// Position.Offset is 0-indexed bytes into the source, exactly what we
		// need for splicing the string.
		if startPos.Offset < 0 || endPos.Offset > len(src) || endPos.Offset < startPos.Offset {
			return true
		}
		spans = append(spans, span{start: startPos.Offset, end: endPos.Offset})
		return true
	})

	if len(spans) == 0 {
		return src, nil
	}

	// Rewrite from the END of the source backwards so earlier offsets stay
	// valid after each splice.
	sort.Slice(spans, func(i, j int) bool { return spans[i].start > spans[j].start })

	out := src
	for _, s := range spans {
		// Defensive: confirm the slice we're about to replace really is the
		// old name. A mismatch here would mean the AST and our byte offsets
		// disagree, which should never happen for an unedited source string,
		// but skipping is safer than corrupting the file.
		if s.end-s.start != len(oldName) || out[s.start:s.end] != oldName {
			continue
		}
		out = out[:s.start] + newName + out[s.end:]
	}
	return out, nil
}

// RenameSymbolCount is the companion of RenameSymbol that also returns how
// many identifiers were rewritten (useful for "renamed N occurrences" status
// messages in an editor). Validation and limitations are identical.
func RenameSymbolCount(src, oldName, newName string) (int, string, error) {
	if oldName == newName {
		return 0, src, nil
	}
	if !isGoIdent(oldName) {
		return 0, src, errors.New("rename: oldName is not a valid Go identifier")
	}
	if !isGoIdent(newName) {
		return 0, src, errors.New("rename: newName is not a valid Go identifier")
	}

	fset := gotoken.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil || file == nil {
		if err == nil {
			err = errors.New("rename: source did not parse")
		}
		return 0, src, err
	}
	if topLevelNameTaken(file, newName) {
		return 0, src, errors.New("rename: newName already declared at package level")
	}

	type span struct{ start, end int }
	var spans []span
	ast.Inspect(file, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id == nil || id.Name != oldName {
			return true
		}
		if id == file.Name {
			return true
		}
		startPos := fset.Position(id.Pos())
		endPos := fset.Position(id.End())
		if startPos.Offset < 0 || endPos.Offset > len(src) || endPos.Offset < startPos.Offset {
			return true
		}
		spans = append(spans, span{start: startPos.Offset, end: endPos.Offset})
		return true
	})

	if len(spans) == 0 {
		return 0, src, nil
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start > spans[j].start })

	out := src
	count := 0
	for _, s := range spans {
		if s.end-s.start != len(oldName) || out[s.start:s.end] != oldName {
			continue
		}
		out = out[:s.start] + newName + out[s.end:]
		count++
	}
	return count, out, nil
}

// topLevelNameTaken reports whether name is already declared at package level
// in file (as a func, method, type, var, or const). Method names are checked
// too because Go forbids a top-level func and a method-on-same-type sharing a
// name in the same package, and we want a conservative bail.
func topLevelNameTaken(file *ast.File, name string) bool {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name != nil && d.Name.Name == name {
				return true
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name != nil && s.Name.Name == name {
						return true
					}
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if n != nil && n.Name == name {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

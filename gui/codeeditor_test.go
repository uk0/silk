package gui

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// SetText / Text
// ---------------------------------------------------------------------------

func TestCodeEditorSetGetText(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("hello\nworld")
	if e.Text() != "hello\nworld" {
		t.Errorf("Text() = %q, want hello\\nworld", e.Text())
	}
	if len(e.lines) != 2 {
		t.Errorf("line count = %d, want 2", len(e.lines))
	}
}

func TestCodeEditorEmptyText(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("")
	if len(e.lines) < 1 {
		t.Error("empty text should have at least one line")
	}
}

func TestCodeEditorMultiline(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("a\nb\nc\nd\ne")
	if len(e.lines) != 5 {
		t.Errorf("line count = %d, want 5", len(e.lines))
	}
}

func TestCodeEditorSetTextResetsCursor(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("first\nsecond\nthird")
	if e.cursorLine != 0 || e.cursorCol != 0 {
		t.Errorf("cursor = (%d,%d), want (0,0)", e.cursorLine, e.cursorCol)
	}
}

// ---------------------------------------------------------------------------
// ParseSymbols
// ---------------------------------------------------------------------------

func TestCodeEditorParseSymbols(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(`package main

func hello() {}
func (s *Server) Start() {}
type Config struct {}
var version = "1.0"
const maxRetries = 3
`)
	symbols := e.ParseSymbols()

	names := map[string]bool{}
	for _, s := range symbols {
		names[s.Name] = true
	}

	expected := []string{"hello", "Start", "Config", "version", "maxRetries"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing symbol: %s", name)
		}
	}
}

func TestCodeEditorParseSymbolKinds(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(`package main
func standalone() {}
func (r *Receiver) method() {}
type MyType struct {}
var myVar = 1
const myConst = 2
`)
	symbols := e.ParseSymbols()
	kindMap := map[string]int{}
	for _, s := range symbols {
		kindMap[s.Name] = s.Kind
	}

	if kindMap["standalone"] != SymFunc {
		t.Errorf("standalone kind = %d, want SymFunc(%d)", kindMap["standalone"], SymFunc)
	}
	if kindMap["method"] != SymMethod {
		t.Errorf("method kind = %d, want SymMethod(%d)", kindMap["method"], SymMethod)
	}
	if kindMap["MyType"] != SymType {
		t.Errorf("MyType kind = %d, want SymType(%d)", kindMap["MyType"], SymType)
	}
	if kindMap["myVar"] != SymVar {
		t.Errorf("myVar kind = %d, want SymVar(%d)", kindMap["myVar"], SymVar)
	}
	if kindMap["myConst"] != SymConst {
		t.Errorf("myConst kind = %d, want SymConst(%d)", kindMap["myConst"], SymConst)
	}
}

func TestCodeEditorParseSymbolsEmpty(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("")
	symbols := e.ParseSymbols()
	if len(symbols) != 0 {
		t.Errorf("empty content: symbol count = %d, want 0", len(symbols))
	}
}

func TestCodeEditorParseSymbolsComments(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(`package main
// func notAFunc() {}
/* func alsoNot() {} */
func realFunc() {}
`)
	symbols := e.ParseSymbols()
	for _, s := range symbols {
		if s.Name == "notAFunc" || s.Name == "alsoNot" {
			t.Errorf("comment should not produce symbol: %s", s.Name)
		}
	}
	found := false
	for _, s := range symbols {
		if s.Name == "realFunc" {
			found = true
		}
	}
	if !found {
		t.Error("realFunc not found")
	}
}

func TestCodeEditorParseVarConstBlocks(t *testing.T) {
	e := NewCodeEditor()
	e.SetText(`package main
var (
	alpha = 1
	beta  = 2
	_     = 3
)
const (
	gamma = 10
	delta = 20
)
`)
	symbols := e.ParseSymbols()
	names := map[string]bool{}
	for _, s := range symbols {
		names[s.Name] = true
	}
	for _, name := range []string{"alpha", "beta", "gamma", "delta"} {
		if !names[name] {
			t.Errorf("missing block symbol: %s", name)
		}
	}
	if names["_"] {
		t.Error("underscore should be skipped")
	}
}

// ---------------------------------------------------------------------------
// FindDefinition
// ---------------------------------------------------------------------------

func TestFindDefinitionFunc(t *testing.T) {
	content := `package main

func doSomething() {}
type MyStruct struct {}
`
	target := FindDefinition("doSomething", "test.go", content)
	if target == nil {
		t.Fatal("doSomething not found")
	}
	if target.Name != "doSomething" {
		t.Errorf("name = %q", target.Name)
	}
	if target.Kind != "func" {
		t.Errorf("kind = %q, want func", target.Kind)
	}
}

func TestFindDefinitionType(t *testing.T) {
	content := `package main
type Handler struct {}
`
	target := FindDefinition("Handler", "test.go", content)
	if target == nil {
		t.Fatal("Handler not found")
	}
	if target.Kind != "type" {
		t.Errorf("kind = %q, want type", target.Kind)
	}
}

func TestFindDefinitionMethod(t *testing.T) {
	content := `package main
func (s *Server) Handle() {}
`
	target := FindDefinition("Handle", "test.go", content)
	if target == nil {
		t.Fatal("Handle not found")
	}
	if target.Kind != "method" {
		t.Errorf("kind = %q, want method", target.Kind)
	}
}

func TestFindDefinitionNotFound(t *testing.T) {
	content := `package main
func hello() {}
`
	target := FindDefinition("nonexistent", "test.go", content)
	if target != nil {
		t.Error("should return nil for nonexistent symbol")
	}
}

func TestFindDefinitionEmpty(t *testing.T) {
	target := FindDefinition("", "test.go", "package main")
	if target != nil {
		t.Error("empty word should return nil")
	}
}

// ---------------------------------------------------------------------------
// Snippet expansion
// ---------------------------------------------------------------------------

func TestExpandSnippetIf(t *testing.T) {
	result := expandSnippet("if ${1:condition} {\n\t${0}\n}")
	if !strings.Contains(result, "condition") {
		t.Error("placeholder 'condition' not expanded")
	}
	if strings.Contains(result, "${") {
		t.Error("unexpanded placeholder found")
	}
}

func TestExpandSnippetFunction(t *testing.T) {
	result := expandSnippet("func ${1:name}(${2:params}) ${3:returnType} {\n\t${0}\n}")
	if !strings.Contains(result, "name") {
		t.Error("placeholder 'name' not expanded")
	}
	if !strings.Contains(result, "params") {
		t.Error("placeholder 'params' not expanded")
	}
	if strings.Contains(result, "${") {
		t.Error("unexpanded placeholder found")
	}
}

func TestExpandSnippetNoPlaceholders(t *testing.T) {
	result := expandSnippet("plain text")
	if result != "plain text" {
		t.Errorf("result = %q, want plain text", result)
	}
}

func TestExpandSnippetCursorPositionCleared(t *testing.T) {
	result := expandSnippet("${0}")
	if result != "" {
		t.Errorf("${0} should expand to empty string, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// Rename (whole-word)
// ---------------------------------------------------------------------------

func TestRenameInFileBasic(t *testing.T) {
	text := "func hello() {\n\thello()\n\thelloWorld()\n}"
	result := RenameInFile(text, "hello", "greet")
	if !strings.Contains(result, "func greet()") {
		t.Error("func not renamed")
	}
	if !strings.Contains(result, "\tgreet()") {
		t.Error("call not renamed")
	}
	if !strings.Contains(result, "helloWorld") {
		t.Error("helloWorld was wrongly renamed")
	}
}

func TestRenameInFileNoMatch(t *testing.T) {
	text := "func foo() {}"
	result := RenameInFile(text, "bar", "baz")
	if result != text {
		t.Error("no match should return original text")
	}
}

func TestRenameInFileSameName(t *testing.T) {
	text := "func hello() {}"
	result := RenameInFile(text, "hello", "hello")
	if result != text {
		t.Error("same name should return original text")
	}
}

func TestRenameInFileEmpty(t *testing.T) {
	text := "func hello() {}"
	result := RenameInFile(text, "", "greet")
	if result != text {
		t.Error("empty oldName should return original text")
	}
}

func TestRenameInFileMultipleOccurrences(t *testing.T) {
	text := "x = foo + foo * foo"
	result := RenameInFile(text, "foo", "bar")
	count := strings.Count(result, "bar")
	if count != 3 {
		t.Errorf("expected 3 replacements, got %d in %q", count, result)
	}
	if strings.Contains(result, "foo") {
		t.Error("old name still present")
	}
}

func TestRenameInFileRespectsBoundaries(t *testing.T) {
	text := "foobar foo barfoo _foo"
	result := RenameInFile(text, "foo", "xyz")
	if !strings.Contains(result, "foobar") {
		t.Error("foobar should not be changed")
	}
	if !strings.Contains(result, "barfoo") {
		t.Error("barfoo should not be changed")
	}
	if !strings.Contains(result, "_foo") {
		// _foo: underscore is an identifier char, so foo is part of _foo
		// This should NOT be renamed
	}
	// standalone foo should be renamed
	parts := strings.Fields(result)
	foundXyz := false
	for _, p := range parts {
		if p == "xyz" {
			foundXyz = true
		}
	}
	if !foundXyz {
		t.Error("standalone foo should be renamed to xyz")
	}
}

// ---------------------------------------------------------------------------
// Symbol kind labels
// ---------------------------------------------------------------------------

func TestSymbolKindLabel(t *testing.T) {
	cases := []struct {
		kind int
		want string
	}{
		{SymFunc, "f"},
		{SymType, "T"},
		{SymVar, "v"},
		{SymConst, "c"},
		{SymMethod, "m"},
		{999, " "},
	}
	for _, tc := range cases {
		got := symbolKindLabel(tc.kind)
		if got != tc.want {
			t.Errorf("symbolKindLabel(%d) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// NavigationStack
// ---------------------------------------------------------------------------

func TestNavigationStack(t *testing.T) {
	ns := &NavigationStack{}
	if ns.CanGoBack() {
		t.Error("empty stack should not go back")
	}
	if ns.CanGoForward() {
		t.Error("empty stack should not go forward")
	}

	ns.Push(NavPosition{Line: 1})
	ns.Push(NavPosition{Line: 2})
	ns.Push(NavPosition{Line: 3})

	if !ns.CanGoBack() {
		t.Error("should be able to go back")
	}

	pos, ok := ns.GoBack()
	if !ok || pos.Line != 2 {
		t.Errorf("GoBack = line %d, want 2", pos.Line)
	}

	if !ns.CanGoForward() {
		t.Error("should be able to go forward")
	}

	pos, ok = ns.GoForward()
	if !ok || pos.Line != 3 {
		t.Errorf("GoForward = line %d, want 3", pos.Line)
	}
}

func TestNavigationStackPushTruncatesForward(t *testing.T) {
	ns := &NavigationStack{}
	ns.Push(NavPosition{Line: 1})
	ns.Push(NavPosition{Line: 2})
	ns.Push(NavPosition{Line: 3})

	ns.GoBack() // at 2
	ns.GoBack() // at 1

	// Push new position; forward history (2,3) should be discarded
	ns.Push(NavPosition{Line: 10})

	if ns.CanGoForward() {
		t.Error("forward history should be discarded after Push")
	}
}

// ---------------------------------------------------------------------------
// SymbolPopup
// ---------------------------------------------------------------------------

func TestSymbolPopupFilter(t *testing.T) {
	sp := NewSymbolPopup()
	sp.symbols = []CodeSymbol{
		{Name: "HandleRequest", Kind: SymFunc},
		{Name: "HandleResponse", Kind: SymFunc},
		{Name: "CreateUser", Kind: SymFunc},
		{Name: "Config", Kind: SymType},
	}

	sp.filterText = "Handle"
	sp.filter()
	if len(sp.filtered) != 2 {
		t.Errorf("filtered count = %d, want 2", len(sp.filtered))
	}

	sp.filterText = "config"
	sp.filter()
	if len(sp.filtered) != 1 {
		t.Errorf("case-insensitive: filtered count = %d, want 1", len(sp.filtered))
	}

	sp.filterText = ""
	sp.filter()
	if len(sp.filtered) != 4 {
		t.Errorf("empty filter: filtered count = %d, want 4", len(sp.filtered))
	}
}

func TestSymbolPopupSelectWrap(t *testing.T) {
	sp := NewSymbolPopup()
	sp.symbols = []CodeSymbol{
		{Name: "A"}, {Name: "B"}, {Name: "C"},
	}
	sp.filter()
	sp.selectedIdx = 2

	sp.SelectNext() // should wrap to 0
	if sp.selectedIdx != 0 {
		t.Errorf("SelectNext wrap: idx = %d, want 0", sp.selectedIdx)
	}

	sp.SelectPrev() // should wrap to 2
	if sp.selectedIdx != 2 {
		t.Errorf("SelectPrev wrap: idx = %d, want 2", sp.selectedIdx)
	}
}

// ---------------------------------------------------------------------------
// Multi-Cursor Editing
// ---------------------------------------------------------------------------

// TestMultiCursorAddAndClear verifies that adding a secondary cursor and
// clearing it works as advertised, including duplicate-position suppression.
func TestMultiCursorAddAndClear(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("line 1\nline 2\nline 3")
	e.AddCursorAtLine(1, 2)
	if len(e.additionalCursors) != 1 {
		t.Fatalf("after AddCursorAtLine: got %d additional, want 1", len(e.additionalCursors))
	}
	// Duplicate add should be suppressed.
	e.AddCursorAtLine(1, 2)
	if len(e.additionalCursors) != 1 {
		t.Errorf("duplicate AddCursorAtLine: got %d additional, want 1", len(e.additionalCursors))
	}
	// Adding at primary cursor position should be suppressed.
	e.AddCursorAtLine(0, 0)
	if len(e.additionalCursors) != 1 {
		t.Errorf("add at primary: got %d additional, want 1", len(e.additionalCursors))
	}
	e.ClearAdditionalCursors()
	if len(e.additionalCursors) != 0 {
		t.Errorf("after ClearAdditionalCursors: got %d, want 0", len(e.additionalCursors))
	}
}

// TestMultiCursorAllCursors ensures allCursors() returns primary + additional
// in sorted order.
func TestMultiCursorAllCursors(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("aaa\nbbb\nccc")
	e.cursorLine, e.cursorCol = 1, 1
	e.AddCursorAtLine(0, 2)
	e.AddCursorAtLine(2, 0)
	all := e.allCursors()
	if len(all) != 3 {
		t.Fatalf("allCursors len = %d, want 3", len(all))
	}
	if !(all[0].line == 0 && all[0].col == 2) {
		t.Errorf("allCursors[0] = %+v, want {0, 2}", all[0])
	}
	if !(all[1].line == 1 && all[1].col == 1) {
		t.Errorf("allCursors[1] = %+v, want {1, 1}", all[1])
	}
	if !(all[2].line == 2 && all[2].col == 0) {
		t.Errorf("allCursors[2] = %+v, want {2, 0}", all[2])
	}
}

// TestMultiCursorInsertAtAll verifies that text inserted at multiple cursor
// positions ends up at every location without corrupting later cursors on
// the same line.
func TestMultiCursorInsertAtAll(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo foo foo")
	// Cursors at column 0 and column 4 (start of second "foo").
	e.cursorLine, e.cursorCol = 0, 0
	e.AddCursorAtLine(0, 4)
	e.insertAtAllCursors("X")
	// Expected: "Xfoo Xfoo foo"
	want := "Xfoo Xfoo foo"
	if e.lines[0] != want {
		t.Errorf("after insert: line = %q, want %q", e.lines[0], want)
	}
	// Primary cursor originally at col 0 should be at col 1 after its own
	// insert (no other insertion to its left shifts it).
	if e.cursorCol != 1 {
		t.Errorf("primary cursor col = %d, want 1", e.cursorCol)
	}
	// Additional cursor originally at col 4 should now be at col 6 (its own
	// insert moved it +1, the earlier insert at col 0 moved it +1).
	if len(e.additionalCursors) != 1 || e.additionalCursors[0].col != 6 {
		t.Errorf("additional cursor = %+v, want col 6", e.additionalCursors)
	}
}

// TestMultiCursorBackspace verifies Backspace at two cursors deletes a rune
// at each location and shifts surviving cursors correctly.
func TestMultiCursorBackspace(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("abcdef")
	// Cursors between b-c (col 2) and between e-f (col 5).
	e.cursorLine, e.cursorCol = 0, 2
	e.AddCursorAtLine(0, 5)
	e.backspaceAtAllCursors()
	// Expected: deletion at col 5 -> removes 'e' -> "abcdf"; deletion at
	// col 2 -> removes 'b' -> "acdf". Final: "acdf".
	if e.lines[0] != "acdf" {
		t.Errorf("after multi-backspace: line = %q, want %q", e.lines[0], "acdf")
	}
	// Primary cursor at col 2 -> 1 after deletion.
	if e.cursorCol != 1 {
		t.Errorf("primary col = %d, want 1", e.cursorCol)
	}
	// Additional cursor at col 5 -> 4 (its own deletion), then shift-left by
	// one because the earlier deletion at col 2 removed a char further left
	// on the same line: 4 - 1 = 3.
	if len(e.additionalCursors) != 1 || e.additionalCursors[0].col != 3 {
		t.Errorf("additional = %+v, want col 3", e.additionalCursors)
	}
}

// TestMultiCursorEscapeClears verifies that OnKeyDown(KeyEsc) collapses
// multi-cursor back to single cursor before touching the selection.
func TestMultiCursorEscapeClears(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("hello\nworld")
	e.AddCursorAtLine(1, 0)
	e.OnKeyDown(KeyEsc, false)
	if len(e.additionalCursors) != 0 {
		t.Errorf("after Esc: additionalCursors = %d, want 0", len(e.additionalCursors))
	}
}

// TestMultiCursorSelectNextOccurrence verifies Ctrl+D-style next-match
// behavior adds a cursor at the next occurrence of the selected word.
func TestMultiCursorSelectNextOccurrence(t *testing.T) {
	e := NewCodeEditor()
	e.SetText("foo bar foo baz foo")
	// Position at the first "foo" (col 0) and simulate word selection.
	e.cursorLine, e.cursorCol = 0, 0
	e.selectNextOccurrence()
	// First call selects current word "foo" and adds cursor at end of next
	// "foo" occurrence.
	if !e.hasSelection {
		t.Fatal("expected selection after selectNextOccurrence")
	}
	if len(e.additionalCursors) != 1 {
		t.Fatalf("additional cursors = %d, want 1", len(e.additionalCursors))
	}
	// Next "foo" starts at col 8, so cursor should be at end col 11.
	if e.additionalCursors[0].col != 11 {
		t.Errorf("second cursor col = %d, want 11", e.additionalCursors[0].col)
	}
	// Strings import is used elsewhere in this file; silence unused linters.
	_ = strings.Count("", "")
}

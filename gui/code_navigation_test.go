package gui

import "testing"

// ---------------------------------------------------------------------------
// FindDefinition (AST-based, single file)
//
// Line/Column in NavigationTarget are 0-based to match the editor.
// ---------------------------------------------------------------------------

func TestNavigationDefinitionFuncCall(t *testing.T) {
	src := `package main

func helper() int { return 1 }

func main() {
	_ = helper()
}
`
	target := FindDefinition("helper", "test.go", src)
	if target == nil {
		t.Fatal("helper not resolved")
	}
	if target.Kind != "func" {
		t.Errorf("kind = %q, want func", target.Kind)
	}
	// `func helper` is on line index 2 (0-based).
	if target.Line != 2 {
		t.Errorf("line = %d, want 2", target.Line)
	}
}

func TestNavigationDefinitionType(t *testing.T) {
	src := `package main

type Server struct{ addr string }

func run(s *Server) {}
`
	target := FindDefinition("Server", "test.go", src)
	if target == nil {
		t.Fatal("Server not resolved")
	}
	if target.Kind != "type" {
		t.Errorf("kind = %q, want type", target.Kind)
	}
	if target.Line != 2 {
		t.Errorf("line = %d, want 2", target.Line)
	}
}

func TestNavigationDefinitionMethod(t *testing.T) {
	src := `package main

type Server struct{}

func (s *Server) Handle() {}
`
	target := FindDefinition("Handle", "test.go", src)
	if target == nil {
		t.Fatal("Handle not resolved")
	}
	if target.Kind != "method" {
		t.Errorf("kind = %q, want method", target.Kind)
	}
}

func TestNavigationDefinitionConst(t *testing.T) {
	src := `package main

const MaxSize = 1024
`
	target := FindDefinition("MaxSize", "test.go", src)
	if target == nil {
		t.Fatal("MaxSize not resolved")
	}
	if target.Kind != "const" {
		t.Errorf("kind = %q, want const", target.Kind)
	}
}

func TestNavigationDefinitionTopLevelVar(t *testing.T) {
	src := `package main

var registry = map[string]int{}
`
	target := FindDefinition("registry", "test.go", src)
	if target == nil {
		t.Fatal("registry not resolved")
	}
	if target.Kind != "var" {
		t.Errorf("kind = %q, want var", target.Kind)
	}
}

func TestNavigationDefinitionLocalVar(t *testing.T) {
	src := `package main

func main() {
	count := 0
	count++
	_ = count
}
`
	target := FindDefinition("count", "main.go", src)
	if target == nil {
		t.Fatal("local count not resolved")
	}
	if target.Kind != "local" {
		t.Errorf("kind = %q, want local", target.Kind)
	}
	// `count := 0` is line index 3 (0-based).
	if target.Line != 3 {
		t.Errorf("line = %d, want 3", target.Line)
	}
}

func TestNavigationDefinitionParam(t *testing.T) {
	src := `package main

func greet(name string) string {
	return name
}
`
	target := FindDefinition("name", "p.go", src)
	if target == nil {
		t.Fatal("param name not resolved")
	}
	if target.Kind != "param" {
		t.Errorf("kind = %q, want param", target.Kind)
	}
	if target.Line != 2 {
		t.Errorf("line = %d, want 2", target.Line)
	}
}

// A shadowing local in a nested block must win over an outer/param of the same
// name (innermost scope preferred).
func TestNavigationDefinitionInnermostScope(t *testing.T) {
	src := `package main

func f(x int) {
	{
		x := 99
		_ = x
	}
}
`
	target := FindDefinition("x", "f.go", src)
	if target == nil {
		t.Fatal("x not resolved")
	}
	// The inner `x := 99` is on line index 4; the param x is on line 2.
	if target.Line != 4 {
		t.Errorf("line = %d, want 4 (inner shadow)", target.Line)
	}
	if target.Kind != "local" {
		t.Errorf("kind = %q, want local", target.Kind)
	}
}

// A top-level declaration is preferred over a function-local of the same name,
// which is what clicking an API symbol expects.
func TestNavigationDefinitionTopLevelBeatsLocal(t *testing.T) {
	src := `package main

func config() {}

func main() {
	config := 1
	_ = config
}
`
	target := FindDefinition("config", "c.go", src)
	if target == nil {
		t.Fatal("config not resolved")
	}
	if target.Kind != "func" {
		t.Errorf("kind = %q, want func (top-level beats local)", target.Kind)
	}
	if target.Line != 2 {
		t.Errorf("line = %d, want 2", target.Line)
	}
}

func TestNavigationDefinitionNotFound(t *testing.T) {
	src := `package main

func hello() {}
`
	if target := FindDefinition("nope", "test.go", src); target != nil {
		t.Errorf("expected nil for unknown symbol, got %+v", target)
	}
}

func TestNavigationDefinitionEmptyWord(t *testing.T) {
	if target := FindDefinition("", "test.go", "package main"); target != nil {
		t.Error("empty word should return nil")
	}
}

// Unparseable source must not panic; resolution falls back to the line scanner.
func TestNavigationDefinitionUnparseableNoPanic(t *testing.T) {
	// Missing closing brace + stray tokens: invalid Go.
	src := `package main

func broken( {
	this is not valid go @@@
`
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on unparseable source: %v", r)
		}
	}()
	// The regex fallback can still find the func name on a best-effort basis;
	// the contract under test is "no panic, returns gracefully".
	_ = FindDefinition("broken", "b.go", src)
	// A name that appears nowhere must be not-found even on broken source.
	if target := FindDefinition("definitely_absent", "b.go", src); target != nil {
		t.Errorf("expected nil for absent symbol in broken source, got %+v", target)
	}
}

// ---------------------------------------------------------------------------
// FindReferences
// ---------------------------------------------------------------------------

func TestNavigationReferencesCountAndPositions(t *testing.T) {
	src := `package main

func helper() int { return 1 }

func main() {
	a := helper()
	b := helper()
	_ = a
	_ = b
}
`
	refs := FindReferences("helper", src)
	// One declaration + two call sites = 3 occurrences.
	if len(refs) != 3 {
		t.Fatalf("got %d references, want 3: %+v", len(refs), refs)
	}
	// First occurrence is the declaration on line index 2.
	if refs[0].Line != 2 {
		t.Errorf("first ref line = %d, want 2", refs[0].Line)
	}
	// Call sites are on lines 5 and 6.
	if refs[1].Line != 5 || refs[2].Line != 6 {
		t.Errorf("call ref lines = %d,%d, want 5,6", refs[1].Line, refs[2].Line)
	}
	// Columns are 0-based; the declaration `helper` follows "func " (5 chars).
	if refs[0].Column != 5 {
		t.Errorf("first ref column = %d, want 5", refs[0].Column)
	}
}

func TestNavigationReferencesNone(t *testing.T) {
	src := `package main

func main() {}
`
	if refs := FindReferences("absent", src); len(refs) != 0 {
		t.Errorf("expected no references, got %+v", refs)
	}
}

func TestNavigationReferencesEmptyWord(t *testing.T) {
	if refs := FindReferences("", "package main"); refs != nil {
		t.Errorf("empty word should return nil, got %+v", refs)
	}
}

func TestNavigationReferencesSkipsPackageName(t *testing.T) {
	// The package clause identifier must not be counted as a reference.
	src := `package main

func main() {}
`
	if refs := FindReferences("main", src); len(refs) != 1 {
		t.Fatalf("got %d refs for 'main', want 1 (func only, not package): %+v", len(refs), refs)
	}
}

func TestNavigationReferencesUnparseable(t *testing.T) {
	src := `package main
func ( broken @@@`
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on unparseable source: %v", r)
		}
	}()
	if refs := FindReferences("broken", src); refs != nil {
		t.Errorf("unparseable source should yield nil, got %+v", refs)
	}
}

// ---------------------------------------------------------------------------
// OutlineSymbols
// ---------------------------------------------------------------------------

func TestNavigationOutlineSymbols(t *testing.T) {
	src := `package main

type Server struct{}

const Version = "1.0"

var globalCount int

func (s *Server) Start() {}

func helper() {}
`
	syms := OutlineSymbols(src)
	if len(syms) != 5 {
		t.Fatalf("got %d symbols, want 5: %+v", len(syms), syms)
	}

	byName := map[string]OutlineSymbol{}
	for _, s := range syms {
		byName[s.Name] = s
	}

	if byName["Server"].Kind != "type" {
		t.Errorf("Server kind = %q, want type", byName["Server"].Kind)
	}
	if byName["Version"].Kind != "const" {
		t.Errorf("Version kind = %q, want const", byName["Version"].Kind)
	}
	if byName["globalCount"].Kind != "var" {
		t.Errorf("globalCount kind = %q, want var", byName["globalCount"].Kind)
	}
	if byName["helper"].Kind != "func" {
		t.Errorf("helper kind = %q, want func", byName["helper"].Kind)
	}
	start := byName["Start"]
	if start.Kind != "method" {
		t.Errorf("Start kind = %q, want method", start.Kind)
	}
	if start.Receiver != "*Server" {
		t.Errorf("Start receiver = %q, want *Server", start.Receiver)
	}

	// Symbols are returned in source order: Server, Version, globalCount,
	// Start, helper.
	wantOrder := []string{"Server", "Version", "globalCount", "Start", "helper"}
	for i, name := range wantOrder {
		if syms[i].Name != name {
			t.Errorf("symbol[%d] = %q, want %q", i, syms[i].Name, name)
		}
	}
}

func TestNavigationOutlineUnparseable(t *testing.T) {
	if syms := OutlineSymbols("package main\nfunc ( @@@"); syms != nil {
		t.Errorf("unparseable source should yield nil, got %+v", syms)
	}
}

package snippet

import (
	"reflect"
	"strings"
	"testing"
)

// assertText fails unless the expansion text matches want.
func assertText(t *testing.T, got Expansion, want string) {
	t.Helper()
	if got.Text != want {
		t.Fatalf("Text = %q, want %q", got.Text, want)
	}
}

// assertStops fails unless the stop slice deep-equals want.
func assertStops(t *testing.T, got Expansion, want []Stop) {
	t.Helper()
	if !reflect.DeepEqual(got.Stops, want) {
		t.Fatalf("Stops = %+v, want %+v", got.Stops, want)
	}
}

// checkOffsets verifies every stop's [Start:End) slice of Text equals its
// Default, catching any drift between recorded offsets and the real text.
func checkOffsets(t *testing.T, e Expansion) {
	t.Helper()
	for i, s := range e.Stops {
		if s.Start < 0 || s.End < s.Start || s.End > len(e.Text) {
			t.Fatalf("stop %d has out-of-range offsets %d..%d (len %d)", i, s.Start, s.End, len(e.Text))
		}
		if got := e.Text[s.Start:s.End]; got != s.Default {
			t.Fatalf("stop %d: Text[%d:%d] = %q, want Default %q", i, s.Start, s.End, got, s.Default)
		}
	}
}

func TestExpandPlainText(t *testing.T) {
	for _, in := range []string{"", "hello world", "no placeholders here", "line1\nline2\ttab"} {
		e, err := Expand(in)
		if err != nil {
			t.Fatalf("Expand(%q) error: %v", in, err)
		}
		assertText(t, e, in)
		if len(e.Stops) != 0 {
			t.Fatalf("Expand(%q) Stops = %+v, want none", in, e.Stops)
		}
		checkOffsets(t, e)
	}
}

func TestExpandDefaultAndFinalCaret(t *testing.T) {
	e, err := Expand("${1:foo} bar $0")
	if err != nil {
		t.Fatal(err)
	}
	assertText(t, e, "foo bar ")
	assertStops(t, e, []Stop{
		{Index: 1, Start: 0, End: 3, Default: "foo"},
		{Index: 0, Start: 8, End: 8, Default: ""},
	})
	checkOffsets(t, e)
}

func TestExpandBracedNoColon(t *testing.T) {
	// ${1} and ${1:} both mean index 1 with an empty default.
	for _, in := range []string{"${1}", "${1:}"} {
		e, err := Expand(in)
		if err != nil {
			t.Fatalf("Expand(%q): %v", in, err)
		}
		assertText(t, e, "")
		assertStops(t, e, []Stop{{Index: 1, Start: 0, End: 0, Default: ""}})
	}
}

func TestExpandBareOrdering(t *testing.T) {
	// $1$2 keeps ascending order; a$2b$1c must reorder by index, not by
	// appearance, while keeping the appearance-based offsets.
	e, err := Expand("a$2b$1c")
	if err != nil {
		t.Fatal(err)
	}
	assertText(t, e, "abc")
	assertStops(t, e, []Stop{
		{Index: 1, Start: 2, End: 2, Default: ""}, // $1 appeared after "ab"
		{Index: 2, Start: 1, End: 1, Default: ""}, // $2 appeared after "a"
	})
	checkOffsets(t, e)
}

func TestExpandZeroSortedLast(t *testing.T) {
	// $0 in the middle must still sort last, others ascending.
	e, err := Expand("${2:b}$0${1:a}")
	if err != nil {
		t.Fatal(err)
	}
	assertText(t, e, "ba")
	assertStops(t, e, []Stop{
		{Index: 1, Start: 1, End: 2, Default: "a"},
		{Index: 2, Start: 0, End: 1, Default: "b"},
		{Index: 0, Start: 1, End: 1, Default: ""},
	})
	checkOffsets(t, e)
}

func TestExpandMirror(t *testing.T) {
	// A default definition plus a bare mirror yield two stops sharing the
	// index; the mirror inserts nothing and keeps its empty default. Stable
	// order keeps the definition before the mirror.
	e, err := Expand("${1:x}=$1")
	if err != nil {
		t.Fatal(err)
	}
	assertText(t, e, "x=")
	assertStops(t, e, []Stop{
		{Index: 1, Start: 0, End: 1, Default: "x"},
		{Index: 1, Start: 2, End: 2, Default: ""},
	})
	checkOffsets(t, e)
}

func TestExpandEscapedDollar(t *testing.T) {
	// \$ is a literal dollar and disables placeholder parsing that follows.
	cases := map[string]string{
		`\$1 costs \$5`: "$1 costs $5",
		`\${1:x}`:       "${1:x}",
		`price: \$`:     "price: $",
	}
	for in, want := range cases {
		e, err := Expand(in)
		if err != nil {
			t.Fatalf("Expand(%q): %v", in, err)
		}
		assertText(t, e, want)
		if len(e.Stops) != 0 {
			t.Fatalf("Expand(%q) Stops = %+v, want none", in, e.Stops)
		}
	}
}

func TestExpandEscapedBackslashAndBrace(t *testing.T) {
	// \\ collapses to one backslash; the following $1 is still a placeholder.
	e, err := Expand(`\\$1`)
	if err != nil {
		t.Fatal(err)
	}
	assertText(t, e, `\`)
	assertStops(t, e, []Stop{{Index: 1, Start: 1, End: 1, Default: ""}})

	// A trailing lone backslash stays literal.
	e2, err := Expand(`abc\`)
	if err != nil {
		t.Fatal(err)
	}
	assertText(t, e2, `abc\`)
	if len(e2.Stops) != 0 {
		t.Fatalf("Stops = %+v, want none", e2.Stops)
	}

	// \} inside a default is a literal brace and does not close the group.
	e3, err := Expand(`${1:a\}b}`)
	if err != nil {
		t.Fatal(err)
	}
	assertText(t, e3, "a}b")
	assertStops(t, e3, []Stop{{Index: 1, Start: 0, End: 3, Default: "a}b"}})
	checkOffsets(t, e3)
}

func TestExpandLoneDollarAndCurrency(t *testing.T) {
	// A '$' not followed by a digit or '{' is literal; but $5 (digit) IS a
	// tab stop, which is the documented gotcha for currency in a body.
	e, err := Expand("$ and $$ end$")
	if err != nil {
		t.Fatal(err)
	}
	assertText(t, e, "$ and $$ end$")
	if len(e.Stops) != 0 {
		t.Fatalf("Stops = %+v, want none", e.Stops)
	}

	e2, err := Expand("cost $5")
	if err != nil {
		t.Fatal(err)
	}
	assertText(t, e2, "cost ")
	assertStops(t, e2, []Stop{{Index: 5, Start: 5, End: 5, Default: ""}})
}

func TestExpandUnicodeByteOffsets(t *testing.T) {
	// Offsets are byte offsets: a multibyte default's End - Start equals its
	// byte length, and Text[Start:End] slices back to the exact default.
	e, err := Expand("x${1:世界}y$0")
	if err != nil {
		t.Fatal(err)
	}
	assertText(t, e, "x世界y")
	assertStops(t, e, []Stop{
		{Index: 1, Start: 1, End: 7, Default: "世界"}, // 世界 = 6 bytes, so 1..7
		{Index: 0, Start: 8, End: 8, Default: ""},     // x(1)+世界(6)+y(1) = 8
	})
	checkOffsets(t, e)

	// A bare stop after a multibyte prefix lands on the byte offset.
	e2, err := Expand("café$0")
	if err != nil {
		t.Fatal(err)
	}
	assertText(t, e2, "café")
	if len(e2.Stops) != 1 || e2.Stops[0].Start != len("café") {
		t.Fatalf("Stops = %+v, want single stop at byte %d", e2.Stops, len("café"))
	}
	checkOffsets(t, e2)
}

func TestExpandMalformed(t *testing.T) {
	bad := []string{
		"${",         // no index, no close
		"${1",        // index, no close
		"${1:",       // colon, no close
		"${1:foo",    // default, no close
		"${}",        // empty index
		"${:x}",      // no index before colon
		"${a}",       // non-numeric index
		"${1x}",      // stray char after index
		"prefix ${9", // unterminated later in the string
	}
	for _, in := range bad {
		e, err := Expand(in)
		if err == nil {
			t.Fatalf("Expand(%q) = %+v, want error", in, e)
		}
		if !reflect.DeepEqual(e, Expansion{}) {
			t.Fatalf("Expand(%q) returned non-zero Expansion with error: %+v", in, e)
		}
		if !strings.Contains(err.Error(), "snippet:") {
			t.Fatalf("Expand(%q) error %q lacks package prefix", in, err)
		}
	}
}

func TestExpandIndexOutOfRange(t *testing.T) {
	// An index too large for int must error, not panic or silently clamp,
	// for both braced and bare forms.
	huge := "99999999999999999999999999999"
	for _, in := range []string{"${" + huge + ":x}", "$" + huge} {
		if _, err := Expand(in); err == nil {
			t.Fatalf("Expand(%q) = nil error, want out-of-range error", in)
		}
	}
}

func TestExpandLargeInput(t *testing.T) {
	// A large body must expand without error and keep offsets consistent.
	prefix := strings.Repeat("x", 200000)
	e, err := Expand(prefix + "${1:tail}$0")
	if err != nil {
		t.Fatal(err)
	}
	if e.Text != prefix+"tail" {
		t.Fatalf("large Text mismatch (len %d)", len(e.Text))
	}
	assertStops(t, e, []Stop{
		{Index: 1, Start: len(prefix), End: len(prefix) + 4, Default: "tail"},
		{Index: 0, Start: len(prefix) + 4, End: len(prefix) + 4, Default: ""},
	})
	checkOffsets(t, e)
}

func TestExpandNestedBraceInDefaultIsLiteral(t *testing.T) {
	// Nesting is not supported: the first unescaped '}' closes the group and
	// the rest is literal text. This documents the boundary behavior.
	e, err := Expand("${1:a{b}c}")
	if err != nil {
		t.Fatal(err)
	}
	assertText(t, e, "a{bc}")
	assertStops(t, e, []Stop{{Index: 1, Start: 0, End: 3, Default: "a{b"}})
	checkOffsets(t, e)
}

func TestNewBook(t *testing.T) {
	ts := []Template{
		{Name: "For loop", Trigger: "for", Body: "for ${1:i} := 0; $1 < ${2:n}; $1++ {\n\t$0\n}"},
		{Name: "If", Trigger: "if", Body: "if ${1:cond} {\n\t$0\n}"},
		{Name: "If shadow", Trigger: "if", Body: "if err != nil {\n\t$0\n}"}, // duplicate trigger; last wins
	}
	book := NewBook(ts)

	if len(book) != 2 {
		t.Fatalf("book size = %d, want 2 (duplicate trigger collapses)", len(book))
	}
	got, ok := book["for"]
	if !ok || got.Name != "For loop" {
		t.Fatalf("lookup for = %+v, ok=%v", got, ok)
	}
	if book["if"].Body != "if err != nil {\n\t$0\n}" {
		t.Fatalf("duplicate trigger: last did not win, got %q", book["if"].Body)
	}
	if _, ok := book["while"]; ok {
		t.Fatal("missing trigger reported present")
	}

	// A looked-up body must round-trip through Expand.
	e, err := Expand(book["for"].Body)
	if err != nil {
		t.Fatalf("Expand(book body): %v", err)
	}
	if len(e.Stops) == 0 {
		t.Fatal("expected stops from the for-loop body")
	}
	checkOffsets(t, e)
}

func TestNewBookEmpty(t *testing.T) {
	book := NewBook(nil)
	if book == nil || len(book) != 0 {
		t.Fatalf("NewBook(nil) = %+v, want empty non-nil map", book)
	}
}

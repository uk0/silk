package gui

import (
	"reflect"
	"testing"
)

// --- Completer.Filter pure-data tests --------------------------------

// TestCompleterStartsWithDefault: default mode + case-insensitive.
// Suggestions ordered alphabetically.
func TestCompleterStartsWithDefault(t *testing.T) {
	c := NewCompleter("Apple", "apricot", "banana", "BERRY", "Cherry")
	got := c.Filter("a")
	want := []string{"Apple", "apricot"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Filter(a) = %v, want %v", got, want)
	}
}

// TestCompleterCaseSensitive: with case-sensitive on, "a" doesn't
// match "Apple" because the prefix differs in case.
func TestCompleterCaseSensitive(t *testing.T) {
	c := NewCompleter("Apple", "apricot", "Banana")
	c.SetCaseSensitive(true)
	got := c.Filter("a")
	want := []string{"apricot"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CaseSensitive Filter(a) = %v, want %v", got, want)
	}
}

// TestCompleterContainsMode finds prefix in any position. Front
// bucket (matches at position 0) sorts before back bucket.
func TestCompleterContainsMode(t *testing.T) {
	c := NewCompleter("Antelope", "Banana", "Cherry", "Manor")
	c.SetMode(MatchContains)
	got := c.Filter("an")
	// "Antelope" matches at index 0 (front, lowercased prefix "an");
	// "Banana" matches at index 1 (back, "b[an]ana");
	// "Manor" matches at index 1 (back, "m[an]or");
	// "Cherry" doesn't contain "an" — excluded.
	// Front bucket sorts alphabetically: ["Antelope"].
	// Back bucket sorts alphabetically: ["Banana", "Manor"].
	want := []string{"Antelope", "Banana", "Manor"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Contains Filter(an) = %v, want %v", got, want)
	}
}

// TestCompleterAnywhereMode returns every candidate regardless of
// the typed prefix.
func TestCompleterAnywhereMode(t *testing.T) {
	c := NewCompleter("alpha", "beta", "gamma")
	c.SetMode(MatchAnywhere)
	got := c.Filter("xyz")
	want := []string{"alpha", "beta", "gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Anywhere Filter(xyz) = %v, want %v", got, want)
	}
}

// TestCompleterEmptyPrefixReturnsAll: typing nothing gives the full
// candidate list (sorted) — common UX for "show all options up front".
func TestCompleterEmptyPrefixReturnsAll(t *testing.T) {
	c := NewCompleter("zebra", "apple", "mango")
	got := c.Filter("")
	want := []string{"apple", "mango", "zebra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("empty prefix = %v, want %v (sorted)", got, want)
	}
}

// TestCompleterDedupe removes duplicate candidates.
func TestCompleterDedupe(t *testing.T) {
	c := NewCompleter("apple", "apple", "Apple", "apricot")
	got := c.Filter("a")
	// case-insensitive comparison + alphabetical sort puts "Apple"
	// and "apple" adjacent; dedupe keeps one of each unique form
	// (case-different strings remain distinct because dedupe runs on
	// raw values, not folded ones).
	if len(got) != 3 {
		t.Errorf("dedupe: got %d entries, want 3 (Apple, apple, apricot)", len(got))
	}
}

// TestCompleterDedupeOff keeps duplicates when explicitly disabled.
func TestCompleterDedupeOff(t *testing.T) {
	c := NewCompleter("apple", "apple", "apricot")
	c.SetDedupe(false)
	got := c.Filter("a")
	if len(got) != 3 {
		t.Errorf("dedupe off: got %d, want 3 (apple, apple, apricot)", len(got))
	}
}

// TestCompleterMaxSuggestions caps the list length.
func TestCompleterMaxSuggestions(t *testing.T) {
	c := NewCompleter("a1", "a2", "a3", "a4", "a5")
	c.SetMaxSuggestions(3)
	got := c.Filter("a")
	if len(got) != 3 {
		t.Errorf("max suggestions: got %d, want 3", len(got))
	}
}

// TestCompleterDynamicSourceWins: when both Source and Candidates
// are set, the Source's CandidateAt wins.
type stubSource struct{ items []string }

func (s stubSource) CandidateAt(prefix string) []string { return s.items }

func TestCompleterDynamicSourceWins(t *testing.T) {
	c := NewCompleter("static-a", "static-b")
	c.Source = stubSource{items: []string{"dyn-x", "dyn-y"}}
	got := c.Filter("dyn")
	want := []string{"dyn-x", "dyn-y"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dynamic source: got %v, want %v", got, want)
	}
}

// TestCompleterModeStringHelpsDebug exercises the String() helper.
func TestCompleterModeStringHelpsDebug(t *testing.T) {
	cases := []struct {
		m    MatchMode
		want string
	}{
		{MatchStartsWith, "StartsWith"},
		{MatchContains, "Contains"},
		{MatchAnywhere, "Anywhere"},
		{MatchMode(99), "<unknown>"},
	}
	for _, c := range cases {
		if got := c.m.String(); got != c.want {
			t.Errorf("MatchMode(%d).String() = %q, want %q", c.m, got, c.want)
		}
	}
}

// --- Edit + Completer integration tests ------------------------------

// TestEditSetCompleterRefreshesOnInput: typing a character triggers
// Filter and the resulting suggestions reflect the new prefix.
func TestEditSetCompleterRefreshesOnInput(t *testing.T) {
	e := NewEdit()
	c := NewCompleter("apple", "banana", "apricot")
	e.SetCompleter(c)

	e.OnTextInput("a")
	got := c.Suggestions()
	want := []string{"apple", "apricot"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after typing 'a': suggestions = %v, want %v", got, want)
	}
}

// TestEditAcceptCompletionReplacesPrefix: AcceptCompletion(idx)
// replaces the active prefix with the chosen candidate.
func TestEditAcceptCompletionReplacesPrefix(t *testing.T) {
	e := NewEdit()
	c := NewCompleter("apple", "apricot")
	e.SetCompleter(c)

	e.OnTextInput("a")
	e.OnTextInput("p")
	// Suggestions: ["apple", "apricot"]
	if !e.AcceptCompletion(0) {
		t.Fatalf("AcceptCompletion(0) returned false")
	}
	if got := e.Text(); got != "apple" {
		t.Errorf("after Accept(0): text = %q, want apple", got)
	}
}

// TestEditAcceptCompletionOutOfRange: bad indices are rejected
// gracefully.
func TestEditAcceptCompletionOutOfRange(t *testing.T) {
	e := NewEdit()
	c := NewCompleter("apple")
	e.SetCompleter(c)
	e.OnTextInput("a")
	if e.AcceptCompletion(99) {
		t.Errorf("AcceptCompletion(99) on 1 suggestion should return false")
	}
	if e.AcceptCompletion(-1) {
		t.Errorf("AcceptCompletion(-1) should return false")
	}
}

// TestEditAcceptCompletionWithoutCompleter is a no-op.
func TestEditAcceptCompletionWithoutCompleter(t *testing.T) {
	e := NewEdit()
	if e.AcceptCompletion(0) {
		t.Errorf("AcceptCompletion without completer should return false")
	}
}

// TestEditCompletionPrefix tracks just the active substring, not
// the entire text. Calling SetCompletionPrefixStart re-bases.
func TestEditCompletionPrefix(t *testing.T) {
	e := NewEdit()
	e.OnTextInput("hello world a")
	// Set start before "a" — the active prefix is just "a".
	e.SetCompletionPrefixStart(len("hello world "))
	if got := e.CompletionPrefix(); got != "a" {
		t.Errorf("CompletionPrefix = %q, want a", got)
	}
}

// TestEditAcceptKeepsTextAfterCaret: text past the caret is
// preserved when accepting a completion.
func TestEditAcceptKeepsTextAfterCaret(t *testing.T) {
	e := NewEdit()
	c := NewCompleter("apple")
	e.SetCompleter(c)
	// Insert "ap[caret]xy" — the caret is in the middle.
	e.OnTextInput("a")
	e.OnTextInput("p")
	// Move caret past insertion (the test scaffold doesn't have a
	// public caret-move helper; fake it via direct fields).
	full := e.Text()
	e.SetText(full + "xy")
	// Reset caret to position after "ap" (length 2).
	e.sel0, e.sel1 = 2, 2
	e.SetCompletionPrefixStart(0)
	c.Filter(e.CompletionPrefix()) // re-Filter explicitly
	if !e.AcceptCompletion(0) {
		t.Fatalf("AcceptCompletion failed")
	}
	if got := e.Text(); got != "applexy" {
		t.Errorf("after Accept: text = %q, want applexy", got)
	}
}

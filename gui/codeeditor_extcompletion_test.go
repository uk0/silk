package gui

import "testing"

// findItem returns the first CompletionItem whose Text matches, or nil.
func findItem(items []CompletionItem, text string) *CompletionItem {
	for i := range items {
		if items[i].Text == text {
			return &items[i]
		}
	}
	return nil
}

func TestExternalCompletionStoreAndClear(t *testing.T) {
	ed := NewCodeEditor()
	if len(ed.externalCompletions) != 0 {
		t.Fatalf("new editor should have no external completions, got %d", len(ed.externalCompletions))
	}

	items := []ExternalCompletion{
		{Label: "fmt.Println", Detail: "func(...any)"},
		{Label: "fmt.Sprintf", Detail: "func(string, ...any) string"},
	}
	ed.SetExternalCompletions(items)
	if len(ed.externalCompletions) != 2 {
		t.Fatalf("SetExternalCompletions: want 2 stored, got %d", len(ed.externalCompletions))
	}

	ed.ClearExternalCompletions()
	if len(ed.externalCompletions) != 0 {
		t.Fatalf("ClearExternalCompletions: want 0 stored, got %d", len(ed.externalCompletions))
	}
}

func TestExternalToItemInsertDefaultsToLabel(t *testing.T) {
	// Empty Insert -> Label is used as the inserted text.
	got := externalToItem(ExternalCompletion{Label: "Println", Detail: "func"})
	if got.Text != "Println" {
		t.Fatalf("empty Insert should default Text to Label; got Text=%q", got.Text)
	}
	if got.Detail != "func" {
		t.Fatalf("Detail not carried through; got %q", got.Detail)
	}
	if got.Kind != CikFunction {
		t.Fatalf("external item should be CikFunction; got %d", got.Kind)
	}

	// Explicit Insert overrides Label for the inserted text (Label stays the
	// display string and is not used for Text).
	got = externalToItem(ExternalCompletion{Label: "Println(…)", Insert: "Println()"})
	if got.Text != "Println()" {
		t.Fatalf("explicit Insert should set Text; got Text=%q", got.Text)
	}
}

func TestMergeCompletionsExternalAppears(t *testing.T) {
	builtin := []CompletionItem{
		{Text: "for", Kind: CikKeyword, Detail: "keyword"},
		{Text: "int", Kind: CikType, Detail: "type"},
	}
	external := []ExternalCompletion{
		{Label: "fmt.Println", Detail: "func"},
	}
	got := mergeCompletions(builtin, external)
	if len(got) != 3 {
		t.Fatalf("merge: want 3 items (2 builtin + 1 external), got %d", len(got))
	}
	if findItem(got, "fmt.Println") == nil {
		t.Fatalf("external item fmt.Println missing from merged set")
	}
	if findItem(got, "for") == nil || findItem(got, "int") == nil {
		t.Fatalf("builtin items must survive the merge")
	}
}

func TestMergeCompletionsExternalWinsOnCollision(t *testing.T) {
	builtin := []CompletionItem{
		// Buffer-scraped guess: looks like a plain identifier.
		{Text: "Println", Kind: CikVariable, Detail: "identifier"},
		{Text: "for", Kind: CikKeyword, Detail: "keyword"},
	}
	external := []ExternalCompletion{
		// LSP knows the real signature.
		{Label: "Println", Detail: "func(...any) (int, error)"},
	}
	got := mergeCompletions(builtin, external)

	// No duplicate label.
	count := 0
	for _, it := range got {
		if it.Text == "Println" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("label collision should dedup to 1 Println, got %d", count)
	}
	if len(got) != 2 {
		t.Fatalf("merge with one collision: want 2 items, got %d", len(got))
	}

	// External wins: Detail/Kind come from the external item.
	p := findItem(got, "Println")
	if p.Detail != "func(...any) (int, error)" {
		t.Fatalf("external should win on collision; Detail=%q", p.Detail)
	}
	if p.Kind != CikFunction {
		t.Fatalf("external should win on collision; Kind=%d", p.Kind)
	}

	// Replacement happens in place — original ordering preserved.
	if got[0].Text != "Println" || got[1].Text != "for" {
		t.Fatalf("external-wins replacement should preserve order; got %q,%q", got[0].Text, got[1].Text)
	}
}

func TestMergeCompletionsEmptyExternalLeavesBuiltinUnchanged(t *testing.T) {
	builtin := []CompletionItem{
		{Text: "for", Kind: CikKeyword, Detail: "keyword"},
		{Text: "int", Kind: CikType, Detail: "type"},
	}
	got := mergeCompletions(builtin, nil)
	if len(got) != len(builtin) {
		t.Fatalf("empty external: want %d items, got %d", len(builtin), len(got))
	}
	for i := range builtin {
		if got[i] != builtin[i] {
			t.Fatalf("empty external must leave builtin unchanged at %d: got %+v want %+v", i, got[i], builtin[i])
		}
	}

	// The returned slice must be a copy, not the input backing array.
	got[0] = CompletionItem{Text: "MUTATED"}
	if builtin[0].Text != "for" {
		t.Fatalf("mergeCompletions must not mutate the builtin slice")
	}
}

func TestMergeCompletionsEmptyBuiltinYieldsExternalOnly(t *testing.T) {
	external := []ExternalCompletion{
		{Label: "a", Detail: "d1"},
		{Label: "b", Insert: "b()", Detail: "d2"},
	}
	got := mergeCompletions(nil, external)
	if len(got) != 2 {
		t.Fatalf("empty builtin: want 2 external items, got %d", len(got))
	}
	if findItem(got, "a") == nil {
		t.Fatalf("external 'a' missing")
	}
	// 'b' was injected with Insert "b()" so its Text is the inserted form.
	if findItem(got, "b()") == nil {
		t.Fatalf("external 'b' should use Insert as Text; merged=%+v", got)
	}
}

// TestMergeExternalIntoPopup exercises the editor-side merge against a live
// popup model (no GL: drives items/filter only, never the visual draw).
func TestMergeExternalIntoPopup(t *testing.T) {
	ed := NewCodeEditor()
	ed.SetText("package main\n")
	ed.SetExternalCompletions([]ExternalCompletion{
		{Label: "fmt.Println", Detail: "func(...any) (int, error)"},
	})

	ed.completion = NewCompletionPopup(ed)
	// Use a real prefix so the injected item ranks into the (capped) filtered
	// window — the realistic path: the host injects after the user has typed.
	ed.completion.Show("Pr", ed) // builds + filters built-ins for "Pr"
	builtinCount := len(ed.completion.items)
	ed.mergeExternalIntoPopup()

	if len(ed.completion.items) != builtinCount+1 {
		t.Fatalf("merge into popup: want %d items, got %d", builtinCount+1, len(ed.completion.items))
	}
	if findItem(ed.completion.items, "fmt.Println") == nil {
		t.Fatalf("external item not merged into popup candidate set")
	}
	if findItem(ed.completion.filtered, "fmt.Println") == nil {
		t.Fatalf("external item not present in filtered set after re-rank for prefix")
	}

	// With no external items the popup is left untouched.
	ed.ClearExternalCompletions()
	ed.completion.Show("Pr", ed)
	before := len(ed.completion.items)
	ed.mergeExternalIntoPopup()
	if len(ed.completion.items) != before {
		t.Fatalf("mergeExternalIntoPopup must be a no-op with no external items")
	}
}

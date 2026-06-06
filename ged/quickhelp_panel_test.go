package ged

import (
	"os/exec"
	"silk/core"
	"strings"
	"testing"
)

// TestQuickHelpSetDocUpdatesDoc verifies the SetDoc / Doc round-trip and
// the side-effects on the panel's split-line cache: an empty SetDoc
// clears the cache, a non-empty SetDoc populates it without dropping the
// last line.
func TestQuickHelpSetDocUpdatesDoc(t *testing.T) {
	p := NewQuickHelpPanel()

	p.SetDoc("first line\nsecond line\nthird line")
	if got := p.Doc(); got != "first line\nsecond line\nthird line" {
		t.Errorf("Doc() = %q", got)
	}
	if n := len(p.lines); n != 3 {
		t.Errorf("len(lines) = %d, want 3", n)
	}

	// SetDoc with an empty string clears the body and the cache.
	p.SetDoc("")
	if got := p.Doc(); got != "" {
		t.Errorf("Doc() after clear = %q", got)
	}
	if len(p.lines) != 0 {
		t.Errorf("len(lines) after clear = %d, want 0", len(p.lines))
	}
}

// TestQuickHelpSetDocClearsSymbol confirms SetDoc explicitly disowns any
// previously-looked-up symbol — the body no longer corresponds to a
// known symbol, so Symbol() must reset.
func TestQuickHelpSetDocClearsSymbol(t *testing.T) {
	p := NewQuickHelpPanel()
	// Simulate a previous Lookup having stashed a symbol.
	p.symbol = "fmt.Println"
	p.SetDoc("manually injected body")
	if s := p.Symbol(); s != "" {
		t.Errorf("Symbol() after SetDoc = %q, want \"\"", s)
	}
}

// TestQuickHelpLookupEmptyIsNoop checks that Lookup with an empty (or
// whitespace-only) symbol is a no-op: no panic, no subprocess, no state
// change. The host can bind Lookup straight to a "word under cursor"
// hook without guarding the call.
func TestQuickHelpLookupEmptyIsNoop(t *testing.T) {
	p := NewQuickHelpPanel()
	p.SetDoc("existing doc")
	p.Lookup("")
	p.Lookup("   ")
	if got := p.Doc(); got != "existing doc" {
		t.Errorf("Doc() after empty Lookup = %q, want %q", got, "existing doc")
	}
	if s := p.Symbol(); s != "" {
		t.Errorf("Symbol() after empty Lookup = %q, want \"\"", s)
	}
}

// TestRunGoDocEmptySymbol confirms the small free helper short-circuits
// on an empty symbol without touching exec.Command. This is the only
// branch of runGoDoc that can be deterministically asserted without a
// live `go` on PATH.
func TestRunGoDocEmptySymbol(t *testing.T) {
	out, err := runGoDoc("", "")
	if out != "" {
		t.Errorf("runGoDoc(\"\") output = %q, want empty", out)
	}
	if err != nil {
		t.Errorf("runGoDoc(\"\") err = %v, want nil", err)
	}
}

// TestQuickHelpLookupSmoke is the optional integration smoke test: if
// the host has a `go` toolchain on PATH, run a real `go doc fmt.Println`
// and assert the captured output mentions "Println". Skipped cleanly
// when `go` is not available so CI environments without a toolchain
// don't fail. `go doc fmt.Println` is fast (~50ms warm), so this stays
// in the regular test set.
func TestQuickHelpLookupSmoke(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; skipping go-doc smoke test")
	}
	p := NewQuickHelpPanel()
	p.Lookup("fmt.Println")
	if s := p.Symbol(); s != "fmt.Println" {
		t.Errorf("Symbol() = %q, want fmt.Println", s)
	}
	doc := p.Doc()
	if doc == "" {
		t.Fatal("Doc() is empty after Lookup(fmt.Println)")
	}
	if !strings.Contains(doc, "Println") {
		t.Errorf("Doc() does not mention Println; got: %s", doc)
	}
}

// TestQuickHelpPanelFactoryRegistered checks the factory id resolves to
// a constructible *QuickHelpPanel, matching how silkide will instantiate
// it for docking.
func TestQuickHelpPanelFactoryRegistered(t *testing.T) {
	obj := core.New("ged.QuickHelp")
	if _, ok := obj.(*QuickHelpPanel); !ok {
		t.Fatalf("factory ged.QuickHelp built %T, want *QuickHelpPanel", obj)
	}
}

package decl_test

// This test file lives in the *_test package to avoid pulling silk/gui
// into the decl package's own import set — gui's init wires the entire
// widget catalog and registers GLFW callbacks, which the lighter
// decl/decl_test.go suite shouldn't pay for. By landing the gui-aware
// tests here we keep that package boundary clean.

import (
	"testing"

	"silk/decl"
	"silk/gui"
)

// TestBuildFormHasTitle exercises the full pipeline: DSL → AST →
// Build() → live *gui.Form. The title prop must propagate through
// applyProp's titleSetter branch onto the widget; without it the
// Form would render with an empty title.
func TestBuildFormHasTitle(t *testing.T) {
	n := decl.Form(decl.ID("Main"), decl.P("title", "Hello, decl"))
	obj, err := n.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	form, ok := obj.(*gui.Form)
	if !ok {
		t.Fatalf("Build returned %T, want *gui.Form", obj)
	}
	if form.Title() != "Hello, decl" {
		t.Errorf("form.Title() = %q, want Hello, decl", form.Title())
	}
}

// TestBuildButtonHasText pins the textSetter wiring on Button — the
// most-common widget property with the most-common decl prop name.
func TestBuildButtonHasText(t *testing.T) {
	n := decl.Button(decl.ID("ok"), decl.P("text", "OK"))
	obj, err := n.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	btn, ok := obj.(*gui.Button)
	if !ok {
		t.Fatalf("Build returned %T, want *gui.Button", obj)
	}
	if btn.Text() != "OK" {
		t.Errorf("btn.Text() = %q, want OK", btn.Text())
	}
}

// TestBuildAttachesChildrenToForm: a Form built with a child Button
// must see that child's parent as the form. attachChild does this via
// reflection; the test pins the round-trip so a future SetParent
// signature change is caught immediately.
func TestBuildAttachesChildrenToForm(t *testing.T) {
	n := decl.Form(decl.ID("Main"),
		decl.Child(decl.Button(decl.ID("ok"), decl.P("text", "OK"))),
	)
	obj, err := n.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	form, ok := obj.(*gui.Form)
	if !ok {
		t.Fatalf("Build returned %T, want *gui.Form", obj)
	}
	// Form's own Children() walks its widget subtree. We don't pin a
	// specific count because Form may inject decoration widgets, but a
	// non-zero count proves SetParent fired.
	if len(form.Children()) == 0 {
		t.Errorf("form has no children; SetParent did not attach")
	}
	// The button must be findable by its ID through the widget tree.
	var found bool
	for _, c := range form.Children() {
		if b, ok := c.(*gui.Button); ok && b.Text() == "OK" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("declared OK button not found among form children")
	}
}

// TestBuildLabelTextRoundTripsViaTDoc: a Label authored in code,
// serialised to TDoc, parsed back, and Built must end up with the
// same text. This is the end-to-end "designer can read code; runtime
// can read designer" guarantee.
func TestBuildLabelTextRoundTripsViaTDoc(t *testing.T) {
	orig := decl.Label(decl.ID("greeting"), decl.P("text", "Hi from decl"))
	doc := decl.ToTDoc(orig)
	parsed, err := decl.FromTDoc(doc)
	if err != nil {
		t.Fatalf("FromTDoc: %v", err)
	}
	obj, err := parsed.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	lbl, ok := obj.(*gui.Label)
	if !ok {
		t.Fatalf("Build returned %T, want *gui.Label", obj)
	}
	if lbl.Text() != "Hi from decl" {
		t.Errorf("lbl.Text() = %q, want Hi from decl", lbl.Text())
	}
}

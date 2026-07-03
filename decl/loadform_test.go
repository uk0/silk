package decl_test

// loadform_test.go pins the on-disk contract: a *decl.Node serialised
// via ToTDoc must be loadable by gui.LoadForm — the same entry the
// designer (.silkui files) uses. If this test fails, the designer
// can no longer read decl-authored designs and the whole "two
// projections of one AST" promise is broken.
//
// Lives in *_test (not _test in package decl) because it imports
// silk/gui's full widget catalog, which the lighter decl test file
// does not pay for.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/gui"
)

// TestSilkuiOnDiskRoundTrip writes a decl tree to a real .silkui file
// (TDoc text format), reads it back via gui.LoadForm, and asserts the
// resulting *gui.Form has the title and child count we declared.
func TestSilkuiOnDiskRoundTrip(t *testing.T) {
	tree := decl.Form(decl.ID("Main"),
		decl.P("title", "Round-Trip Demo"),
		decl.Children(
			decl.Label(decl.ID("greeting"), decl.P("text", "Hello")),
			decl.Button(decl.ID("ok"), decl.P("text", "OK")),
		),
	)

	doc := decl.ToTDoc(tree)
	if doc == nil {
		t.Fatalf("ToTDoc returned nil")
	}

	// Write to a real file in t.TempDir so the loader exercises its
	// own file-IO path (not just an in-memory TDoc shortcut).
	dir := t.TempDir()
	path := filepath.Join(dir, "round_trip.silkui")
	if err := os.WriteFile(path, []byte(doc.String()), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	form, err := gui.LoadForm(path)
	if err != nil {
		t.Fatalf("gui.LoadForm: %v", err)
	}

	// LoadForm reads "title" via the Form Title attribute; without our
	// codec writing that key, this assertion would fail.
	if form.Title() != "Round-Trip Demo" {
		t.Errorf("form.Title() = %q, want Round-Trip Demo", form.Title())
	}

	// Children attached during load. We don't pin a precise count
	// because the designer dialect parents children directly under the
	// form; we just confirm at least the two we declared appear and
	// have the expected types.
	var sawLabel, sawButton bool
	var labelText, buttonText string
	for _, c := range form.Children() {
		switch w := c.(type) {
		case *gui.Label:
			sawLabel = true
			labelText = w.Text()
		case *gui.Button:
			sawButton = true
			buttonText = w.Text()
		}
	}
	if !sawLabel {
		t.Errorf("Label child missing after on-disk round-trip")
	}
	if !sawButton {
		t.Errorf("Button child missing after on-disk round-trip")
	}
	if labelText != "Hello" {
		t.Errorf("Label.Text() = %q, want Hello", labelText)
	}
	if buttonText != "OK" {
		t.Errorf("Button.Text() = %q, want OK", buttonText)
	}
}

// TestSilkuiInMemoryRoundTripViaLoadFormFromDoc shortcuts the file
// system: a decl tree → ToTDoc → gui.LoadFormFromDoc directly. This
// pins the in-memory contract independently of any TDoc text encoder
// quirks, so a future change to the .silkui text format can't mask
// a regression in the structural codec.
func TestSilkuiInMemoryRoundTripViaLoadFormFromDoc(t *testing.T) {
	tree := decl.Form(decl.ID("Main"),
		decl.P("title", "In-Memory"),
		decl.Children(
			decl.Label(decl.ID("greeting"), decl.P("text", "Direct")),
		),
	)

	doc := decl.ToTDoc(tree)
	form, err := gui.LoadFormFromDoc(doc)
	if err != nil {
		t.Fatalf("LoadFormFromDoc: %v", err)
	}
	if form.Title() != "In-Memory" {
		t.Errorf("form.Title() = %q, want In-Memory", form.Title())
	}

	var saw bool
	for _, c := range form.Children() {
		if lbl, ok := c.(*gui.Label); ok && lbl.Text() == "Direct" {
			saw = true
			break
		}
	}
	if !saw {
		t.Errorf("Label not found after in-memory round-trip")
	}
}

// TestSilkuiTextDumpVisible writes a decl tree's TDoc text to t.Log
// so a CI run captures the on-disk format alongside the test result.
// Useful when the designer renders something differently from what
// we expect — the dump lets a human compare side by side.
func TestSilkuiTextDumpVisible(t *testing.T) {
	tree := decl.Form(decl.ID("Main"),
		decl.P("title", "DumpDemo"),
		decl.Children(decl.Button(decl.ID("ok"), decl.P("text", "OK"))),
	)
	doc := decl.ToTDoc(tree)
	t.Logf("decl→silkui text:\n%s", doc.String())
	if doc.String() == "" {
		t.Errorf("TDoc.String() returned empty")
	}
	// Quiet the unused-import on core in this file when its use is only
	// a transitive type — without this reference the build complains.
	_ = core.NewTDoc
}

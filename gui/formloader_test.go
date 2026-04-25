package gui

import (
	"os"
	"path/filepath"
	"testing"

	"silk/core"
	"silk/geom"
)

// TestLoadFormFromDoc_LegacyFormat exercises the loader against the exact TDoc
// shape produced by ged.GedScene.SaveDesign / ged.FakeWidget.SaveDesign. No
// ged package dependency is used — we build the document by hand so the test
// also demonstrates that the design format is consumable through just
// silk/core + silk/gui.
func TestLoadFormFromDoc_LegacyFormat(t *testing.T) {
	// Build a designer-style document containing a Form at 200x150mm with a
	// Button ("OK") and a CheckBox ("Remember me") child.
	root := core.NewTDoc()
	if err := root.SetValue("form"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	root.WriteAttr("bounds", geom.Rect{X: 0, Y: 0, Width: 200, Height: 150})
	root.WriteAttr("title", "My Dialog")

	children := core.NewTDoc()
	children.SetKey("children")

	btn := core.NewTDoc()
	btn.SetValue("gui.Button")
	btn.WriteAttr("bounds", geom.Rect{X: 10, Y: 10, Width: 50, Height: 20})
	btn.WriteAttr("text", "OK")
	children.AddChild(btn)

	cb := core.NewTDoc()
	cb.SetValue("gui.CheckBox")
	cb.WriteAttr("bounds", geom.Rect{X: 10, Y: 40, Width: 80, Height: 15})
	cb.WriteAttr("text", "Remember me")
	cb.WriteAttr("checked", true)
	children.AddChild(cb)

	root.AddChild(children)

	form, err := LoadFormFromDoc(root)
	if err != nil {
		t.Fatalf("LoadFormFromDoc: %v", err)
	}
	if form == nil {
		t.Fatal("LoadFormFromDoc returned nil form")
	}

	if got, want := form.Title(), "My Dialog"; got != want {
		t.Errorf("Form.Title() = %q, want %q", got, want)
	}
	if w, h := form.Size(); w <= 0 || h <= 0 {
		t.Errorf("Form.Size() = (%v, %v), expected positive dimensions", w, h)
	}

	kids := form.Children()
	if len(kids) != 2 {
		t.Fatalf("Form.Children() len = %d, want 2", len(kids))
	}

	loadedBtn, ok := kids[0].(*Button)
	if !ok {
		t.Fatalf("child[0] type = %T, want *Button", kids[0])
	}
	if loadedBtn.Text() != "OK" {
		t.Errorf("Button.Text() = %q, want %q", loadedBtn.Text(), "OK")
	}

	loadedCB, ok := kids[1].(*CheckBox)
	if !ok {
		t.Fatalf("child[1] type = %T, want *CheckBox", kids[1])
	}
	if loadedCB.Text() != "Remember me" {
		t.Errorf("CheckBox.Text() = %q, want %q", loadedCB.Text(), "Remember me")
	}
	if !loadedCB.IsChecked() {
		t.Error("CheckBox should be checked after loading")
	}
}

// TestLoadFormFromDoc_NilDoc ensures the loader rejects a nil TDoc cleanly.
func TestLoadFormFromDoc_NilDoc(t *testing.T) {
	if _, err := LoadFormFromDoc(nil); err == nil {
		t.Error("LoadFormFromDoc(nil) = nil error, want error")
	}
}

// TestLoadForm_BadPath ensures the file-based entry point surfaces a real
// I/O error instead of panicking.
func TestLoadForm_BadPath(t *testing.T) {
	if _, err := LoadForm("/nonexistent/definitely-not-a-real.silkui"); err == nil {
		t.Error("LoadForm on a missing path should return an error")
	}
}

// TestSaveAndLoadForm_RoundTrip builds a Form programmatically, saves it to a
// temp .silkui file via SaveForm, then reloads it with LoadForm and verifies
// the resulting widget hierarchy matches. This is the end-to-end SDK story.
func TestSaveAndLoadForm_RoundTrip(t *testing.T) {
	form := NewForm()
	form.SetTitle("Roundtrip")
	form.SetSize(MmToPixelZ(180), MmToPixelZ(120))

	btn := NewButton1("Save", nil)
	btn.SetParent(form)
	btn.SetBounds(MmToPixelZ(5), MmToPixelZ(5), MmToPixelZ(40), MmToPixelZ(18))

	cb := NewCheckBox()
	cb.SetText("Auto")
	cb.SetChecked(true)
	cb.SetParent(form)
	cb.SetBounds(MmToPixelZ(5), MmToPixelZ(30), MmToPixelZ(40), MmToPixelZ(15))

	tmpDir := t.TempDir()
	// Deliberately leave off the extension so SaveForm appends .silkui.
	base := filepath.Join(tmpDir, "roundtrip")
	if err := SaveForm(form, base); err != nil {
		t.Fatalf("SaveForm: %v", err)
	}

	// Ensure the extension was appended.
	saved := base + ".silkui"
	if _, err := os.Stat(saved); err != nil {
		t.Fatalf("expected %q to exist: %v", saved, err)
	}

	loaded, err := LoadForm(saved)
	if err != nil {
		t.Fatalf("LoadForm: %v", err)
	}
	if loaded.Title() != "Roundtrip" {
		t.Errorf("loaded form title = %q, want %q", loaded.Title(), "Roundtrip")
	}
	kids := loaded.Children()
	if len(kids) != 2 {
		t.Fatalf("loaded form children = %d, want 2", len(kids))
	}

	var gotButton bool
	var gotCheckBox bool
	for _, k := range kids {
		switch w := k.(type) {
		case *Button:
			gotButton = true
			if w.Text() != "Save" {
				t.Errorf("Button.Text() after round trip = %q, want %q", w.Text(), "Save")
			}
		case *CheckBox:
			gotCheckBox = true
			if w.Text() != "Auto" {
				t.Errorf("CheckBox.Text() after round trip = %q, want %q", w.Text(), "Auto")
			}
			if !w.IsChecked() {
				t.Error("CheckBox should stay checked across save/load")
			}
		}
	}
	if !gotButton || !gotCheckBox {
		t.Errorf("missing widgets after round trip: button=%v checkbox=%v", gotButton, gotCheckBox)
	}
}

// TestHasExt confirms the internal extension detector handles the edge cases
// that matter for SaveForm (no extension, trailing dot, path separators in
// directory names containing dots).
func TestHasExt(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"foo", false},
		{"foo.silkui", true},
		{"foo.", false},
		{"dir.with.dots/foo", false},
		{"dir/foo.silkui", true},
	}
	for _, c := range cases {
		if got := hasExt(c.in); got != c.want {
			t.Errorf("hasExt(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

package ged

import (
	"testing"
)

// TestReadFormSettingsSnapshotsScene: the dialog opens on the scene's current
// values, so a cancelled dialog cannot be distinguished from an unedited one.
func TestReadFormSettingsSnapshotsScene(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("登录")
	scene.SetSize(160, 120)
	scene.SetGrid(GridModel{Pitch: 8, Visible: false, Snap: false})

	got := readFormSettings(scene)
	want := formSettings{
		Title:  "登录",
		Width:  160,
		Height: 120,
		Grid:   GridModel{Pitch: 8, Visible: false, Snap: false},
	}
	if got != want {
		t.Errorf("readFormSettings() = %+v, want %+v", got, want)
	}
}

// TestParseFormSettingsRejectsGarbage: a numeric field that does not parse
// keeps its current value instead of collapsing to zero — the property sheet's
// rule. A width of "" must not shrink the form to nothing.
func TestParseFormSettingsRejectsGarbage(t *testing.T) {
	cur := formSettings{
		Title:  "Form",
		Width:  100,
		Height: 80,
		Grid:   GridModel{Pitch: 5, Visible: true, Snap: true},
	}

	got := parseFormSettings(cur, "登录", "", "abc", "5mm", true, true)
	if got.Width != 100 {
		t.Errorf("width after empty input = %g, want 100 unchanged", got.Width)
	}
	if got.Height != 80 {
		t.Errorf("height after %q = %g, want 80 unchanged", "abc", got.Height)
	}
	if got.Grid.Pitch != 5 {
		t.Errorf("pitch after %q = %g, want 5 unchanged", "5mm", got.Grid.Pitch)
	}
	// The title is free text; anything typed is accepted verbatim.
	if got.Title != "登录" {
		t.Errorf("title = %q, want %q", got.Title, "登录")
	}
}

// TestParseFormSettingsClampsRange: a value that parses but sits outside its
// range is clamped, not rejected — typing 0.1 should give the smallest legal
// pitch rather than silently doing nothing.
func TestParseFormSettingsClampsRange(t *testing.T) {
	cur := formSettings{
		Title:  "Form",
		Width:  100,
		Height: 80,
		Grid:   GridModel{Pitch: 5, Visible: true, Snap: true},
	}

	got := parseFormSettings(cur, "Form", "2", "-40", "0.1", true, true)
	if got.Width != minFormSize {
		t.Errorf("width from %q = %g, want %g", "2", got.Width, minFormSize)
	}
	if got.Height != minFormSize {
		t.Errorf("height from %q = %g, want %g", "-40", got.Height, minFormSize)
	}
	if got.Grid.Pitch != minGridPitch {
		t.Errorf("pitch from %q = %g, want %g", "0.1", got.Grid.Pitch, minGridPitch)
	}

	got = parseFormSettings(cur, "Form", "300", "200", "999", false, false)
	if got.Width != 300 || got.Height != 200 {
		t.Errorf("size = (%g, %g), want (300, 200)", got.Width, got.Height)
	}
	if got.Grid.Pitch != maxGridPitch {
		t.Errorf("pitch from %q = %g, want %g", "999", got.Grid.Pitch, maxGridPitch)
	}
	if got.Grid.Visible || got.Grid.Snap {
		t.Errorf("checkbox state = %+v, want both false", got.Grid)
	}
}

// TestApplyFormSettingsUndo: title and size touch the scene, so one undo must
// put both back. The grid prefs are plain state and deliberately survive the
// undo — they are a view preference that happens to be stored with the file,
// not part of the design the user is editing.
func TestApplyFormSettingsUndo(t *testing.T) {
	scene := NewGedScene()
	before := readFormSettings(scene)

	next := before
	next.Title = "登录"
	next.Width, next.Height = 200, 150
	next.Grid.Pitch = 10
	next.Grid.Snap = false

	if !applyFormSettings(scene, next) {
		t.Fatal("applyFormSettings reported no change")
	}
	if scene.FormTitle() != "登录" {
		t.Errorf("title = %q, want %q", scene.FormTitle(), "登录")
	}
	if _, _, w, h := scene.Bounds(); w != 200 || h != 150 {
		t.Errorf("size = (%g, %g), want (200, 150)", w, h)
	}
	if g := scene.Grid(); g.Pitch != 10 || g.Snap {
		t.Errorf("grid = %+v, want pitch 10 and snap off", g)
	}

	scene.UndoStack().Undo()

	if scene.FormTitle() != before.Title {
		t.Errorf("title after undo = %q, want %q", scene.FormTitle(), before.Title)
	}
	if _, _, w, h := scene.Bounds(); w != before.Width || h != before.Height {
		t.Errorf("size after undo = (%g, %g), want (%g, %g)", w, h, before.Width, before.Height)
	}
	if g := scene.Grid(); g.Pitch != 10 || g.Snap {
		t.Errorf("grid after undo = %+v, want the applied prefs to survive", g)
	}

	scene.UndoStack().Redo()
	if scene.FormTitle() != "登录" {
		t.Errorf("title after redo = %q, want %q", scene.FormTitle(), "登录")
	}
	if _, _, w, h := scene.Bounds(); w != 200 || h != 150 {
		t.Errorf("size after redo = (%g, %g), want (200, 150)", w, h)
	}
}

// TestApplyFormSettingsGridOnlyPushesNothing: changing only the grid prefs must
// not leave an empty undo step behind for the user to step through.
func TestApplyFormSettingsGridOnlyPushesNothing(t *testing.T) {
	scene := NewGedScene()

	next := readFormSettings(scene)
	next.Grid.Visible = false

	if !applyFormSettings(scene, next) {
		t.Fatal("applyFormSettings reported no change")
	}
	if scene.Grid().Visible {
		t.Errorf("grid still visible after applying visible=false")
	}
	if scene.UndoStack().CanUndo() {
		t.Errorf("grid-only edit pushed an undo command")
	}
}

// TestApplyFormSettingsNoOp: re-opening the dialog and pressing 确定 without
// touching anything must not dirty the document.
func TestApplyFormSettingsNoOp(t *testing.T) {
	scene := NewGedScene()

	if applyFormSettings(scene, readFormSettings(scene)) {
		t.Errorf("applying the current settings reported a change")
	}
	if scene.UndoStack().CanUndo() {
		t.Errorf("no-op apply pushed an undo command")
	}
}

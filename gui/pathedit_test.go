package gui

import (
	"path/filepath"
	"testing"

	"silk/core"
)

// TestPathEditTextRoundTrip: SetText writes the value and Text reads
// it back. A redundant SetText(current) is a cheap no-op — the
// change callback must not refire for it.
func TestPathEditTextRoundTrip(t *testing.T) {
	p := NewPathEdit()

	var fired []string
	p.SigPathChanged(func(s string) { fired = append(fired, s) })

	p.SetText("/tmp/a.txt")
	if got := p.Text(); got != "/tmp/a.txt" {
		t.Errorf("Text() = %q, want %q", got, "/tmp/a.txt")
	}

	// Redundant write must not refire.
	p.SetText("/tmp/a.txt")
	// Real change must fire.
	p.SetText("/tmp/b.txt")

	want := []string{"/tmp/a.txt", "/tmp/b.txt"}
	if len(fired) != len(want) {
		t.Fatalf("SigPathChanged fired %v, want %v", fired, want)
	}
	for i := range want {
		if fired[i] != want[i] {
			t.Errorf("fired[%d] = %q, want %q", i, fired[i], want[i])
		}
	}
}

// TestPathEditDefaultMode: PathFile is the default so a freshly
// constructed PathEdit is wired for the most common picker case.
func TestPathEditDefaultMode(t *testing.T) {
	p := NewPathEdit()
	if p.Mode() != PathFile {
		t.Errorf("default Mode = %v, want PathFile", p.Mode())
	}
}

// TestPathEditSetMode: SetMode round-trips for all three PathMode
// values.
func TestPathEditSetMode(t *testing.T) {
	p := NewPathEdit()
	modes := []PathMode{PathFile, PathFolder, PathSaveFile}
	for _, m := range modes {
		p.SetMode(m)
		if p.Mode() != m {
			t.Errorf("after SetMode(%v), Mode() = %v", m, p.Mode())
		}
	}
}

// TestPathEditPlaceholder: SetPlaceholder is read back by Placeholder
// — used by Draw to decide whether to paint a grey hint.
func TestPathEditPlaceholder(t *testing.T) {
	p := NewPathEdit()
	p.SetPlaceholder("Select a file…")
	if got := p.Placeholder(); got != "Select a file…" {
		t.Errorf("Placeholder() = %q, want %q", got, "Select a file…")
	}
}

// TestPathEditReadOnlyBlocksTyping: SetReadOnly stops OnTextInput
// from mutating the field. The browse button stays live (covered by
// TestPathEditBrowseButtonClickOpensDialog), so a read-only PathEdit
// can still receive a dialog-picked path.
func TestPathEditReadOnlyBlocksTyping(t *testing.T) {
	p := NewPathEdit()
	p.SetText("orig")
	p.SetReadOnly(true)
	if !p.IsReadOnly() {
		t.Fatalf("IsReadOnly() = false after SetReadOnly(true)")
	}
	p.OnTextInput("xyz")
	if p.Text() != "orig" {
		t.Errorf("readonly OnTextInput mutated text to %q, want %q", p.Text(), "orig")
	}
	// Backspace must also be a no-op when read-only.
	p.OnKeyDown(KeyBackSpace, false)
	if p.Text() != "orig" {
		t.Errorf("readonly OnKeyDown(Backspace) mutated text to %q, want %q", p.Text(), "orig")
	}
}

// TestPathEditTypingPath: with read-only off the field appends typed
// characters and Backspace deletes the trailing rune. Confirms the
// underlying input mechanism actually flows through SetText so
// SigPathChanged observes every keystroke.
func TestPathEditTypingPath(t *testing.T) {
	p := NewPathEdit()
	var fired int
	p.SigPathChanged(func(string) { fired++ })

	p.OnTextInput("a")
	p.OnTextInput("b")
	p.OnTextInput("c")
	if got := p.Text(); got != "abc" {
		t.Errorf("after typing 'abc', Text() = %q, want %q", got, "abc")
	}
	p.OnKeyDown(KeyBackSpace, false)
	if got := p.Text(); got != "ab" {
		t.Errorf("after Backspace, Text() = %q, want %q", got, "ab")
	}
	if fired != 4 {
		t.Errorf("SigPathChanged fired %d times, want 4 (a, ab, abc, ab)", fired)
	}
}

// TestButtonHitTestBoundaries: the pure helper that decides whether
// a click landed on the right-side browse-button region. Tests the
// inside, the boundary, and out-of-range x.
func TestButtonHitTestBoundaries(t *testing.T) {
	const w, btnW = 200.0, 32.0
	// Left edge of the button region is w-btnW = 168.
	cases := []struct {
		name string
		x    float64
		want bool
	}{
		{"far-left text region", 0, false},
		{"middle text region", 100, false},
		{"just before button", 167.999, false},
		{"exact button left edge", 168, true},
		{"button middle", 184, true},
		{"button right edge - inside", 199.999, true},
		{"at total width - outside", 200, false},
		{"past widget", 250, false},
		{"negative x", -1, false},
	}
	for _, c := range cases {
		got := buttonHitTest(c.x, w, btnW)
		if got != c.want {
			t.Errorf("buttonHitTest(%v, %v, %v) = %v, want %v",
				c.x, w, btnW, got, c.want)
		}
	}

	// Defensive: a zero-width widget or button should never claim a hit.
	if buttonHitTest(0, 0, 32) {
		t.Errorf("buttonHitTest with zero widget width should be false")
	}
	if buttonHitTest(0, 200, 0) {
		t.Errorf("buttonHitTest with zero button width should be false")
	}
}

// TestPathEditButtonClickInvokesOpenFn: clicking inside the button
// region must call the injected openFn with the current mode. We
// use a stub instead of the real dialog because the real one needs
// a window + event loop and would block headless tests.
func TestPathEditButtonClickInvokesOpenFn(t *testing.T) {
	p := NewPathEdit()
	p.SetSize(200, 28)
	p.SetMode(PathSaveFile)

	var calledWith PathMode
	var called int
	p.SetOpenFn(func(mode PathMode) (string, bool) {
		called++
		calledWith = mode
		return "/picked/file.silkui", true
	})

	var fired []string
	p.SigPathChanged(func(s string) { fired = append(fired, s) })

	// Click in the right-most 32px (the button region for a 200px wide
	// widget). 190 is well inside [168, 200).
	p.OnLeftDown(190, 14)

	if called != 1 {
		t.Errorf("openFn called %d times, want 1", called)
	}
	if calledWith != PathSaveFile {
		t.Errorf("openFn received mode %v, want PathSaveFile", calledWith)
	}
	if p.Text() != "/picked/file.silkui" {
		t.Errorf("after dialog, Text() = %q, want %q", p.Text(), "/picked/file.silkui")
	}
	if len(fired) != 1 || fired[0] != "/picked/file.silkui" {
		t.Errorf("SigPathChanged fired %v, want [/picked/file.silkui]", fired)
	}
}

// TestPathEditTextRegionClickNoDialog: clicking in the text region
// (anywhere left of w-btnW) must NOT invoke the dialog and must NOT
// mutate the text. It just grabs focus for typing.
func TestPathEditTextRegionClickNoDialog(t *testing.T) {
	p := NewPathEdit()
	p.SetSize(200, 28)
	p.SetText("kept")

	var called int
	p.SetOpenFn(func(PathMode) (string, bool) {
		called++
		return "/should/not/appear", true
	})

	p.OnLeftDown(20, 14) // well inside the text region

	if called != 0 {
		t.Errorf("openFn called %d times for a text-region click, want 0", called)
	}
	if p.Text() != "kept" {
		t.Errorf("text-region click mutated Text() to %q, want %q", p.Text(), "kept")
	}
}

// TestPathEditOpenFnCancelLeavesText: when the dialog returns ok=false
// (user cancelled) or an empty path, the existing text must stay put
// and SigPathChanged must not fire.
func TestPathEditOpenFnCancelLeavesText(t *testing.T) {
	p := NewPathEdit()
	p.SetSize(200, 28)
	p.SetText("existing")

	p.SetOpenFn(func(PathMode) (string, bool) {
		return "", false // user cancelled
	})

	var fired int
	p.SigPathChanged(func(string) { fired++ })

	p.OnLeftDown(190, 14)

	if p.Text() != "existing" {
		t.Errorf("after cancel, Text() = %q, want %q", p.Text(), "existing")
	}
	if fired != 0 {
		t.Errorf("after cancel, SigPathChanged fired %d times, want 0", fired)
	}
}

// TestPathEditSetOpenFnNilRestoresDefault: passing nil to SetOpenFn
// reinstalls the real-dialog driver instead of crashing on a nil
// dispatch. We can't drive the real dialog from a test, but we can
// confirm openFn is non-nil after the reset.
func TestPathEditSetOpenFnNilRestoresDefault(t *testing.T) {
	p := NewPathEdit()
	p.SetOpenFn(func(PathMode) (string, bool) { return "", false })
	p.SetOpenFn(nil)
	if p.openFn == nil {
		t.Errorf("SetOpenFn(nil) left openFn nil; want default driver restored")
	}
}

// TestResolvePickedPath: the pure helper that maps the dialog's raw
// return value to what lands in the field. PathFolder simulates a
// folder picker by taking the parent dir of the picked file (the
// GLFW build has no native folder dialog); PathFile / PathSaveFile
// pass the path through unchanged; an empty raw stays empty.
func TestResolvePickedPath(t *testing.T) {
	cases := []struct {
		mode PathMode
		raw  string
		want string
	}{
		{PathFile, "/tmp/a.silkui", "/tmp/a.silkui"},
		{PathSaveFile, "/tmp/save.silkui", "/tmp/save.silkui"},
		{PathFolder, "/tmp/sub/a.txt", filepath.Dir("/tmp/sub/a.txt")},
		{PathFile, "", ""},
		{PathFolder, "", ""},
	}
	for _, c := range cases {
		got := resolvePickedPath(c.mode, c.raw)
		if got != c.want {
			t.Errorf("resolvePickedPath(%v, %q) = %q, want %q",
				c.mode, c.raw, got, c.want)
		}
	}
}

// TestPathEditFactoryRegistered: gui.PathEdit is registered with the
// factory system so the visual designer can instantiate it from the
// widget palette in ged.
func TestPathEditFactoryRegistered(t *testing.T) {
	for _, f := range core.AllFactories() {
		if f.Name() != "gui.PathEdit" {
			continue
		}
		obj := f.New()
		if _, ok := obj.(*PathEdit); !ok {
			t.Errorf("factory(gui.PathEdit).New() returned %T, want *PathEdit", obj)
		}
		return
	}
	t.Fatalf("factory %q not registered", "gui.PathEdit")
}

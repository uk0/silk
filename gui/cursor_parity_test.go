package gui

import (
	"os"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// noCursorResources skips a test that only describes the empty configuration.
func noCursorResources(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(core.ResourceDir() + "/cursor"); err == nil {
		t.Skip("cursor resources are installed; this test covers the empty case")
	}
}

// TestDropCursorsWithoutResourcesAreUsable pins the configuration this repo
// actually ships: nothing installs <ResourceDir>/cursor, so every
// LoadCursorData call inside GenerateDropCursors fails and the result is built
// entirely from fallbacks. Both drag backends index cursors[0..3] without
// checking — dnd_windows.go hands them to SetCursor, dnd_glfw.go to
// SetOverrideCursor — so a short slice or a nil entry breaks every drag before
// the first mouse move.
func TestDropCursorsWithoutResourcesAreUsable(t *testing.T) {
	noCursorResources(t)

	curs := GenerateDropCursors(paint.NewPixmap(32, 16))
	if len(curs) != 4 {
		t.Fatalf("GenerateDropCursors returned %d cursors, want 4", len(curs))
	}
	for i, c := range curs {
		if c == nil {
			t.Errorf("cursors[%d] is nil", i)
		}
	}
}

// TestLoadCursorFallsBackAndCachesMissingName pins LoadCursor's negative
// caching. graph/resize-decor.go calls it from the hover path, so an
// unresolvable name must resolve once to the arrow and stay cached; without
// that every mouse move re-opens and re-scans <ResourceDir>/cursor, and a nil
// result would reach the SetCursor path that dereferences it.
func TestLoadCursorFallsBackAndCachesMissingName(t *testing.T) {
	noCursorResources(t)

	const name = "silk-test-no-such-cursor"
	defer delete(cursorCache, name)

	got := LoadCursor(name)
	if got == nil {
		t.Fatal("LoadCursor returned nil for an unresolvable name")
	}
	if got != DefaultCursor() {
		t.Errorf("LoadCursor(%q) did not fall back to the default arrow", name)
	}
	if cached := cursorCache[name]; cached != got {
		t.Errorf("LoadCursor(%q) left %v in the cache, want its own result %v", name, cached, got)
	}
}

// TestNewCursorFromDataReportsFailure pins the contract GenerateDropCursors
// relies on: a cursor that could not be built comes back as (nil, error), not
// as a Cursor wrapping a null native handle. The Win32 backend used to wrap
// whatever CreateIconIndirect returned, and SetCursor on a null handle hides
// the pointer instead of failing visibly.
func TestNewCursorFromDataReportsFailure(t *testing.T) {
	cur, err := NewCursorFromData(CursorData{})
	if cur != nil || err == nil {
		t.Errorf("NewCursorFromData(zero) = %v, %v; want nil and an error", cur, err)
	}
}

// TestDropCursorsFallBackWhenCursorCreationFails covers the half of the empty
// configuration that no runtime test can reach on both backends at once: even
// with the resources installed, NewCursorFromData fails when cairo refuses the
// composed pixmap or the platform refuses the handle, leaving a nil in the
// slice the drag code indexes blindly. Only the GLFW backend substituted the
// arrow; a runtime test can only ever exercise the backend it was compiled
// for, which is why this reads both sources.
func TestDropCursorsFallBackWhenCursorCreationFails(t *testing.T) {
	for _, file := range []string{"cursor_windows.go", "cursor_glfw.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("cannot read %s: %v", file, err)
		}
		body := generateDropCursorsBody(t, file, string(src))
		if !strings.Contains(body, "cur == nil") {
			t.Errorf("%s: GenerateDropCursors appends the result of NewCursorFromData without checking it; a failed cursor reaches SetCursor as nil", file)
		}
	}
}

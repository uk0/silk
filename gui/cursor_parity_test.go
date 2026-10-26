package gui

import (
	"errors"
	"os"
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

// Both drag backends index all four entries. Built-in assets must work when
// an SDK application ships no optional cursor directory; native-unavailable
// test environments must still receive non-nil shared fallbacks.
func TestDropCursorsWithoutResourcesAreUsable(t *testing.T) {
	noCursorResources(t)

	pix := paint.NewPixmap(32, 16)
	defer paint.CloseSurface(pix)
	curs := GenerateDropCursors(pix)
	defer ReleaseDropCursors(curs)
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

// Exercise the actual shared generator, including a failed native allocator.
func TestDropCursorsFallBackWhenCursorCreationFails(t *testing.T) {
	calls := 0
	curs := generateDropCursors(nil, func(data CursorData) (*Cursor, error) { calls++; return nil, errors.New("native allocation failed") })
	if calls != 4 || len(curs) != 4 {
		t.Fatal("incomplete feedback", calls, len(curs))
	}
	for _, cur := range curs {
		if cur == nil {
			t.Fatal("nil cursor escapes allocator failure")
		}
	}
	ReleaseDropCursors(curs)
	if DefaultCursor() == nil {
		t.Fatal("shared fallback destroyed")
	}
}

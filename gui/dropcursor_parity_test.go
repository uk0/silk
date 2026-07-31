package gui

import (
	"os"
	"strings"
	"testing"
)

// TestDropCursorsSurviveMissingResources covers a nil dereference that only
// ever fired on Windows: LoadCursorData leaves a zero CursorData behind when
// the "cursor" resource directory is absent — which it is in every install
// that does not ship one — and GenerateDropCursors then walked straight into
// arrow.Pixmap.Width(). The panic happened inside the WM_MOUSEMOVE handler,
// where wndProcFunc recovers it, so the only symptom was that nothing in the
// widget palette could be dragged.
//
// The GLFW backend had grown the guard; the Win32 one had not. Both are
// checked here because the divergence is the actual defect — a runtime test
// can only ever run on one of them at a time.
func TestDropCursorsSurviveMissingResources(t *testing.T) {
	for _, file := range []string{"cursor_windows.go", "cursor_glfw.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("cannot read %s: %v", file, err)
		}
		body := generateDropCursorsBody(t, file, string(src))
		if !strings.Contains(body, "arrow.Pixmap == nil") {
			t.Errorf("%s: GenerateDropCursors does not guard against a cursor that failed to load; dragging will panic wherever the resources are missing", file)
		}
	}
}

// generateDropCursorsBody returns the text of GenerateDropCursors in src.
func generateDropCursorsBody(t *testing.T, file, src string) string {
	t.Helper()
	const header = "func GenerateDropCursors("
	start := strings.Index(src, header)
	if start < 0 {
		t.Fatalf("%s has no GenerateDropCursors", file)
	}
	rest := src[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("%s: GenerateDropCursors is not terminated", file)
	}
	return rest[:end]
}

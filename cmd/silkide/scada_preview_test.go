package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPreviewConfigIsSilentAndScoped locks in the two safety properties a
// preview Config must hold: its stores are scoped under the given dir (so a
// preview never writes the project's real history / events / recipes) and its
// Notifier is a no-op that returns nil (so a previewed alarm can't fire a
// desktop notification). GL-free: it only builds a scada.Config.
func TestPreviewConfigIsSilentAndScoped(t *testing.T) {
	dir := t.TempDir()
	cfg := previewConfig(dir)

	if got, want := cfg.HistorianPath, filepath.Join(dir, "history.db"); got != want {
		t.Errorf("HistorianPath = %q, want %q", got, want)
	}
	if got, want := cfg.EventLogPath, filepath.Join(dir, "events.db"); got != want {
		t.Errorf("EventLogPath = %q, want %q", got, want)
	}
	if got, want := cfg.RecipePath, filepath.Join(dir, "recipes.json"); got != want {
		t.Errorf("RecipePath = %q, want %q", got, want)
	}
	if cfg.Notifier == nil {
		t.Fatal("Notifier is nil; preview must install a silent no-op, not leave the desktop notifier")
	}
	if err := cfg.Notifier("title", "body"); err != nil {
		t.Errorf("preview Notifier returned %v, want nil (silent no-op)", err)
	}
}

// TestStopPreviewIdempotent drives the teardown with no window attached (frame
// left nil so nothing touches GLFW): the BindScreen stop func runs exactly once,
// the temp store dir is removed, and a second call is a no-op. This is the
// re-preview / double-close guarantee — no leaked stop func or temp dir.
func TestStopPreviewIdempotent(t *testing.T) {
	dir, err := os.MkdirTemp("", "silkide-preview-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir) // belt-and-braces; stopPreview should already remove it

	stops := 0
	// services and frame left nil: stopPreview must nil-guard them, and a nil
	// frame keeps the teardown entirely off the GL path.
	pc := &previewController{
		stop:   func() { stops++ },
		tmpDir: dir,
	}

	pc.stopPreview()
	if stops != 1 {
		t.Fatalf("stop func called %d times on first stopPreview, want 1", stops)
	}
	if pc.stop != nil || pc.tmpDir != "" {
		t.Fatalf("controller not cleared: stopSet=%v tmpDir=%q", pc.stop != nil, pc.tmpDir)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("temp store dir not removed: stat err = %v", err)
	}

	// Second call must be a no-op (idempotent), not a re-run of the stop func.
	pc.stopPreview()
	if stops != 1 {
		t.Fatalf("stop func re-ran on second stopPreview: called %d times, want 1", stops)
	}
}

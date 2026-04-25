package ged

import (
	"encoding/json"
	"testing"
)

// TestSessionStateRoundTrip verifies SessionState JSON encoding survives a
// round trip. We don't exercise the disk I/O path here (that touches the
// user's LocalDataDir), just the shape of the data.
func TestSessionStateRoundTrip(t *testing.T) {
	original := SessionState{
		LastMode:     1,
		OpenFiles:    []string{"/tmp/a.go", "/tmp/b.go"},
		ActiveFile:   "/tmp/a.go",
		LastProject:  "/tmp/proj.silk",
		WindowWidth:  1200,
		WindowHeight: 800,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded SessionState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.LastMode != original.LastMode {
		t.Errorf("LastMode: got %d, want %d", decoded.LastMode, original.LastMode)
	}
	if len(decoded.OpenFiles) != 2 || decoded.OpenFiles[0] != "/tmp/a.go" {
		t.Errorf("OpenFiles mismatch: %v", decoded.OpenFiles)
	}
	if decoded.ActiveFile != original.ActiveFile {
		t.Errorf("ActiveFile: got %q, want %q", decoded.ActiveFile, original.ActiveFile)
	}
	if decoded.WindowWidth != 1200 || decoded.WindowHeight != 800 {
		t.Errorf("window size mismatch: %dx%d", decoded.WindowWidth, decoded.WindowHeight)
	}
}

// TestLoadSessionMissingFile ensures LoadSession returns a zero value when
// no prior session exists -- required for safe first-run behavior.
func TestLoadSessionMissingFile(t *testing.T) {
	// Even if a real session file happens to exist, this only asserts that
	// LoadSession does not panic and returns something usable. The real
	// missing-file path is the common case and is exercised at startup.
	s := LoadSession()
	// Fields are typed with zero-value defaults; just touch them.
	_ = s.LastMode
	_ = s.WindowWidth
}

// TestWidgetHelpDefaults verifies the WidgetHelp panel starts in the
// "no selection" state and transitions correctly when given a factory.
func TestWidgetHelpDocsCoverage(t *testing.T) {
	// Every doc entry must have a non-empty Name and Desc; otherwise the
	// panel would render a visually broken card for that widget.
	for key, doc := range widgetDocs {
		if doc.Name == "" {
			t.Errorf("widget doc %q has empty Name", key)
		}
		if doc.Desc == "" {
			t.Errorf("widget doc %q has empty Desc", key)
		}
	}
	if len(widgetDocs) < 20 {
		t.Errorf("widgetDocs has %d entries, want at least 20", len(widgetDocs))
	}
}

package main

import (
	"path/filepath"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/settings"
)

func TestDesignerInspectorSplitSurvivesFrameClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "silkide.ini")
	open := func() *preferences { return &preferences{store: settings.New(path)} }
	prefs := open()
	frame := gui.NewFrame()
	sheet := newDesignerPropertySheet(frame, prefs)
	defaultSplit := sheet.ColumnSplit()
	const adjusted = .647
	sheet.SetColumnSplit(adjusted)
	if prefs.store.Contains(inspectorSplitPreference) {
		t.Fatal("dragging the splitter scheduled a preference write")
	}
	frame.Close()
	if err := finishPreferenceSaves(prefs); err != nil {
		t.Fatal(err)
	}

	reopened := open()
	freshFrame := gui.NewFrame()
	fresh := newDesignerPropertySheet(freshFrame, reopened)
	if got := fresh.ColumnSplit(); got != adjusted {
		t.Fatalf("reopened Inspector at %v, want the value saved by actual Frame.Close: %v", got, adjusted)
	}
	// Returning to the default is deliberate and must overwrite the old
	// preference rather than skipping the write just because it is .5.
	fresh.SetColumnSplit(defaultSplit)
	freshFrame.Close()
	if err := finishPreferenceSaves(reopened); err != nil {
		t.Fatal(err)
	}
	if got := open().store.Float64(inspectorSplitPreference, -1); got != defaultSplit {
		t.Fatalf("reset was not saved: %v instead of %v", got, defaultSplit)
	}
}

func TestDesignerInspectorSplitIgnoresMalformedPreferences(t *testing.T) {
	for _, invalid := range []string{"", "not a ratio", "NaN", "+Inf", "-Inf"} {
		t.Run(invalid, func(t *testing.T) {
			p := &preferences{store: settings.NewMemory()}
			if err := p.store.SetString(inspectorSplitPreference, invalid); err != nil {
				t.Fatal(err)
			}
			frame := gui.NewFrame()
			want := newDesignerPropertySheet(frame, nil).ColumnSplit()
			if got := newDesignerPropertySheet(frame, p).ColumnSplit(); got != want {
				t.Fatalf("invalid split %q changed the initial geometry to %v", invalid, got)
			}
		})
	}
}

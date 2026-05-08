package ged

import "testing"

// TestWelcomeScreenStoresRecentFiles: SetRecentFiles round-trips the
// list through the widget's internal state. silkide's buildPanels
// feeds preferences.RecentFiles() into the welcome screen at startup;
// without this contract the recent-projects column on the welcome
// page would render empty no matter what the MRU file says.
func TestWelcomeScreenStoresRecentFiles(t *testing.T) {
	w := NewWelcomeScreen()
	files := []string{"/tmp/a.silkui", "/tmp/b.silkui"}
	w.SetRecentFiles(files)
	got := w.recentFiles
	if len(got) != 2 {
		t.Fatalf("recentFiles = %v, want %v", got, files)
	}
	if got[0] != files[0] || got[1] != files[1] {
		t.Errorf("order broken: %v vs %v", got, files)
	}
}

// TestWelcomeScreenCallbacksWired: the three public callback setters
// store the function pointers on the widget. Each callback fires
// from a different click target (New button / Open button / recent-
// file row), so a regression in the slot wiring would silently break
// one of those entry points.
func TestWelcomeScreenCallbacksWired(t *testing.T) {
	w := NewWelcomeScreen()

	var newFired, openFired bool
	var recentArg string
	w.SetNewProjectCallback(func() { newFired = true })
	w.SetOpenFileCallback(func() { openFired = true })
	w.SetOpenRecentCallback(func(p string) { recentArg = p })

	if w.cbNewProject == nil || w.cbOpenFile == nil || w.cbOpenRecent == nil {
		t.Fatal("callbacks not stored")
	}

	w.cbNewProject()
	w.cbOpenFile()
	w.cbOpenRecent("/tmp/x.silkui")

	if !newFired {
		t.Errorf("New callback not invoked")
	}
	if !openFired {
		t.Errorf("Open callback not invoked")
	}
	if recentArg != "/tmp/x.silkui" {
		t.Errorf("Recent callback arg = %q, want /tmp/x.silkui", recentArg)
	}
}

package ged

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func TestAutoSaverHostRequestDoesNotPretendPhysicalSuccess(t *testing.T) {
	s := NewGedScene()
	s.SetFilename(filepath.Join(t.TempDir(), "view.silkui"))
	s.PushCommand(gui.NewCompoundCommand())
	a := NewAutoSaver()
	before := a.lastSave
	calls := 0
	a.SetSaveRequest(func(got *GedScene) {
		calls++
		if got != s {
			t.Error("wrong UI scene")
		}
	})
	a.scene = s
	a.tick()
	if calls != 1 || a.lastSave != before || s.IsClean() {
		t.Fatal("host admission treated as physical success")
	}
	if _, err := os.Stat(s.Filename() + ".autosave"); !os.IsNotExist(err) {
		t.Fatal("host callback used hidden synchronous writer", err)
	}
	a.SetSaveRequest(nil)
	a.tick()
	if _, err := core.LoadTDocFile(s.Filename() + ".autosave"); err != nil {
		t.Fatal("legacy default behavior changed", err)
	}
	a.SetSaveRequest(func(*GedScene) { t.Error("disabled host invoked") })
	a.SetEnabled(false)
	a.tick()
}

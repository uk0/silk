package ged

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func TestDesignSaveToAdoptsIdentityOnlyAfterSuccess(t *testing.T) {
	s := NewGedScene()
	s.SetTitle("original")
	s.PushCommand(gui.NewCompoundCommand())
	oldTitle := s.Title()
	bad := filepath.Join(t.TempDir(), "missing-parent", "failed")
	if err := s.SaveTo(bad); err == nil {
		t.Fatal("save into missing parent succeeded")
	}
	if s.Filename() != "" || s.Title() != oldTitle || s.IsClean() {
		t.Fatal("failed first save changed identity or clean marker")
	}
	path := filepath.Join(t.TempDir(), "design")
	if err := s.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if s.Filename() != path+".silkui" || !s.IsClean() || s.Title() != "design.silkui" {
		t.Fatalf("successful save identity=%q title=%q clean=%v", s.Filename(), s.Title(), s.IsClean())
	}
	if _, err := core.LoadTDocFile(s.Filename()); err != nil {
		t.Fatal(err)
	}
}

func TestDesignSaveExtensionFailureDoesNotRetargetExistingIdentity(t *testing.T) {
	s := NewGedScene()
	path := filepath.Join(t.TempDir(), "no-parent", "legacy-name")
	s.SetFilename(path)
	s.SetTitle("current title")
	s.PushCommand(gui.NewCompoundCommand())
	title := s.Title()
	if s.Save() || s.Filename() != path || s.Title() != title || s.IsClean() {
		t.Fatal("failed extensionless Save retargeted or marked scene saved")
	}
}

func TestDesignSaveToLegacyExtensionAndFailedSaveAs(t *testing.T) {
	s := NewGedScene()
	path := filepath.Join(t.TempDir(), "legacy.cml")
	if err := s.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	s.PushCommand(gui.NewCompoundCommand())
	before, _ := os.ReadFile(path)
	title := s.Title()
	target := filepath.Join(t.TempDir(), "protected.silkui")
	if err := os.WriteFile(target, []byte("protected"), 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(target, 0600) })
	if err := s.SaveTo(target); err == nil || s.Filename() != path || s.Title() != title || s.IsClean() {
		t.Fatal("failed Save As mutated original identity/history")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed Save As changed original design")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "protected" {
		t.Fatal("failed Save As damaged destination")
	}
}

func TestDesignSaveToCommitsDraftBeforeWritingAndKeepsUndo(t *testing.T) {
	v, panel, widget := draftPanel(t)
	s := v.GedScene()
	panel.HandlerEditor().ReplaceAllText("func onButtonClick(){println(\"save😀\")}")
	path := filepath.Join(t.TempDir(), "draft.silkui")
	if err := s.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if !s.IsClean() || panel.HasPendingChanges() || !strings.Contains(widget.GetCode(), "save😀") {
		t.Fatal("saved wrong draft or clean marker")
	}
	v.Undo()
	if s.IsClean() || widget.GetCode() != "" {
		t.Fatal("SaveTo destroyed draft Undo history")
	}
	v.Redo()
	if !s.IsClean() || !strings.Contains(widget.GetCode(), "save😀") {
		t.Fatal("Redo did not restore the saved state")
	}
}

type saveCallbackDraft struct {
	pending bool
	commit  func() error
}

func (d *saveCallbackDraft) HasPendingChanges() bool { return d.pending }
func (d *saveCallbackDraft) CommitPendingChanges() error {
	d.pending = false
	return d.commit()
}

func TestDesignSaveToRejectsRetargetAndNestedSaveDuringCommit(t *testing.T) {
	for _, nested := range []bool{false, true} {
		name := "retarget"
		if nested {
			name = "nested-save"
		}
		t.Run(name, func(t *testing.T) {
			s := NewGedScene()
			path := filepath.Join(t.TempDir(), "request.silkui")
			next := path + ".new"
			d := &saveCallbackDraft{pending: true}
			d.commit = func() error {
				if nested {
					return s.SaveTo(next)
				}
				s.SetFilename(next)
				return nil
			}
			s.AddPendingEditor(d)
			if err := s.SaveTo(path); err == nil {
				t.Fatal("obsolete/reentrant save accepted")
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("obsolete request wrote a file: %v", err)
			}
			if nested && s.Filename() != "" || !nested && s.Filename() != next {
				t.Fatal("save request overwrote callback's identity")
			}
			if err := s.SaveTo(path); err != nil {
				t.Fatalf("failed save left reentry guard latched: %v", err)
			}
		})
	}
}

func TestDesignAutosaveFailurePreservesRecoveryAndDirtyState(t *testing.T) {
	s := NewGedScene()
	path := filepath.Join(t.TempDir(), "design.silkui")
	s.SetFilename(path)
	s.PushCommand(gui.NewCompoundCommand())
	if err := os.WriteFile(path+".autosave", []byte("previous recovery"), 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path+".autosave", 0600) })
	a := NewAutoSaver()
	a.scene = s
	a.tick()
	data, err := os.ReadFile(path + ".autosave")
	if err != nil || string(data) != "previous recovery" || !a.lastSave.IsZero() || s.IsClean() {
		t.Fatalf("failed autosave damaged recovery/clean marker: %q %v", data, err)
	}
}

func TestDesignSaveReentryNeverOpensUnnamedDialog(t *testing.T) {
	s := NewGedScene()
	d := &saveCallbackDraft{pending: true}
	d.commit = func() error {
		if s.Save() {
			t.Fatal("nested unnamed Save accepted")
		}
		return nil
	}
	s.AddPendingEditor(d)
	if err := s.SaveTo(filepath.Join(t.TempDir(), "outer.silkui")); err != nil {
		t.Fatal(err)
	}
}

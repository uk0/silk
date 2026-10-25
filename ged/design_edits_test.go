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

func draftPanel(t *testing.T) (*GedView, *CodePanel, *FakeWidget) {
	t.Helper()
	v := NewGedView()
	w := addFakeTo(t, v.GedScene(), "gui.Button", "button")
	p := NewCodePanel()
	p.BindGedView(v)
	p.SetWidget(w)
	t.Cleanup(func() { p.DiscardPendingChanges(); p.BindGedView(nil) })
	v.GedScene().UndoStack().SetClean()
	return v, p, w
}

func TestDesignDraftSavedAndRecoveredWithoutPanelSave(t *testing.T) {
	v, p, w := draftPanel(t)
	s := v.GedScene()
	path := filepath.Join(t.TempDir(), "draft.silkui")
	s.SetFilename(path)
	if !s.IsClean() || p.HasPendingChanges() || w.GetCode() != "" {
		t.Fatal("untouched template dirtied design")
	}
	draft := "func onButtonClick(){println(\"保留😀草稿\")}"
	p.HandlerEditor().ReplaceAllText(draft)
	if s.IsClean() || !strings.HasSuffix(s.Title(), " *") || len(s.DirtyList()) != 1 {
		t.Fatal("draft missing from document dirty state")
	}
	a := NewAutoSaver()
	a.scene = s
	a.tick()
	doc, err := core.LoadTDocFile(path + ".autosave")
	if err != nil {
		t.Fatal(err)
	}
	recovered := NewGedScene()
	if err := recovered.LoadDesign(doc); err != nil {
		t.Fatal(err)
	}
	if recovered.Children()[0].(*FakeWidget).GetCode() != draft || s.IsClean() {
		t.Fatal("recovery lost draft or marked it saved")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("autosave overwrote original")
	}
	p.HandlerEditor().ReplaceAllText(draft + "\n// newer")
	if !s.Save() || !s.IsClean() {
		t.Fatal("save did not flush pending source")
	}
	saved, err := core.LoadTDocFile(path)
	if err != nil {
		t.Fatal(err)
	}
	check := NewGedScene()
	_ = check.LoadDesign(saved)
	if check.Children()[0].(*FakeWidget).GetCode() != draft+"\n// newer" {
		t.Fatal("disk snapshot is stale")
	}
}

func TestDesignDraftCommitsBeforeMutationAndUndo(t *testing.T) {
	v, p, w := draftPanel(t)
	s := v.GedScene()
	draft := "func onButtonClick(){println(42)}"
	p.HandlerEditor().ReplaceAllText(draft)
	s.PushCommand(gui.NewCompoundCommand())
	if w.GetCode() != draft || p.HasPendingChanges() || s.UndoStack().Count() != 2 {
		t.Fatal("mutation did not flush one undoable draft")
	}
	v.Undo()
	if w.GetCode() != draft {
		t.Fatal("undo of next operation lost draft")
	}
	v.Undo()
	if w.GetCode() != "" {
		t.Fatal("draft not separately undoable")
	}
	v.Redo()
	if w.GetCode() != draft {
		t.Fatal("redo did not restore draft")
	}
	p.HandlerEditor().ReplaceAllText(draft + "\n// latest")
	v.Undo()
	if w.GetCode() != draft || p.HasPendingChanges() {
		t.Fatal("canvas undo ignored active draft")
	}
}

type failingDraft struct{ pending bool }

func (d *failingDraft) HasPendingChanges() bool     { return d.pending }
func (d *failingDraft) CommitPendingChanges() error { return errors.New("blocked draft") }
func (d *failingDraft) DiscardPendingChanges()      { d.pending = false }

func TestDesignDraftFailureNeverWritesOrLosesBuffer(t *testing.T) {
	v, p, w := draftPanel(t)
	s := v.GedScene()
	path := filepath.Join(t.TempDir(), "draft.silkui")
	s.SetFilename(path)
	if !s.Save() {
		t.Fatal("baseline save")
	}
	before, _ := os.ReadFile(path)
	p.HandlerEditor().ReplaceAllText("func onButtonClick(){println(42)}")
	w.SetCode("func onButtonClick(){println(7)}") // external model change
	if s.Save() || !p.HasPendingChanges() {
		t.Fatal("conflicting code silently overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("failed save changed disk")
	}
	p.DiscardPendingChanges()
	if p.HandlerEditor().Text() != w.GetCode() {
		t.Fatal("explicit discard did not refresh model")
	}
	id := s.AddPendingEditor(&failingDraft{true})
	defer s.RemovePendingEditor(id)
	out := filepath.Join(t.TempDir(), "export.go")
	if _, err := s.GenerateCodeFile(out, CodeGenOptions{}); err == nil {
		t.Fatal("export ignored draft failure")
	}
	if _, err := os.Stat(strings.TrimSuffix(out, ".go") + ".silk.go"); !os.IsNotExist(err) {
		t.Fatal("failed flush wrote machine file")
	}
}

func TestDesignDraftCloseAndDetachRetireSubscriptions(t *testing.T) {
	v, p, _ := draftPanel(t)
	s := v.GedScene()
	p.HandlerEditor().ReplaceAllText("func onButtonClick(){println(42)}")
	v.Close() // the dock's already-confirmed discard transition
	if len(s.pendingEditors) != 0 || len(s.designChangedList) != 0 || len(v.selectionListeners) != 0 || p.currentWidget != nil {
		t.Fatal("closed document retained panel subscriptions")
	}
}

func TestDesignCopyIncludesPendingHandlerDraft(t *testing.T) {
	oldClipboard := clipboard
	t.Cleanup(func() { clipboard = oldClipboard })
	v, p, w := draftPanel(t)
	v.Selection().Add(w)
	p.HandlerEditor().ReplaceAllText("func onButtonClick(){println(99)}")
	v.CopySelected()
	if len(clipboard) != 1 || !strings.Contains(clipboard[0].String(), "println(99)") || p.HasPendingChanges() {
		t.Fatal("copy omitted pending handler source")
	}
}

func TestHasPendingEditsDoesNotCommitDraft(t *testing.T) {
	v, p, w := draftPanel(t)
	s := v.GedScene()
	count := s.UndoStack().Count()
	if s.HasPendingEdits() {
		t.Fatal("template is not a draft")
	}
	p.HandlerEditor().ReplaceAllText("func pending(){}")
	for i := 0; i < 10; i++ {
		if !s.HasPendingEdits() {
			t.Fatal("missing draft")
		}
	}
	if w.GetCode() != "" || s.UndoStack().Count() != count {
		t.Fatal("query committed draft")
	}
	if err := s.CommitPendingEdits(); err != nil {
		t.Fatal(err)
	}
	if s.HasPendingEdits() {
		t.Fatal("committed draft remains pending")
	}
}

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func editorPointerPoint(t *testing.T, b *gui.TabBar, index int, close bool) (float64, float64) {
	t.Helper()
	y := b.Height() / 2
	for x := 1.0; x < b.Width(); x++ {
		if i, c := b.HitTest(x, y); i == index && c == close {
			return x, y
		}
	}
	t.Fatal("missing pointer target", index, close)
	return 0, 0
}

func TestEditorPointerTabFocusAndImmediateTitle(t *testing.T) {
	w := testWorkbench(t)
	a := writeFile(t, t.TempDir(), "a.txt", "alpha")
	b := writeFile(t, t.TempDir(), "b.txt", "beta")
	openFileInEditor(w.tabs, a)
	first := activeEditor(w.tabs)
	openFileInEditor(w.tabs, b)
	w.activate(modeEdit)
	w.tabs.SetSize(900, 300)
	w.tabs.Layout()
	bar := w.tabs.TabBar()
	field := gui.NewEdit()
	field.SetParent(w.frame)
	field.SetFocus()
	w.tabs.SetCurrentIndex(0)
	if !field.HasFocus() {
		t.Fatal("programmatic selection stole field focus")
	}
	if !strings.HasPrefix(w.frame.Title(), "a.txt — ") {
		t.Fatal("title waited for timer", w.frame.Title())
	}
	bar.SetFocus()
	bar.OnKeyDown(gui.KeyRight, false)
	if !bar.HasFocus() || !strings.HasPrefix(w.frame.Title(), "b.txt — ") {
		t.Fatal("keyboard strip behavior or title")
	}
	x, y := editorPointerPoint(t, bar, 0, false)
	bar.OnLeftDown(x, y)
	bar.OnLeftUp(x, y)
	if !first.HasFocus() || activeEditor(w.tabs) != first {
		t.Fatal("pointer selection not ready for typing")
	}
	// Clicking the already-active tab also returns focus from the strip.
	bar.OnLeftDown(x, y)
	bar.OnLeftUp(x, y)
	if !first.HasFocus() {
		t.Fatal("same-tab focus")
	}
	bar.OnLeftDown(x, y)
	field.SetFocus()
	bar.OnLeftUp(x, y)
	if !field.HasFocus() {
		t.Fatal("late pointer release stole newly acquired focus")
	}
	bar.SetFocus()
	w.activate(modeWelcome)
	focusEditorAfterPointerTab(w.tabs, bar)
	if first.HasFocus() {
		t.Fatal("hidden editor focused")
	}
}

func TestEditorPointerCloseFocusesRemainingDocument(t *testing.T) {
	w := testWorkbench(t)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "a.txt", "alpha"))
	first := activeEditor(w.tabs)
	openFileInEditor(w.tabs, writeFile(t, t.TempDir(), "b.txt", "beta"))
	w.activate(modeEdit)
	w.tabs.SetSize(900, 300)
	w.tabs.Layout()
	bar := w.tabs.TabBar()
	x, y := editorPointerPoint(t, bar, 1, true)
	bar.OnLeftDown(x, y)
	bar.OnLeftUp(x, y)
	if w.tabs.Count() != 1 || !first.HasFocus() || !strings.HasPrefix(w.frame.Title(), "a.txt — ") {
		t.Fatal("close left stale focus/title")
	}
}

func TestEditorToolbarMouseSaveKeepsTypingAndMenuTarget(t *testing.T) {
	w := testWorkbench(t)
	path := writeFile(t, t.TempDir(), "target.txt", "alpha")
	openFileInEditor(w.tabs, path)
	w.activate(modeEdit)
	ed := activeEditor(w.tabs)
	buildToolBar(w.frame, w.tabs, w.canvas)
	bar := w.frame.ToolBar()
	if !bar.PreserveContentFocus() {
		t.Fatal("IDE toolbar did not opt in")
	}
	var save *gui.Button
	for _, item := range bar.Items() {
		if b, ok := item.(*gui.Button); ok && b.Icon() == paint.LoadIcon("save") {
			save = b
			break
		}
	}
	if save == nil {
		t.Fatal("save missing")
	}
	ed.ReplaceAllText("changed\r\n中🙂")
	ed.SetFocus()
	save.OnLeftDown(1, 1)
	save.Action().Trigger(nil)
	if !ed.HasFocus() {
		t.Fatal("mouse save stole editor focus")
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "changed\r\n中🙂" {
		t.Fatal("save mismatch", err)
	}
	m := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	for _, id := range []string{"editor.selectAll", "navigate.line", "editor.lineEndings", "editor.cleanup"} {
		b := menuCommandButton(m, id)
		if b == nil || !b.IsEnabled() {
			t.Fatal("menu unavailable after mouse save", id)
		}
	}
	menuCommandButton(m, "editor.selectAll").Action().Trigger(nil)
	if !ed.HasSelection() {
		t.Fatal("menu selected no text")
	}
	field := gui.NewEdit()
	field.SetParent(w.frame)
	field.SetFocus()
	save.OnLeftDown(1, 1)
	save.Action().Trigger(nil)
	if !field.HasFocus() {
		t.Fatal("toolbar stole a non-document field")
	}
	blocked := buildWorkbenchSubMenu("Edit", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(blocked)
	if menuCommandButton(blocked, "editor.selectAll").IsEnabled() {
		t.Fatal("menu targeted document behind focused field")
	}
}

func TestEditorPointerTitleSupportsUntrackedPageAndDirtyState(t *testing.T) {
	w := testWorkbench(t)
	ed := gui.NewCodeEditor()
	w.tabs.AddTab(ed, "Scratch", nil)
	w.activate(modeEdit)
	syncDocumentWindowTitle(w.frame, w.canvas)
	if !strings.HasPrefix(w.frame.Title(), "Scratch — ") {
		t.Fatal("untracked page title", w.frame.Title())
	}
	ed.ReplaceAllText("draft")
	syncDocumentWindowTitle(w.frame, w.canvas)
	if !strings.HasPrefix(w.frame.Title(), "Scratch * — ") {
		t.Fatal("dirty page title", w.frame.Title())
	}
}

func TestPreservingToolbarFocusStillCommitsDesignDraft(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeDesign)
	scene := w.canvas.GedScene()
	draft := &historyPendingDraft{scene: scene, pending: true}
	id := scene.AddPendingEditor(draft)
	defer scene.RemovePendingEditor(id)
	field := gui.NewEdit()
	field.SetParent(w.frame)
	field.SetFocus()
	bar := gui.NewToolBar()
	bar.SetParent(w.frame)
	bar.SetPreserveContentFocus(true)
	button := bar.AddAction("Save", nil, func() {
		if !saveCurrentDocument(w.tabs, w.canvas) {
			t.Error("save failed")
		}
	})
	button.OnLeftDown(1, 1)
	button.Action().Trigger(nil)
	if draft.pending || !field.HasFocus() {
		t.Fatal("save relied on losing focus to commit draft")
	}
	if _, err := os.Stat(scene.Filename()); err != nil {
		t.Fatal("design was not written", err)
	}
}

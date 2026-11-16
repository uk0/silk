package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDesignSaveCompletionSynchronizesWindowTitle(t *testing.T) {
	w, _, posts := testOwnedDesignSaver(t)
	w.activate(modeDesign)
	s := w.canvas.GedScene()
	s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "saved design"})
	syncDocumentWindowTitle(w.frame, w.canvas)
	if !strings.Contains(w.frame.Title(), " *") {
		t.Fatal("fixture not dirty")
	}
	completed := false
	if !requestDesignDocumentSave(w.canvas, func(ok bool) {
		if !ok {
			t.Error("not adopted")
		}
		completed = true
	}) {
		t.Fatal("save admission")
	}
	pumpOwnedSaves(t, posts, func() bool { return completed })
	if !s.IsClean() || strings.Contains(w.frame.Title(), " *") || !strings.HasPrefix(w.frame.Title(), filepath.Base(s.Filename())) {
		t.Fatal("saved design left stale native window title", w.frame.Title())
	}
}

func TestDesignOpenCompletionSynchronizesWindowTitle(t *testing.T) {
	w := testWorkbench(t)
	w.activate(modeDesign)
	syncDocumentWindowTitle(w.frame, w.canvas)
	p := designOpenPath(t, "new title")
	if !requestDesignOpen(p, w.tabs, w.canvas, false) {
		t.Fatal("open admission")
	}
	pumpTestDesignOpens(t, w)
	if !strings.HasPrefix(w.frame.Title(), filepath.Base(p)) || strings.Contains(w.frame.Title(), " *") {
		t.Fatal("opened design left old native window title", w.frame.Title())
	}
}

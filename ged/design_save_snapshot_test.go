package ged

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func TestNewDesignSceneTitleAndCleanBeforeHistory(t *testing.T) {
	s := NewGedScene()
	if !s.IsClean() || s.Title() != "untitled" || len(s.DirtyList()) != 0 {
		t.Fatal("fresh scene has a dirty identity before any history")
	}
}

func TestPreparedDesignSaveIndependentWriterAndOwnedCompletion(t *testing.T) {
	v, panel, widget := draftPanel(t)
	s := v.GedScene()
	panel.HandlerEditor().ReplaceAllText("func onButtonClick(){println(\"captured😀\")}")
	path := filepath.Join(t.TempDir(), "new-design")
	p, err := s.PrepareSave(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Filename() != "" || s.IsClean() || panel.HasPendingChanges() || s.CompleteSave(p) {
		t.Fatal("prepare pretended to save or complete without physical write")
	}
	if !s.PreparedSaveCurrent(p) {
		t.Fatal("unwritten receipt must still identify current capture")
	}
	writer := p.Writer() // only this independent object goes to the worker
	done := make(chan error, 1)
	go func() { done <- writer.Write() }()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if s.Filename() != "" || s.IsClean() {
		t.Fatal("worker touched scene identity/clean marker")
	}
	if !s.CompleteSave(p) || !s.IsClean() || s.Filename() != path+".silkui" || s.CompleteSave(p) {
		t.Fatal("owned completion did not adopt exactly once")
	}
	if s.PreparedSaveCurrent(p) {
		t.Fatal("consumed receipt still current")
	}
	v.Undo()
	if s.IsClean() || widget.GetCode() != "" {
		t.Fatal("completion cleared Undo history")
	}
	v.Redo()
	if !s.IsClean() {
		t.Fatal("Redo to saved state is not clean")
	}
}

func TestPreparedDesignSaveRejectsLateHistoryAndIdentityChanges(t *testing.T) {
	for _, action := range []string{"edit", "undo-redo", "branch", "direct-history", "load", "path-aba", "newer-prepare", "failed-newer-prepare", "draft", "other-scene"} {
		t.Run(action, func(t *testing.T) {
			v, panel, _ := draftPanel(t)
			s := v.GedScene()
			s.PushCommand(gui.NewCompoundCommand())
			p, err := s.PrepareSave(filepath.Join(t.TempDir(), "saved.silkui"))
			if err != nil {
				t.Fatal(err)
			}
			switch action {
			case "edit":
				s.PushCommand(gui.NewCompoundCommand())
			case "undo-redo":
				v.Undo()
				v.Redo()
			case "branch":
				v.Undo()
				s.PushCommand(gui.NewCompoundCommand())
			case "direct-history":
				s.UndoStack().Clear()
			case "load":
				if err := s.LoadDesign(s.SaveDesign()); err != nil {
					t.Fatal(err)
				}
			case "path-aba":
				old := s.Filename()
				s.SetFilename("other")
				s.SetFilename(old)
			case "newer-prepare":
				if _, err := s.PrepareSave(filepath.Join(t.TempDir(), "newer")); err != nil {
					t.Fatal(err)
				}
			case "failed-newer-prepare":
				if _, err := s.PrepareSave(""); err == nil {
					t.Fatal("empty target accepted")
				}
			case "draft":
				panel.HandlerEditor().ReplaceAllText("func onButtonClick(){println(42)}")
			case "other-scene":
				s = NewGedScene()
			}
			name, title := s.Filename(), s.Title()
			if s.PreparedSaveCurrent(p) {
				t.Fatal("capture guard accepted changed history/identity")
			}
			if err := p.Writer().Write(); err != nil {
				t.Fatal(err)
			}
			if s.CompleteSave(p) || s.Filename() != name || s.Title() != title {
				t.Fatal("stale completion adopted a filename, clean marker or title")
			}
			if _, err := os.Stat(p.Writer().Path()); err != nil {
				t.Fatal("physical success was not preserved independently", err)
			}
		})
	}
}

func TestDesignWriteSnapshotDocumentCopyDoesNotExposeEncoderTree(t *testing.T) {
	s := NewGedScene()
	s.SetFormTitle("Original document")
	p, err := s.PrepareSave(filepath.Join(t.TempDir(), "saved.silkui"))
	if err != nil {
		t.Fatal(err)
	}
	w := p.Writer()
	before := w.DocumentCopy().String()
	copy := w.DocumentCopy()
	copy.Clear()
	if w.DocumentCopy().String() != before {
		t.Fatal("companion consumer changed encoder tree")
	}
	if err := w.Write(); err != nil {
		t.Fatal(err)
	}
	doc, err := core.LoadTDocFile(w.Path())
	if err != nil || doc.String() != before {
		t.Fatal("copy mutation changed physical primary", err)
	}
}

func TestPreparedDesignSaveRemainsUIResponsiveWhilePhysicalWriterBlocked(t *testing.T) {
	s := NewGedScene()
	s.PushCommand(gui.NewCompoundCommand())
	p, err := s.PrepareSave(filepath.Join(t.TempDir(), "blocked.silkui"))
	if err != nil {
		t.Fatal(err)
	}
	w := p.Writer()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	go func() {
		done <- w.writeWith(func(doc *core.TDoc, path string) error { close(entered); <-release; return doc.SaveFileAtomic(path) })
	}()
	<-entered
	if s.CompleteSave(p) {
		t.Fatal("in-flight write was considered complete")
	}
	// This mutation is UI-owned, while the worker retains only the captured doc.
	s.SetFormTitle("newer UI input")
	s.NotifyDesignChanged()
	if s.FormTitle() != "newer UI input" {
		t.Fatal("UI input unavailable during I/O")
	}
	unblock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not return")
	}
	if s.CompleteSave(p) || s.IsClean() || s.Filename() != "" {
		t.Fatal("late write cleaned newer input")
	}
	doc, err := core.LoadTDocFile(w.Path())
	if err != nil {
		t.Fatal(err)
	}
	var title string
	doc.ReadAttr("title", &title)
	if title != "Form" {
		t.Fatal("worker traversed newer UI state instead of independent snapshot", title)
	}
}

func TestPreparedDesignWriterOnceOnlyAndFailedRetryIsNotSuccess(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			s := NewGedScene()
			s.PushCommand(gui.NewCompoundCommand())
			p, err := s.PrepareSave(filepath.Join(t.TempDir(), "saved"))
			if err != nil {
				t.Fatal(err)
			}
			w := p.Writer()
			var calls atomic.Int32
			cause := errors.New("injected write failure")
			var wg sync.WaitGroup
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					err := w.writeWith(func(doc *core.TDoc, path string) error {
						calls.Add(1)
						if fail {
							return cause
						}
						return doc.SaveFileAtomic(path)
					})
					if fail && !errors.Is(err, cause) || !fail && err != nil {
						t.Errorf("write error=%v", err)
					}
				}()
			}
			wg.Wait()
			if calls.Load() != 1 || s.CompleteSave(p) == fail {
				t.Fatal("duplicate physical write or false success", calls.Load())
			}
		})
	}
}

func TestPreparedDesignWriterPanicDoesNotBecomeCachedSuccess(t *testing.T) {
	s := NewGedScene()
	p, err := s.PrepareSave(filepath.Join(t.TempDir(), "saved"))
	if err != nil {
		t.Fatal(err)
	}
	w := p.Writer()
	if err := w.writeWith(func(*core.TDoc, string) error { panic("writer panic") }); err == nil {
		t.Fatal("panic lost")
	}
	if err := w.Write(); err == nil || s.CompleteSave(p) {
		t.Fatal("panicked once-only write became successful on retry")
	}
}

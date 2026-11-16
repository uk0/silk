package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func installTestSaveChooser(t *testing.T, c *ownedDesignSaver, path string) {
	t.Helper()
	choose, after, ask := chooseDesignSavePath, afterDesignSaveChooser, askReplaceDesignFile
	chooseDesignSavePath = func() string { return path }
	afterDesignSaveChooser = c.settings.post
	askReplaceDesignFile = func(*ged.GedView, string) gui.DialogResult {
		t.Error("unexpected overwrite question")
		return gui.DialogCancel
	}
	t.Cleanup(func() { chooseDesignSavePath, afterDesignSaveChooser, askReplaceDesignFile = choose, after, ask })
}

func TestDesignSaveAsReviewsExactObservedTarget(t *testing.T) {
	for _, action := range []string{"absent", "replace", "cancel", "conflict", "edit", "undo-redo", "owner-close", "readonly"} {
		t.Run(action, func(t *testing.T) {
			w, c, posts := testOwnedDesignSaver(t)
			s := w.canvas.GedScene()
			s.SetFilename("")
			s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "ours"})
			path := filepath.Join(t.TempDir(), "saved")
			output := path + ".silkui"
			if action != "absent" {
				if err := os.WriteFile(output, []byte("existing"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if action == "readonly" {
				if err := os.Chmod(output, 0444); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(output, 0644) })
			}
			installTestSaveChooser(t, c, path)
			asks, calls := 0, 0
			askReplaceDesignFile = func(canvas *ged.GedView, p string) gui.DialogResult {
				asks++
				if canvas != w.canvas || p != output {
					t.Error("wrong overwrite owner/path", p)
				}
				switch action {
				case "cancel":
					return gui.DialogCancel
				case "conflict":
					if err := os.WriteFile(output, []byte("external after review"), 0644); err != nil {
						t.Error(err)
					}
				case "edit":
					s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "newer"})
				case "undo-redo":
					s.UndoStack().Undo()
					s.UndoStack().Redo()
				case "owner-close":
					c.Close()
				}
				return gui.DialogOK
			}
			var saved bool
			if !requestDesignDocumentSave(w.canvas, func(ok bool) { calls++; saved = ok }) {
				t.Fatal("chooser not admitted")
			}
			pumpOwnedSaves(t, posts, func() bool { return calls != 0 })
			if calls != 1 || anySaveBusy() || c.review != nil {
				t.Fatal("completion or reservation leak", calls)
			}
			want := action == "absent" || action == "replace"
			if saved != want || (asks == 0) != (action == "absent") {
				t.Fatal("wrong overwrite decision", saved, asks)
			}
			if want {
				doc, err := core.LoadTDocFile(output)
				if err != nil || !strings.Contains(doc.String(), "ours") || !s.IsClean() || s.Filename() != output {
					t.Fatal("actual save not adopted", err)
				}
			} else {
				before := "existing"
				if action == "conflict" {
					before = "external after review"
				}
				if diskText(t, output) != before || s.Filename() != "" || s.IsClean() {
					t.Fatal("rejected review overwrote data or adopted identity")
				}
			}
		})
	}
}

func TestDesignSaveAsInspectionIsOwnedAndDoesNotBlockUI(t *testing.T) {
	for _, retire := range []bool{false, true} {
		t.Run(fmt.Sprint(retire), func(t *testing.T) {
			w, c, posts := testOwnedDesignSaver(t)
			s := w.canvas.GedScene()
			s.SetFilename("")
			s.PushCommand(&ownedSaveTitleCommand{s, s.FormTitle(), "ours"})
			path := filepath.Join(t.TempDir(), "saved.silkui")
			installTestSaveChooser(t, c, path)
			entered, release := make(chan struct{}), make(chan struct{})
			inspect := c.settings.inspect
			c.settings.inspect = func(ctx context.Context, p string) (core.FileFingerprint, error) {
				close(entered)
				<-release
				return inspect(ctx, p)
			}
			released := false
			unblock := func() {
				if !released {
					close(release)
					released = true
				}
			}
			t.Cleanup(unblock)
			calls := 0
			if !requestDesignDocumentSave(w.canvas, func(ok bool) {
				calls++
				if retire && ok {
					t.Error("retired save adopted")
				}
			}) {
				t.Fatal("admission")
			}
			select {
			case f := <-posts:
				f()
			case <-time.After(5 * time.Second):
				t.Fatal("no chooser cleanup")
			}
			<-entered
			if calls != 0 || c.review == nil || !savePathBusy(path) {
				t.Fatal("inspection pretended completed")
			}
			box := c.review.box
			if retire {
				c.Close()
				if calls != 1 || box.owner.Load() != nil || !savePathBusy(path) {
					t.Fatal("retirement released blocked inspection or retained UI")
				}
			}
			unblock()
			if !retire {
				pumpOwnedSaves(t, posts, func() bool { return calls != 0 })
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := c.settings.queue.Wait(ctx); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("retired inspector dispatched a write", err)
				}
			}
			if calls != 1 || anySaveBusy() {
				t.Fatal("late callback or reservation leak", calls)
			}
		})
	}
}

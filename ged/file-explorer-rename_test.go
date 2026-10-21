package ged

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/diagnostic"
)

func finishRenameUI(t *testing.T, queue <-chan func(), e *FileExplorer) {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for e.RenameBusy() {
		select {
		case fn := <-queue:
			fn()
		case <-timeout:
			t.Fatal("rename completion not delivered")
		}
	}
}
func TestExplorerRenameAsyncReservationAndState(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old")
	next := filepath.Join(root, "next")
	if err := os.MkdirAll(filepath.Join(old, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(old, "nested", "中文.go")
	if err := os.WriteFile(child, []byte("package p"), 0644); err != nil {
		t.Fatal(err)
	}
	e := NewFileExplorer()
	e.rootDir = root
	t.Cleanup(e.CancelScan)
	q := make(chan func(), 16)
	e.post = func(fn func()) { q <- fn }
	selected := &fileEntry{path: child, name: "中文.go"}
	e.root = &fileEntry{path: root, isDir: true, children: []*fileEntry{selected}}
	e.flatList = []*fileEntry{selected}
	e.selectedIdx = 0
	e.selectionPath = child
	e.expanded = map[string]bool{old: true, filepath.Join(old, "nested"): true}
	e.searchCollapsed = map[string]bool{old + "sibling": true}
	entered, release := make(chan struct{}), make(chan struct{})
	e.renameIO = func(p, n string) (string, error) { close(entered); <-release; return fsRename(p, n) }
	prepared, completed, notified := false, false, false
	e.SetRenameCoordinator(func(a, b string) (func(error), error) {
		if a != old || b != next {
			t.Fatalf("prepare paths %s %s", a, b)
		}
		if _, err := os.Stat(old); err != nil {
			t.Fatal("prepare occurred after disk mutation")
		}
		prepared = true
		return func(err error) {
			if err != nil {
				t.Error(err)
			}
			completed = true
		}, nil
	})
	if err := e.RenamePath(old, "next", func(err error) {
		if err != nil {
			t.Error(err)
		}
		if e.RenameBusy() {
			t.Error("busy not released before done")
		}
		notified = true
	}); err != nil {
		t.Fatal(err)
	}
	if !prepared || completed || !e.RenameBusy() {
		t.Fatal("reservation lifecycle")
	}
	<-entered
	if err := e.RenamePath(old, "other", nil); diagnostic.CodeOf(err) != diagnostic.FileOperationBusy {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("blocked worker mutated source")
	}
	close(release)
	finishRenameUI(t, q, e)
	if !completed || !notified {
		t.Fatal("missing UI completion")
	}
	if got := e.SelectedPath(); got != filepath.Join(next, "nested", "中文.go") {
		t.Fatal("selection", got)
	}
	if !e.expanded[next] || !e.expanded[filepath.Join(next, "nested")] || e.expanded[old] || !e.searchCollapsed[old+"sibling"] {
		t.Fatal("expansion migration", e.expanded)
	}
	if _, err := os.Stat(child); !os.IsNotExist(err) {
		t.Fatal("old source survived", err)
	}
	if b, err := os.ReadFile(filepath.Join(next, "nested", "中文.go")); err != nil || string(b) != "package p" {
		t.Fatal("disk", string(b), err)
	}
}
func TestExplorerRenameRejectAndFailureDoNotCommit(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old")
	next := filepath.Join(root, "next")
	for _, p := range []string{old, next} {
		if err := os.WriteFile(p, []byte(p), 0644); err != nil {
			t.Fatal(err)
		}
	}
	e := NewFileExplorer()
	e.rootDir = root
	t.Cleanup(e.CancelScan)
	q := make(chan func(), 16)
	e.post = func(fn func()) { q <- fn }
	for _, path := range []string{root, root + "sibling", filepath.Dir(root)} {
		if err := e.RenamePath(path, "new", nil); diagnostic.CodeOf(err) != diagnostic.FileName {
			t.Fatal(path, err)
		}
	}
	refused := errors.New("host refused")
	e.SetRenameCoordinator(func(string, string) (func(error), error) { return nil, refused })
	if err := e.RenamePath(old, "new", nil); !errors.Is(err, refused) || e.RenameBusy() {
		t.Fatal("rejection", err)
	}
	count := 0
	e.SetRenameCoordinator(func(string, string) (func(error), error) {
		return func(err error) {
			count++
			if diagnostic.CodeOf(err) != diagnostic.FileConflict {
				t.Error(err)
			}
		}, nil
	})
	if err := e.RenamePath(old, "next", nil); err != nil {
		t.Fatal(err)
	}
	finishRenameUI(t, q, e)
	if count != 1 {
		t.Fatal("completion count", count)
	}
	for _, p := range []string{old, next} {
		if b, err := os.ReadFile(p); err != nil || string(b) != p {
			t.Fatal("contents changed", p, err)
		}
	}
}
func TestExplorerRenameCompletionSurvivesRootRetirement(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old")
	if err := os.WriteFile(old, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	e := NewFileExplorer()
	e.rootDir = root
	t.Cleanup(e.CancelScan)
	q := make(chan func(), 16)
	e.post = func(fn func()) { q <- fn }
	completed := false
	e.SetRenameCoordinator(func(string, string) (func(error), error) {
		return func(err error) {
			if err != nil {
				t.Error(err)
			}
			completed = true
		}, nil
	})
	if err := e.RenamePath(old, "next", nil); err != nil {
		t.Fatal(err)
	}
	e.CancelScan()
	e.rootDir = t.TempDir()
	e.selectionPath = "unchanged"
	finishRenameUI(t, q, e)
	if !completed || e.selectionPath != "unchanged" {
		t.Fatal("host completion lost or new root state changed")
	}
}

func TestExplorerRenamePreparationWaitsOffUIAndTimeoutNeverMutates(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			root := t.TempDir()
			old := filepath.Join(root, "old")
			next := filepath.Join(root, "next")
			if err := os.WriteFile(old, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			e := NewFileExplorer()
			e.rootDir = root
			t.Cleanup(e.CancelScan)
			q := make(chan func(), 16)
			e.post = func(fn func()) { q <- fn }
			entered, release := make(chan struct{}), make(chan struct{})
			var diskCalls atomic.Int32
			e.renameIO = func(path, name string) (string, error) { diskCalls.Add(1); return fsRename(path, name) }
			complete, notify := 0, 0
			e.renameWaitTimeout = 3 * time.Second
			if fail {
				e.renameWaitTimeout = 20 * time.Millisecond
			}
			e.SetRenamePreparation(func(string, string) (RenamePreparation, error) {
				return RenamePreparation{
					Wait: func(ctx context.Context) error {
						close(entered)
						if fail {
							<-ctx.Done()
							return ctx.Err()
						}
						select {
						case <-release:
							return nil
						case <-ctx.Done():
							return ctx.Err()
						}
					},
					Complete: func(err error) {
						complete++
						if fail {
							if diagnostic.CodeOf(err) != diagnostic.FileOperationWait || !errors.Is(err, context.DeadlineExceeded) {
								t.Error("lost code/cause", err)
							}
						} else if err != nil {
							t.Error(err)
						}
					},
				}, nil
			})
			if err := e.RenamePath(old, "next", func(err error) { notify++ }); err != nil {
				t.Fatal(err)
			}
			<-entered
			if !e.RenameBusy() || diskCalls.Load() != 0 || complete != 0 {
				t.Fatal("wait was skipped or ran completion early")
			}
			// Root retirement cannot drop the completion of an accepted transaction.
			e.rootDir = t.TempDir()
			close(release)
			finishRenameUI(t, q, e)
			if complete != 1 || notify != 1 {
				t.Fatal("completion not exactly once", complete, notify)
			}
			if fail {
				if diskCalls.Load() != 0 {
					t.Fatal("filesystem touched after wait failure")
				}
				if b, err := os.ReadFile(old); err != nil || string(b) != "unchanged" {
					t.Fatal("source damaged", err)
				}
			} else {
				if diskCalls.Load() != 1 {
					t.Fatal("filesystem phase count")
				}
				if b, err := os.ReadFile(next); err != nil || string(b) != "unchanged" {
					t.Fatal("rename missing", err)
				}
			}
		})
	}
}

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/internal/fileops"
)

type testRenameGitGate struct {
	ready    chan struct{}
	released atomic.Int32
	actual   renameGitSuspension
}

func (g *testRenameGitGate) Wait(ctx context.Context) error {
	select {
	case <-g.ready:
		return g.actual.Wait(ctx)
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (g *testRenameGitGate) Release() { g.released.Add(1); g.actual.Release() }

func TestEditorRenameBarrierKeepsLatestBufferAndReleasesOnce(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[failure], func(t *testing.T) {
			w := renameWorkbench(t)
			root := t.TempDir()
			old := filepath.Join(root, "old")
			next := filepath.Join(root, "next")
			if err := os.Mkdir(old, 0700); err != nil {
				t.Fatal(err)
			}
			path := writeFile(t, old, "a.go", "package p\n")
			if !openFileInEditor(w.tabs, path) {
				t.Fatal("open failed")
			}
			e := openEditors[path]
			gate := &testRenameGitGate{ready: make(chan struct{})}
			original := suspendRenameGit
			t.Cleanup(func() { suspendRenameGit = original })
			suspendRenameGit = func(p string) (renameGitSuspension, error) {
				if p != old {
					t.Fatal("wrong protected path", p)
				}
				actual, err := gui.SuspendEditorGitPath(p)
				if err != nil {
					return nil, err
				}
				gate.actual = actual
				return gate, nil
			}
			plan, err := prepareEditorRename(w.tabs, w.canvas, old, next)
			if err != nil {
				t.Fatal(err)
			}
			e.ReplaceAllText("package p\nvar Before int\n")
			e.ReplaceAllText("package p\nvar Latest int\n")
			e.Undo()
			e.Redo()
			if e.Text() != "package p\nvar Latest int\n" {
				t.Fatal("edit/undo unavailable while waiting")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := plan.Wait(ctx); !errors.Is(err, context.Canceled) {
				t.Fatal("wait claimed readiness", err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("wait mutated disk", err)
			}
			if failure {
				plan.Complete(context.DeadlineExceeded)
			} else {
				close(gate.ready)
				awaitEditorRenamePlan(t, plan)
				if err := fileops.RenameNoReplace(old, next); err != nil {
					t.Fatal(err)
				}
				plan.Complete(nil)
			}
			plan.Complete(nil)
			if gate.released.Load() != 1 || activeFileRename != nil {
				t.Fatal("reservation leak/double release", gate.released.Load())
			}
			expected := path
			if !failure {
				expected = filepath.Join(next, "a.go")
			}
			if e.FilePath() != expected || e.Text() != "package p\nvar Latest int\n" || !e.Modified() {
				t.Fatal("final identity/buffer lost")
			}
			if diskText(t, expected) != "package p\n" {
				t.Fatal("rename saved dirty buffer")
			}
		})
	}
}

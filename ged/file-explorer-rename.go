package ged

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/internal/fileops"
)

// SetRenameCoordinator installs a host reservation/commit hook. prepare runs on
// the UI thread BEFORE any disk mutation; an error rejects the operation. Its
// returned completion runs exactly once on the UI thread AFTER disk completion,
// including failure and a switched/removed explorer. It must release any host
// reservation and, on success, migrate open document identities without losing
// buffers. Neither callback may block. Disk mutation remains owned by explorer.
func (e *FileExplorer) SetRenameCoordinator(prepare func(oldPath, newPath string) (complete func(error), err error)) {
	if prepare == nil {
		e.renameCoordinator = nil
		return
	}
	e.renameCoordinator = func(oldPath, newPath string) (RenamePreparation, error) {
		complete, err := prepare(oldPath, newPath)
		return RenamePreparation{Complete: complete}, err
	}
}

// RenamePreparation is the optional background-work barrier between UI
// reservation and filesystem mutation. Wait runs on the explorer worker and
// must honor ctx; it must not access UI-owned state. Complete runs exactly once
// on the UI thread after wait failure, OS failure, or success. It must release
// all reservations. An accepted operation with nil Wait proceeds immediately.
type RenamePreparation struct {
	Wait     func(context.Context) error
	Complete func(error)
}

// SetRenamePreparation extends SetRenameCoordinator with an off-UI readiness
// barrier. Preparation itself still runs on the UI thread and must not block.
// A Wait error prevents all disk mutation and is reported as SILK-FS009 with its
// original cause. The default wait deadline is five seconds (cooperative).
func (e *FileExplorer) SetRenamePreparation(prepare func(oldPath, newPath string) (RenamePreparation, error)) {
	e.renameCoordinator = prepare
}
func (e *FileExplorer) RenameBusy() bool { return e.renameBusy }

// RenamePath asynchronously renames one in-root entry with no replacement.
// It returns validation/reservation errors immediately; after acceptance done
// runs once on the UI thread with the filesystem result (nil means committed).
// The root itself cannot be renamed. Pending OS I/O is not cancellable: the host
// must keep its event loop alive until completion, even after CancelScan.
func (e *FileExplorer) RenamePath(path, name string, done func(error)) error {
	return e.renamePath(path, name, done, false)
}

func (e *FileExplorer) renamePath(path, name string, done func(error), compatible bool) error {
	if e.renameBusy {
		return diagnostic.New(diagnostic.FileOperationBusy, "rename is still running", nil)
	}
	if err := validateFileName(name); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return diagnostic.New(diagnostic.FileName, "source path cannot be empty", nil)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return diagnostic.New(diagnostic.FileName, "invalid source path", err)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(e.rootDir, abs)
	if e.rootDir == "" || err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return diagnostic.New(diagnostic.FileName, "rename must target an entry inside the project, not its root", err).At(diagnostic.Location{File: abs})
	}
	next := filepath.Join(filepath.Dir(abs), strings.TrimSpace(name))
	e.renameBusy = true
	var preparation RenamePreparation
	if e.renameCoordinator != nil {
		preparation, err = e.renameCoordinator(abs, next)
		if err != nil {
			e.renameBusy = false
			return err
		}
	}
	root := e.rootDir
	post, rename := e.post, e.renameIO
	if post == nil {
		post = gui.Post
	}
	if rename == nil {
		rename = func(path, name string) (string, error) { return fsRenameMode(path, name, compatible) }
	}
	waitTimeout := e.renameWaitTimeout
	if waitTimeout <= 0 {
		waitTimeout = 5 * time.Second
	}
	go func() {
		var err error
		if preparation.Wait != nil {
			ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
			err = preparation.Wait(ctx)
			if err == nil {
				err = ctx.Err()
			} // never mutate after an expired wait
			cancel()
			if err != nil {
				err = diagnostic.New(diagnostic.FileOperationWait, "waiting for background work before rename", err).At(diagnostic.Location{File: abs})
			}
		}
		if err == nil {
			_, err = rename(abs, name)
		}
		post(func() {
			// Host completion must not be suppressed by a stale scan/root generation.
			// A real filesystem mutation has already finished and needs reconciliation.
			func() {
				defer func() { e.renameBusy = false }()
				if preparation.Complete != nil {
					preparation.Complete(err)
				}
				if err == nil && e.rootDir == root {
					e.rebaseRenamedPaths(abs, next)
					e.refreshKeepingExpansion()
				}
			}()
			if done != nil {
				done(err)
			}
		})
	}()
	return nil
}

func (e *FileExplorer) rebaseRenamedPaths(oldPath, newPath string) {
	remap := func(p string) string { mapped, _ := fileops.RebasePath(p, oldPath, newPath); return mapped }
	rekey := func(set map[string]bool) map[string]bool {
		out := make(map[string]bool, len(set))
		for p, v := range set {
			out[remap(p)] = v
		}
		return out
	}
	e.expanded = rekey(e.expanded)
	e.searchCollapsed = rekey(e.searchCollapsed)
	e.selectionPath = remap(e.selectionPath)
	e.revealPath = remap(e.revealPath)
	e.lastClickPath = remap(e.lastClickPath)
	var walk func(*fileEntry)
	walk = func(entry *fileEntry) {
		if entry == nil {
			return
		}
		entry.path = remap(entry.path)
		entry.name = filepath.Base(entry.path)
		entry.textCached = false
		for _, child := range entry.children {
			walk(child)
		}
	}
	walk(e.root)
	e.gitStatus = nil
}

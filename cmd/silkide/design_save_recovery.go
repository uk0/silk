package main

import (
	"context"
	"fmt"
	"os"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/internal/fileops"
)

func cleanupSavedRecovery(path string, expected core.FileFingerprint) error {
	if !expected.Known() {
		return fmt.Errorf("recovery cleanup skipped: original backup baseline unavailable")
	}
	actual, err := fileops.InspectFingerprint(path + ".autosave")
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("%w: recovery changed during manual save", core.ErrSaveConflict)
	}
	if actual == core.MissingFingerprint() {
		return nil
	}
	err = os.Remove(path + ".autosave")
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func safeCleanupSavedRecovery(cleanup func(string, core.FileFingerprint) error, path string, expected core.FileFingerprint) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("recovery cleanup panicked after primary save: %v", p)
		}
	}()
	return cleanup(path, expected)
}

// UI timer admission only. A recovery tick never replaces a manual queued
// save, adopts a filename/clean marker or constructs/traverses UI on a worker.
func requestDesignRecovery(scene *ged.GedScene) {
	w := globalWorkbench
	if w == nil || w.canvas == nil || w.canvas.IsClosed() || w.canvas.GedScene() != scene || scene.Filename() == "" || scene.IsClean() || transitionBusy() || anySaveBusy() || saveBatchBusy() || activeEditorClose != nil || activeFileRename != nil || workspaceSearchBusy() {
		return
	}
	c := w.designSaves
	if c == nil {
		c = newOwnedDesignSaver(w, ownedDesignSaveSettings{})
	}
	path := scene.Filename()
	doc, err := scene.SaveDesignWithPendingEdits()
	if err != nil {
		core.Warn("recovery capture: ", err)
		return
	}
	if !c.valid() || w.canvas.GedScene() != scene || scene.Filename() != path || anySaveBusy() {
		return
	}
	ticket, ok := reserveSavePaths(c.reservationOwner, []string{path, path + ".autosave"})
	if !ok {
		return
	}
	write := c.settings.recovery
	future, err := c.settings.queue.Submit(context.Background(), func(ctx context.Context) (designWriteResult, error) {
		if err := ctx.Err(); err != nil {
			return designWriteResult{}, err
		}
		err := write(ctx, doc, path+".autosave")
		if err != nil {
			core.Warn("background design recovery: ", path, ": ", err)
		}
		return designWriteResult{Written: err == nil}, err
	})
	// No UI publication is required for recovery. The physical future owns the
	// remaining reservation even after a canvas close or owner retirement.
	ticket.finish(false)
	if err != nil {
		ticket.finish(true)
		core.Warn("recovery admission: ", err)
		return
	}
	_ = future.OnComplete(func(f func()) { f() }, func(designWriteResult, error) { ticket.finish(true) })
}

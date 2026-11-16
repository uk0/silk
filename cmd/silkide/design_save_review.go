package main

import (
	"context"
	"path/filepath"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

var askReplaceDesignFile = func(canvas *ged.GedView, path string) gui.DialogResult {
	dlg := gui.NewDialog(i18n.T("Replace existing design?"), canvas)
	label := gui.NewLabel(i18n.Tf("%s already exists. Replace it with the current design? External changes made after this check will still be refused.", filepath.Base(path)))
	label.SetWrap(true)
	dlg.SetContent(label)
	dlg.AddButton(i18n.T("Replace"), gui.DialogOK)
	cancel := dlg.AddButton(i18n.T("Cancel"), gui.DialogCancel)
	dlg.SetDefaultButton(cancel)
	return dlg.ShowModal()
}

// UI-only review. The worker retains just path/inspector/ticket/mailbox; neither
// the scene nor native modal continuation crosses the worker boundary.
type designSaveReview struct {
	id, scope  *designSaveToken
	box        *designSaveMailbox
	scene      *ged.GedScene
	checkpoint ged.DesignEditCheckpoint
	path       string
	future     *async.Future[designWriteResult]
	ticket     *saveReservation
	run        func(string) bool
	done       func(bool)
}

func (c *ownedDesignSaver) reviewSaveAs(scene *ged.GedScene, path string, id, scope *designSaveToken, run func(string) bool, done func(bool)) bool {
	if !c.valid() || c.intent != id || c.review != nil || c.owner.canvas.GedScene() != scene {
		return false
	}
	if filepath.Ext(path) == "" {
		path += ged.DefaultDesignExt
	}
	ticket, ok := reserveSavePaths(c.reservationOwner, designSavePaths(path))
	if !ok {
		return false
	}
	r := &designSaveReview{id: id, scope: scope, box: new(designSaveMailbox), scene: scene, checkpoint: scene.EditCheckpoint(), path: path, ticket: ticket, run: run, done: done}
	if !c.valid() || c.intent != id || c.owner.canvas.GedScene() != scene || !r.checkpoint.ModelCurrent(scene) {
		ticket.finish(true)
		ticket.finish(false)
		return false
	}
	c.review = r
	r.box.owner.Store(c)
	inspect, post, box := c.settings.inspect, c.settings.post, r.box
	future, err := c.settings.queue.Submit(context.Background(), func(ctx context.Context) (designWriteResult, error) {
		v, err := inspect(ctx, path)
		return designWriteResult{Fingerprint: v}, err
	})
	if err != nil {
		c.review = nil
		box.owner.Store(nil)
		ticket.finish(true)
		ticket.finish(false)
		reportSaveFailure(err)
		return false
	}
	r.future = future
	_ = future.OnComplete(func(f func()) { f() }, func(result designWriteResult, err error) {
		ticket.finish(true)
		if box.owner.Load() == nil {
			return
		}
		post(func() {
			if owner := box.owner.Load(); owner != nil {
				owner.reviewedSaveAs(id, result.Fingerprint, err)
			}
		})
	})
	return true
}
func (c *ownedDesignSaver) reviewCurrent(r *designSaveReview) bool {
	return c.valid() && r != nil && c.review == r && c.intent == r.id && c.owner.canvas.GedScene() == r.scene && r.checkpoint.ModelCurrent(r.scene)
}
func (c *ownedDesignSaver) reviewedSaveAs(id *designSaveToken, version core.FileFingerprint, err error) {
	r := c.review
	if r == nil || r.id != id {
		return
	}
	r.future = nil
	if err != nil || !version.Known() || !c.reviewCurrent(r) {
		reportSaveFailure(err)
		if c.review == r {
			c.retireReview(async.ErrSuperseded)
		}
		return
	}
	if version == core.MissingFingerprint() {
		c.approveSaveAs(r, version)
		return
	}
	choice := askDirty(func() gui.DialogResult { return askReplaceDesignFile(c.owner.canvas, r.path) })
	if !c.reviewCurrent(r) {
		if c.review == r {
			c.retireReview(async.ErrSuperseded)
		}
		return
	}
	box := r.box
	// Native cleanup/focus restoration precedes draft capture and write admission.
	afterDesignSaveChooser(func() {
		if owner := box.owner.Load(); owner != nil && owner.review == r {
			if choice != gui.DialogOK || !owner.reviewCurrent(r) {
				owner.retireReview(async.ErrSuperseded)
				return
			}
			owner.approveSaveAs(r, version)
		}
	})
}
func (c *ownedDesignSaver) approveSaveAs(r *designSaveReview, version core.FileFingerprint) {
	if !c.reviewCurrent(r) {
		if c.review == r {
			c.retireReview(async.ErrSuperseded)
		}
		return
	}
	// The actual atomic write must match precisely the version reviewed, not a
	// fresh observation after user consent. It still detects later disk drift.
	c.diskState(r.scene, r.path).store(version)
	c.review = nil
	r.box.owner.Store(nil)
	r.ticket.finish(false)
	run, done := r.run, r.done
	r.run, r.done, r.scene = nil, nil, nil
	r.checkpoint = ged.DesignEditCheckpoint{}
	if !run(r.path) && done != nil {
		done(false)
	}
}
func (c *ownedDesignSaver) retireReview(err error) {
	if r := c.review; r != nil {
		c.review = nil
		r.box.owner.Store(nil)
		r.ticket.finish(false)
		if r.future != nil {
			r.future.Cancel()
			r.future = nil
		}
		done := r.done
		r.run, r.done, r.scene = nil, nil, nil
		r.checkpoint = ged.DesignEditCheckpoint{}
		if done != nil {
			done(false)
		}
	}
}

package main

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

type diffSource struct{ oldText, newText string }
type diffLoader func(context.Context) (diffSource, error)

var diffLoadExecutor = async.NewExecutor(2)
var errNoTextComparison = errors.New("no text changes to compare")

// The dialog is created before loading any disk/git data. Its single lifetime
// context covers both retrieval and comparison; late UI deliveries are fenced.
type diffDialog struct {
	dialog         *gui.Dialog
	view           *gui.DiffView
	status         *gui.Label
	previous, next *gui.Button
	ctx            context.Context
	cancel         context.CancelFunc
	closed         bool
	post           func(func())
}

func newDiffDialog(title string, parent gui.IWidget, post func(func())) *diffDialog {
	if post == nil {
		post = gui.Post
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &diffDialog{ctx: ctx, cancel: cancel, post: post}
	c.dialog = gui.NewDialog(title, parent)
	c.view = gui.NewDiffView()
	c.status = gui.NewLabel(i18n.T("Loading comparison..."))
	c.status.SetWrap(true)
	c.previous = gui.NewButton1(i18n.T("Previous change (P)"), nil)
	c.next = gui.NewButton1(i18n.T("Next change (N)"), nil)
	c.previous.SetEnabled(false)
	c.next.SetEnabled(false)
	c.previous.Action().BindFunc0(func() { c.view.JumpToPrevChange(); c.view.SetFocus() })
	c.next.Action().BindFunc0(func() { c.view.JumpToNextChange(); c.view.SetFocus() })
	actions := gui.NewHBox()
	actions.SetSpacing(6)
	actions.AddWidget(c.previous)
	actions.AddWidget(c.next)
	box := gui.NewVBox()
	box.SetSpacing(6)
	box.AddWidget(c.status)
	box.AddWidget(actions)
	box.AddWidget(c.view)
	c.dialog.SetContent(box)
	c.dialog.AddButton(i18n.T("Close"), gui.DialogCancel)
	c.dialog.SetSize(820, 600)
	c.dialog.SetInitialFocus(c.view)
	c.view.SetDiffStateChanged(func(v *gui.DiffView) {
		if c.closed {
			return
		}
		ready := !v.DiffPending() && v.DiffError() == nil && v.DiffSummary().ChangedRows() > 0
		c.previous.SetEnabled(ready)
		c.next.SetEnabled(ready)
		switch {
		case v.DiffPending():
			c.status.SetText(i18n.T("Computing comparison..."))
		case v.DiffError() != nil:
			c.status.SetText(i18n.Tf("Comparison failed: %v", v.DiffError()))
		default:
			s := v.DiffSummary()
			c.status.SetText(i18n.Tf("%d rows - %d changed", s.Rows, s.ChangedRows()))
		}
		c.dialog.Layout()
	})
	return c
}
func (c *diffDialog) start(load diffLoader) {
	if c.closed {
		return
	}
	future, err := async.Submit(c.ctx, diffLoadExecutor, load)
	if err != nil {
		c.status.SetText(i18n.Tf("Comparison failed: %v", err))
		c.dialog.Layout()
		return
	}
	_ = future.OnComplete(c.post, func(source diffSource, err error) {
		if c.closed || c.ctx.Err() != nil {
			return
		}
		if errors.Is(err, errNoTextComparison) {
			c.view.SetTexts("", "")
			c.status.SetText(i18n.T("No text changes to compare"))
			c.dialog.Layout()
			return
		}
		if err != nil {
			c.status.SetText(i18n.Tf("Comparison failed: %v", err))
			c.dialog.Layout()
			return
		}
		if err = c.view.SetTextsAsync(c.ctx, source.oldText, source.newText); err != nil {
			c.status.SetText(i18n.Tf("Comparison failed: %v", err))
			c.dialog.Layout()
		}
	})
}
func (c *diffDialog) close() {
	if c.closed {
		return
	}
	c.closed = true
	c.cancel()
	c.view.CancelDiff()
	c.view.SetDiffStateChanged(nil)
	c.dialog.SetParent(nil)
}
func showDiffDialog(title string, load diffLoader) {
	var parent gui.IWidget
	if globalFrame != nil {
		parent = globalFrame
	} else {
		parent = gui.DefaultFrame()
	}
	c := newDiffDialog(title, parent, gui.Post)
	defer c.close()
	c.start(load)
	c.dialog.ShowModal()
}

func loadSavedDiff(ctx context.Context, path, current string) (diffSource, error) {
	if err := ctx.Err(); err != nil {
		return diffSource{}, err
	}
	// os.ReadFile can block on the filesystem, but never on the UI thread. A
	// cancelled/closed dialog discards it; kernel I/O is not forcibly preempted.
	saved, err := os.ReadFile(path)
	if err != nil {
		return diffSource{}, err
	}
	if err = ctx.Err(); err != nil {
		return diffSource{}, err
	}
	return diffSource{string(saved), current}, nil
}
func loadPatchDiff(ctx context.Context, patch string) (diffSource, error) {
	if err := ctx.Err(); err != nil {
		return diffSource{}, err
	}
	files, err := core.ParseUnifiedDiff(patch)
	if err != nil {
		return diffSource{}, err
	}
	if len(files) == 0 {
		return diffSource{}, errNoTextComparison
	}
	// Preserve the current changed-neighborhood/first-file contract. Multi-file
	// navigation and omitted context reconstruction remain separate features.
	a, b := diffOldNewFromHunks(files[0].Hunks)
	if err = ctx.Err(); err != nil {
		return diffSource{}, err
	}
	return diffSource{strings.Join(a, "\n"), strings.Join(b, "\n")}, nil
}

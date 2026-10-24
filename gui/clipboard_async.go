package gui

import (
	"context"
	"errors"
	"strings"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/i18n"
)

const maxClipboardTextBytes = 16 << 20

var ErrClipboardTooLarge = errors.New("clipboard text exceeds 16 MiB")
var errClipboardPasteBurst = errors.New("too many pending paste requests (maximum 64)")

type clipboardTextReader func(context.Context, func(string, error)) error

// ReadTextAsync reads plain text and delivers once through gui.Post. Call on
// the UI thread with a live window. Cancellation is reported as an error, and
// never applies text itself. macOS reads in a bounded OS helper process instead
// of calling main-thread-only GLFW from a worker. Other backends retain their
// native UI-thread read; asynchronous delivery is not a latency guarantee there.
// Payloads over 16 MiB are rejected, never silently truncated.
func (c *clipBoard) ReadTextAsync(ctx context.Context, done func(string, error)) error {
	if ctx == nil || done == nil {
		return async.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return readClipboardTextAsync(ctx, func(text string, err error) {
		if ctx.Err() != nil {
			done("", ctx.Err())
			return
		}
		done(text, err)
	})
}

type clipboardPaste struct {
	cancel  context.CancelFunc
	valid   func() bool
	apply   func(string)
	repeats int
	owner   *Window
}

// PastePending reports a pending built-in control paste. UI thread only.
func (w *Widget) PastePending() bool { return w.pendingPaste != nil }

// CancelPendingPaste cancels this widget's pending read without blocking.
// Late callbacks cannot edit the widget. Hide/disable/reparent also cancel.
func (w *Widget) CancelPendingPaste() {
	if p := w.pendingPaste; p != nil {
		w.pendingPaste = nil
		p.cancel()
	}
}
func cancelClipboardPastes(root IWidget) {
	if root == nil {
		return
	}
	root.NakedWidget().CancelPendingPaste()
	for _, child := range root.Children() {
		cancelClipboardPastes(child)
	}
}
func (w *Widget) pasteTargetValid(p *clipboardPaste) bool {
	if w.self == nil || !w.IsEnabled() || !w.IsAllAncentorsVisible() || w.OwnerWindow() != p.owner {
		return false
	}
	// Headless SDK tests have no native focus. Real deliveries require the same
	// editor to own focus; closing a popup must not paste into another document.
	if p.owner != nil && !w.HasFocus() {
		return false
	}
	return p.valid()
}
func (w *Widget) pasteFailed(err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	core.Warn("clipboard paste: ", err)
	if w.OwnerWindow() != nil {
		ShowToast(w.self, i18n.Tf("Paste failed: %v", err), 3500, ToastError)
	}
}

// Keep the other platforms' synchronous control behavior for compatibility.
// The injected reader is a deterministic unit-test seam, never a global hook.
func (w *Widget) tryAsyncPaste(valid func() bool, apply func(string)) bool {
	if !asyncClipboardControls && w.clipboardReader == nil {
		return false
	}
	if !valid() || !w.IsEnabled() {
		return true
	}
	if p := w.pendingPaste; p != nil {
		if w.pasteTargetValid(p) {
			// Repeated paste keys at an unchanged anchor are one insertion/Undo group,
			// not discarded search requests. Bound the burst before reading any bytes.
			if p.repeats >= 64 {
				w.CancelPendingPaste()
				w.pasteFailed(errClipboardPasteBurst)
			} else {
				p.repeats++
			}
			return true
		}
		w.CancelPendingPaste()
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &clipboardPaste{cancel: cancel, valid: valid, apply: apply, repeats: 1, owner: w.OwnerWindow()}
	w.pendingPaste = p
	read := w.clipboardReader
	if read == nil {
		read = Clipboard.ReadTextAsync
	}
	finish := func(text string, err error) {
		if w.pendingPaste != p {
			return
		}
		w.pendingPaste = nil
		defer cancel()
		if !w.pasteTargetValid(p) {
			return
		}
		if err != nil {
			w.pasteFailed(err)
			return
		}
		if text == "" {
			return
		}
		if len(text) > maxClipboardTextBytes/p.repeats {
			w.pasteFailed(ErrClipboardTooLarge)
			return
		}
		if err := clipboardTextErr(text); err != nil {
			w.pasteFailed(err)
			return
		}
		p.apply(strings.Repeat(text, p.repeats))
	}
	if err := read(ctx, finish); err != nil {
		finish("", err)
	}
	return true
}

package main

import (
	"context"
	"errors"
	"sync"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// preferenceWriter owns one demand-driven host worker. The SDK's Sync joins its
// own disk writer; neither worker lives when idle. At most one UI notification
// is queued, irrespective of save-request count or a stalled UI event loop.
type preferenceWriter struct {
	mu                      sync.Mutex
	save                    func() error
	post                    func(func()) // queues, never executes inline (gui.Post)
	report                  func(error)  // UI thread only
	pending, closed, posted bool
	flight                  *preferenceFlight
	lastErr                 error
}
type preferenceFlight struct {
	done chan struct{}
	err  error
}

func (w *preferenceWriter) request() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("preferences writer has shut down")
	}
	w.requestLocked()
	return nil
}
func (w *preferenceWriter) requestLocked() *preferenceFlight {
	w.pending = true
	if w.flight == nil {
		w.flight = &preferenceFlight{done: make(chan struct{})}
		go w.run(w.flight)
	}
	return w.flight
}
func (w *preferenceWriter) run(f *preferenceFlight) {
	for {
		w.mu.Lock()
		w.pending = false
		w.mu.Unlock()
		err := w.save() // no host or UI lock held while SDK writes/joins
		w.mu.Lock()
		w.lastErr = err
		if w.pending {
			w.mu.Unlock()
			continue
		}
		w.pending = false
		if !w.closed && !w.posted {
			w.posted = true
			// gui.Post only enqueues. Include enqueue/wakeup in the joined lifecycle
			// so no worker can wake a destroyed UI backend after Shutdown returns.
			w.post(w.deliver)
		}
		f.err = err
		w.flight = nil
		close(f.done)
		w.mu.Unlock()
		return
	}
}
func (w *preferenceWriter) deliver() {
	w.mu.Lock()
	w.posted = false
	if w.closed || w.flight != nil {
		w.mu.Unlock()
		return
	}
	err := w.lastErr
	w.mu.Unlock()
	if w.report != nil {
		w.report(err)
	}
}
func (w *preferenceWriter) wait(ctx context.Context, shutdown bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.mu.Lock()
	f := w.flight
	if !w.closed {
		f = w.requestLocked()
		if shutdown {
			w.closed = true
		}
	}
	err := w.lastErr
	w.mu.Unlock()
	if f == nil {
		return err
	}
	select {
	case <-f.done:
		return f.err
	default:
	}
	select {
	case <-f.done:
		return f.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *preferences) saveWriter() *preferenceWriter {
	p.saveOnce.Do(func() { p.writer = &preferenceWriter{save: p.store.Sync, post: gui.Post, report: p.reportSaveResult} })
	return p.writer
}
func (p *preferences) requestSave() error {
	if err := p.saveWriter().request(); err != nil {
		return diagnostic.New(diagnostic.SettingsWrite, "cannot queue settings save", err).At(diagnostic.Location{File: p.store.Path()})
	}
	return nil
}
func (p *preferences) Flush(ctx context.Context) error    { return p.waitSave(ctx, false) }
func (p *preferences) Shutdown(ctx context.Context) error { return p.waitSave(ctx, true) }
func (p *preferences) waitSave(ctx context.Context, shutdown bool) error {
	err := p.saveWriter().wait(ctx, shutdown)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return diagnostic.New(diagnostic.SettingsWait, "preferences save has not finished", err).At(diagnostic.Location{File: p.store.Path()})
	}
	return err
}
func preferenceErrorDetails(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	var detail *diagnostic.Error
	if errors.As(err, &detail) {
		message += "\n" + detail.Hint
	}
	return message
}
func (p *preferences) reportSaveResult(err error) {
	message := preferenceErrorDetails(err)
	if message == p.lastSaveMessage {
		return
	}
	p.lastSaveMessage = message
	if err != nil {
		logEvent(ged.LogWarn, message)
		silkideToast(err.Error(), gui.ToastWarning)
	}
}

// Called after core.EventLoop and after final session capture, with producers
// stopped. No timeout may silently strand the final writer during process exit.
// An OS filesystem stall can delay exit; it cannot freeze an active UI callback.
func finishPreferenceSaves(p *preferences) error {
	if p == nil {
		return nil
	}
	err := p.Shutdown(context.Background())
	if err != nil {
		core.Warn(preferenceErrorDetails(err))
	}
	return err
}

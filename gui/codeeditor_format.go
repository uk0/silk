package gui

import (
	"context"
	"errors"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/formatx"
)

type EditorFormatOptions = formatx.Options

type editorFormatJob struct {
	future    *async.Future[string]
	cancelled bool // UI thread only
}

// FormatDocument is the synchronous SDK form. Interactive callers should use
// FormatDocumentAsync. Replacement preserves earlier history and adds one Undo.
func (e *CodeEditor) FormatDocument(options EditorFormatOptions) error {
	if e.readOnly {
		return ErrEditorReadOnly
	}
	e.CancelFormat()
	text, err := formatx.Format(context.Background(), e.Language(), e.Text(), options)
	if err == nil {
		e.ReplaceAllText(text)
	}
	return err
}

// FormatCode retains its Go-only synchronous compatibility contract. It no
// longer launches gofmt through PATH or resets the editor's Undo history.
func (e *CodeEditor) FormatCode() {
	if e.readOnly || e.Language() != "go" {
		return
	}
	if err := e.FormatDocument(EditorFormatOptions{}); err != nil {
		core.Error(err)
	}
}

// Formatting reports a non-cancelled request awaiting UI delivery.
func (e *CodeEditor) Formatting() bool { return e.formatJob != nil && !e.formatJob.cancelled }

// CancelFormat suppresses the result and callback, without waiting. A cancelled
// job still occupies this editor's slot until its worker has actually returned.
// Hosts must call this on disposal/before stopping the UI event loop.
func (e *CodeEditor) CancelFormat() {
	if job := e.formatJob; job != nil && !job.cancelled {
		job.cancelled = true
		job.future.Cancel()
		e.Update()
	}
}

// FormatDocumentAsync starts bounded background formatting. All widget access,
// calls to this method and done run on the UI thread. Admission errors are
// returned immediately (done is not called). Editing, changing path, hiding or
// cancelling an editor retires both result and callback, including edit/undo ABA.
func (e *CodeEditor) FormatDocumentAsync(options EditorFormatOptions, done func(error)) error {
	if e.readOnly {
		return ErrEditorReadOnly
	}
	if e.formatJob != nil {
		return async.ErrBusy
	}
	text, language, path := e.Text(), e.Language(), e.filePath
	revision := e.TextRevision()
	var future *async.Future[string]
	var err error
	if e.formatWork != nil { // deterministic worker boundary for SDK tests
		work := e.formatWork
		future, err = async.Submit(context.Background(), editorFormatExecutor, func(ctx context.Context) (string, error) { return work(ctx, language, text, options) })
	} else {
		future, err = formatx.FormatAsync(context.Background(), editorFormatExecutor, language, text, options)
	}
	if err != nil {
		return err
	}
	job := &editorFormatJob{future: future}
	e.formatJob = job
	post := e.formatPost
	if post == nil {
		post = Post
	}
	_ = future.OnComplete(post, func(formatted string, err error) {
		if e.formatJob != job {
			return
		}
		e.formatJob = nil
		if job.cancelled || e.filePath != path || e.Language() != language || e.TextRevision() != revision {
			return
		}
		e.Update()
		if err == nil {
			e.ReplaceAllText(formatted)
		}
		if done != nil {
			done(err)
		}
	})
	e.Update()
	return nil
}

var editorFormatExecutor = async.NewExecutor(2)

// Used by the embedded editor's shortcut and the SDK code-panel button.
func (e *CodeEditor) RequestFormat() {
	if e.readOnly {
		return
	}
	report := func(err error) {
		if err != nil && !errors.Is(err, context.Canceled) {
			core.Error(err)
		}
	}
	report(e.FormatDocumentAsync(EditorFormatOptions{}, report))
}

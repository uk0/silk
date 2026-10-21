package gui

import (
	"fmt"
	"runtime/debug"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/diagnostic"
)

// ModalError returns the last recovered initialization/event-loop failure.
// Cancellation is not an error. Read after ShowModal returns, on the UI thread.
func (w *Window) ModalError() error { return w.modalErr }

func modalPanicDiagnostic(value any) *diagnostic.Error {
	cause, ok := value.(error)
	if !ok {
		cause = fmt.Errorf("%v", value)
	}
	return diagnostic.New(diagnostic.GUIModalPanic, "", cause)
}
func reportModalPanic(value any) error {
	err := modalPanicDiagnostic(value)
	core.Warn(err.Error(), "\n", err.Hint, "\n", string(debug.Stack()))
	return err
}

package main

import (
	"errors"
	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

// Recovered SDK exceptions must not look like an ordinary user cancellation.
func reportDialogFailure(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	var detail *diagnostic.Error
	if errors.As(err, &detail) {
		message += "\n" + detail.Hint
	}
	logEvent(ged.LogError, message)
	silkideToast(err.Error(), gui.ToastError)
	return true
}

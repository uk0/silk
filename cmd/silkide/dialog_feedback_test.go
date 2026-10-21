package main

import (
	"errors"
	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"strings"
	"testing"
)

func TestDialogFailureCodeAndRecoveryFeedback(t *testing.T) {
	oldToast, oldLog := silkideToast, globalLog
	defer func() { silkideToast, globalLog = oldToast, oldLog }()
	globalLog = ged.NewLogPanel()
	var messages []string
	silkideToast = func(s string, l gui.ToastLevel) {
		messages = append(messages, s)
		if l != gui.ToastError {
			t.Error("wrong severity")
		}
	}
	if reportDialogFailure(nil) || len(messages) != 0 {
		t.Fatal("normal cancellation shown as error")
	}
	err := diagnostic.New(diagnostic.GUIModalPanic, "", errors.New("callback failed"))
	if !reportDialogFailure(err) || len(messages) != 1 || !strings.Contains(messages[0], "SILK-GUI001") {
		t.Fatal(messages)
	}
	entries := globalLog.Entries()
	if len(entries) != 1 || !strings.Contains(entries[0].Message, err.Hint) || !strings.Contains(entries[0].Message, "callback failed") {
		t.Fatal("recovery hint/cause missing")
	}
}

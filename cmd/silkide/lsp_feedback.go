package main

import (
	"context"
	"errors"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

// UI-thread-only: keep a stable code in the short banner and the underlying
// cause + recovery hint in the session Log pane. Never show cancellation of
// a superseded request as a language-service failure.
func reportLSPUnavailable() *diagnostic.Error {
	err := diagnostic.New(diagnostic.LSPUnavailable, i18n.T("LSP not running"), nil)
	logEvent(ged.LogWarn, err.Error()+"\n"+err.Hint)
	silkideToast(err.Error(), gui.ToastWarning)
	return err
}

func reportLSPRequestError(operation, path string, cause error) *diagnostic.Error {
	if cause == nil || (errors.Is(cause, context.Canceled) && diagnostic.CodeOf(cause) != diagnostic.LSPTransport) {
		return nil
	}
	code := diagnostic.LSPRequest
	if diagnostic.CodeOf(cause) == diagnostic.LSPTransport {
		code = diagnostic.LSPTransport
	}
	err := diagnostic.New(code, operation, cause).At(diagnostic.Location{File: path})
	logEvent(ged.LogError, err.Error()+"\n"+err.Hint)
	silkideToast(err.Error(), gui.ToastError)
	return err
}

// UI-thread-only and session guarded. No user buffer is discarded on failure.
// Snapshot the SDK terminal cause before retiring request deliveries, otherwise
// cancellation/stale-result guards can hide the only useful explanation.
func finishLSPConnection(client *core.LSPClient, epoch uint64) {
	if globalLSP != client || epoch != lspSessionGeneration {
		return
	}
	cause := client.Err()
	globalLSP = nil
	lspActivityPhase = "offline"
	refreshLSPActivity()
	retireLSPDocuments()
	if cause != nil {
		reportLSPRequestError(i18n.T("Language service disconnected"), "", cause)
	}
	go client.Close()
}

func reportLSPStartFailure(epoch uint64, cause error) {
	if epoch != lspSessionGeneration {
		return
	}
	lspActivityPhase = "offline"
	refreshLSPActivity()
	if lspStartCancel != nil {
		lspStartCancel()
		lspStartCancel = nil
	}
	err := diagnostic.New(diagnostic.LSPUnavailable, i18n.T("Language service failed to start"), cause)
	logEvent(ged.LogWarn, err.Error()+"\n"+err.Hint)
	silkideToast(err.Error(), gui.ToastWarning)
}

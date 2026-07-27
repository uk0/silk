package main

import (
	"encoding/json"

	"github.com/uk0/silk/core"
)

// LSP MessageType (spec §window/showMessage). gopls reports why it cannot
// serve a workspace through these — the text is the only explanation the user
// ever gets for "the IDE opened but nothing works".
const (
	lspMsgError   = 1
	lspMsgWarning = 2
	lspMsgInfo    = 3
	lspMsgLog     = 4
)

// lspWindowMessage extracts the payload of a window/showMessage or
// window/logMessage notification: {"type": <1..4>, "message": "..."}.
// ok is false for any other method, a nil message, or params that do not
// decode — the caller then falls back to logging the bare method name.
func lspWindowMessage(m *core.LSPMessage) (text string, level int, ok bool) {
	if m == nil {
		return "", 0, false
	}
	if m.Method != "window/showMessage" && m.Method != "window/logMessage" {
		return "", 0, false
	}
	var p struct {
		Type    int    `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return "", 0, false
	}
	if p.Message == "" {
		return "", 0, false
	}
	if p.Type < lspMsgError || p.Type > lspMsgLog {
		p.Type = lspMsgInfo
	}
	return p.Message, p.Type, true
}

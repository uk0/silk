package main

import (
	"encoding/json"
	"testing"

	"github.com/uk0/silk/core"
)

func msg(method, params string) *core.LSPMessage {
	return &core.LSPMessage{Method: method, Params: json.RawMessage(params)}
}

// TestLSPWindowMessageExtractsText is the regression guard for the bug this
// fixes: silkide logged only "lsp: window/showMessage" and discarded the text,
// so a gopls failure was unexplainable from the log.
func TestLSPWindowMessageExtractsText(t *testing.T) {
	cases := []struct {
		name, method, params, wantText string
		wantLevel                      int
	}{
		{"showMessage error", "window/showMessage",
			`{"type":1,"message":"no go.mod file found"}`, "no go.mod file found", lspMsgError},
		{"showMessage warning", "window/showMessage",
			`{"type":2,"message":"upgrade gopls"}`, "upgrade gopls", lspMsgWarning},
		{"logMessage info", "window/logMessage",
			`{"type":3,"message":"go env GOPATH"}`, "go env GOPATH", lspMsgInfo},
		{"logMessage log", "window/logMessage",
			`{"type":4,"message":"trace"}`, "trace", lspMsgLog},
	}
	for _, c := range cases {
		text, level, ok := lspWindowMessage(msg(c.method, c.params))
		if !ok {
			t.Errorf("%s: ok=false, want true", c.name)
			continue
		}
		if text != c.wantText {
			t.Errorf("%s: text=%q, want %q", c.name, text, c.wantText)
		}
		if level != c.wantLevel {
			t.Errorf("%s: level=%d, want %d", c.name, level, c.wantLevel)
		}
	}
}

// TestLSPWindowMessageIgnoresOthers keeps every non-window notification on the
// original bare-method log path.
func TestLSPWindowMessageIgnoresOthers(t *testing.T) {
	for _, c := range []struct {
		name string
		m    *core.LSPMessage
	}{
		{"nil", nil},
		{"other method", msg("textDocument/publishDiagnostics", `{}`)},
		{"bad json", msg("window/showMessage", `{`)},
		{"empty message", msg("window/showMessage", `{"type":1,"message":""}`)},
	} {
		if _, _, ok := lspWindowMessage(c.m); ok {
			t.Errorf("%s: ok=true, want false", c.name)
		}
	}
}

// TestLSPWindowMessageClampsUnknownType keeps an out-of-range severity usable
// instead of dropping the message.
func TestLSPWindowMessageClampsUnknownType(t *testing.T) {
	for _, params := range []string{
		`{"type":0,"message":"x"}`,
		`{"type":99,"message":"x"}`,
		`{"message":"x"}`,
	} {
		text, level, ok := lspWindowMessage(msg("window/showMessage", params))
		if !ok || text != "x" {
			t.Fatalf("params %s: text=%q ok=%v", params, text, ok)
		}
		if level != lspMsgInfo {
			t.Errorf("params %s: level=%d, want %d (info)", params, level, lspMsgInfo)
		}
	}
}

package core

import (
	"bytes"
	"strings"
	"testing"
)

func TestQuietLogFilterPreservesDiagnosticContent(t *testing.T) {
	for _, prefix := range []string{"", "2026/10/24 12:34:56 ", "12:34:56 ", "12:34:56.123456 "} {
		var output bytes.Buffer
		w := newSTWriter(&output, false)
		lines := []string{"warning: ui: Set SILK_DEBUG=1 for goroutine stacks.", "error: debugger failed to start", "traceback in response body", "silkide: gopls debug adapter missing"}
		input := prefix + "debug: noisy\n" + prefix + "dbg: noisy\n" + prefix + "trace: noisy\n"
		for _, line := range lines {
			input += prefix + line + "\n"
		}
		// Exercise partial writes, not only a single newline-terminated chunk.
		for _, part := range []string{input[:7], input[7:29], input[29:]} {
			if _, err := w.Write([]byte(part)); err != nil {
				t.Fatal(err)
			}
		}
		got := output.String()
		if strings.Contains(got, "noisy") {
			t.Fatalf("debug severity leaked: %q", got)
		}
		for _, line := range lines {
			if !strings.Contains(got, line) {
				t.Fatalf("diagnostic disappeared: %q in %q", line, got)
			}
		}
	}
}

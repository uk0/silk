package main

import (
	"context"
	"errors"
)

func lspCompletionStatus(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "gopls timed out; Ctrl+Space to retry"
	}
	return "gopls unavailable; check Tools > Toolchain Status"
}

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

type completionFailureProbe struct {
	documentProbe
	failure error
}

func (p *completionFailureProbe) CompletionContext(context.Context, string, int, int) ([]core.LSPCompletionItem, error) {
	return nil, p.failure
}

func TestLSPCompletionFailureIsDelivered(t *testing.T) {
	for _, failure := range []error{context.DeadlineExceeded, errors.New("server unavailable")} {
		t.Run(failure.Error(), func(t *testing.T) {
			probe := &completionFailureProbe{failure: failure}
			done := make(chan error, 1)
			d := newLSPDocument(probe, "file:///a.go", "Pr", nil, func(s lspSnapshot, _ []core.LSPCompletionItem, err error) {
				if s.version == 2 {
					done <- err
				}
			})
			defer func() { d.close(); waitDocument(t, d) }()
			d.update(lspSnapshot{text: "Pr", version: 2, col: 2, complete: true})
			select {
			case err := <-done:
				if !errors.Is(err, failure) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("silent failure")
			}
		})
	}
}

func TestLSPCompletionFailureStaleAndClosedSuppressed(t *testing.T) {
	calls := 0
	d := &lspDocument{latest: lspSnapshot{version: 3}, deliver: func(lspSnapshot, []core.LSPCompletionItem, error) { calls++ }}
	d.finish(lspSnapshot{version: 2}, nil, context.DeadlineExceeded)
	d.closed = true
	d.finish(lspSnapshot{version: 3}, nil, context.DeadlineExceeded)
	if calls != 0 {
		t.Fatal("stale failure delivered")
	}
}

func TestLSPCompletionStatusDoesNotExposeServerPayload(t *testing.T) {
	if got := lspCompletionStatus(fmt.Errorf("RPC: %w", context.DeadlineExceeded)); !strings.Contains(got, "Ctrl+Space") {
		t.Fatal(got)
	}
	if got := lspCompletionStatus(errors.New("secret document payload")); strings.Contains(got, "secret") || !strings.Contains(got, "Toolchain Status") {
		t.Fatal(got)
	}
}

package core

import (
	"bufio"
	"context"
	"errors"
	"testing"
	"time"
)

func TestLSPRequestCancellationSendsProtocolAndClearsPending(t *testing.T) {
	c, in, out := newPipedClient(t)
	defer in.Close()
	defer out.Close()
	defer c.stdin.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := c.SendRequestContext(ctx, "textDocument/completion", nil); result <- err }()
	r := bufio.NewReader(in)
	request, err := ReadLSPMessage(r)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	m, err := ReadLSPMessage(r)
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != "$/cancelRequest" {
		t.Fatal(m.Method)
	}
	var p struct {
		ID int `json:"id"`
	}
	if err := DecodeParams(m, &p); err != nil {
		t.Fatal(err)
	}
	if request.ID == nil || p.ID != 1 {
		t.Fatal(p)
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel blocked")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pending) != 0 {
		t.Fatal("pending request leaked")
	}
}
func TestLSPCloseUnblocksUnresponsiveWriter(t *testing.T) {
	c, in, out := newPipedClient(t)
	defer in.Close()
	defer out.Close()
	done := make(chan struct{})
	go func() { _ = c.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close hung on stdin")
	}
}

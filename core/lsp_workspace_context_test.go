package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func readWorkspaceFrame(t *testing.T, r *bufio.Reader) *LSPMessage {
	t.Helper()
	type result struct {
		m   *LSPMessage
		err error
	}
	ch := make(chan result, 1)
	go func() { m, e := ReadLSPMessage(r); ch <- result{m, e} }()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatal(r.err)
		}
		return r.m
	case <-time.After(3 * time.Second):
		t.Fatal("workspace protocol frame timeout")
		return nil
	}
}

func TestWorkspaceSymbolContextCancelsAndRejectsLateReply(t *testing.T) {
	c, in, out := newPipedClient(t)
	defer in.Close()
	defer out.Close()
	defer c.stdin.Close()
	reader := bufio.NewReader(in)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := c.WorkspaceSymbolContext(ctx, "Target中文"); result <- err }()
	req := readWorkspaceFrame(t, reader)
	var params struct {
		Query string `json:"query"`
	}
	if err := DecodeParams(req, &params); err != nil || req.Method != "workspace/symbol" || params.Query != "Target中文" {
		t.Fatalf("bad request: %+v %v", req, err)
	}
	cancel()
	cancelled := readWorkspaceFrame(t, reader)
	var id struct {
		ID json.RawMessage `json:"id"`
	}
	if err := DecodeParams(cancelled, &id); err != nil || cancelled.Method != "$/cancelRequest" || req.ID == nil || string(id.ID) != string(*req.ID) {
		t.Fatalf("wrong cancellation: %+v %v", cancelled, err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not return")
	}
	c.mu.Lock()
	pending := len(c.pending)
	c.mu.Unlock()
	if pending != 0 {
		t.Fatalf("pending after cancel=%d", pending)
	}
	type answer struct {
		symbols []LSPWorkspaceSymbol
		err     error
	}
	next := make(chan answer, 1)
	go func() { s, e := c.WorkspaceSymbolContext(context.Background(), "Current"); next <- answer{s, e} }()
	req2 := readWorkspaceFrame(t, reader)
	if err := WriteLSPMessage(out, &LSPMessage{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`[{"name":"Stale"}]`)}); err != nil {
		t.Fatal(err)
	}
	if err := WriteLSPMessage(out, &LSPMessage{JSONRPC: "2.0", ID: req2.ID, Result: json.RawMessage(`[{"name":"Current","location":{"uri":"file:///a.go","range":{"start":{"line":4,"character":17}}}}]`)}); err != nil {
		t.Fatal(err)
	}
	select {
	case a := <-next:
		if a.err != nil || len(a.symbols) != 1 || a.symbols[0].Name != "Current" || a.symbols[0].Character != 17 {
			t.Fatalf("late reply or UTF16 corruption: %+v", a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("new request blocked")
	}
}

func TestWorkspaceSymbolContextPrecancelledDoesNotWrite(t *testing.T) {
	c, in, out := newPipedClient(t)
	defer in.Close()
	defer out.Close()
	defer c.stdin.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.WorkspaceSymbolContext(ctx, "No wire activity"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nextID != 0 || len(c.pending) != 0 {
		t.Fatal("pre-cancelled request reached transport")
	}
}

func TestWorkspaceSymbolContextDeadline(t *testing.T) {
	c, in, out := newPipedClient(t)
	defer in.Close()
	defer out.Close()
	defer c.stdin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := c.WorkspaceSymbolContext(ctx, "Timeout"); result <- err }()
	r := bufio.NewReader(in)
	readWorkspaceFrame(t, r)
	if m := readWorkspaceFrame(t, r); m.Method != "$/cancelRequest" {
		t.Fatal(m.Method)
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deadline lost")
	}
}

package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/uk0/silk/diagnostic"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// This is an OS pipe, not a fake request callback. No reader consumes its bytes.
// The large body exceeds the anonymous pipe capacity on macOS and Windows.
type lspStalledPipe struct {
	*os.File
	started chan struct{}
	once    sync.Once
}

func (p *lspStalledPipe) Write(b []byte) (int, error) {
	if len(b) > 65536 {
		p.once.Do(func() { close(p.started) })
	}
	return p.File.Write(b)
}
func TestLSPContextCancelsStalledNativePipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	pipe := &lspStalledPipe{File: w, started: make(chan struct{})}
	c := &LSPClient{stdin: pipe, pending: make(map[int]chan *LSPMessage)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, e := c.SendRequestContext(ctx, "workspace/symbol", strings.Repeat("x", 1<<20)); result <- e }()
	select {
	case <-pipe.started:
	case <-time.After(3 * time.Second):
		t.Fatal("write never reached native pipe")
	}
	cancel()
	select {
	case e := <-result:
		if !errors.Is(e, context.Canceled) {
			t.Fatalf("lost cancellation: %v", e)
		}
	case <-time.After(time.Second):
		_ = w.Close()
		select {
		case <-result:
		case <-time.After(2 * time.Second):
			t.Fatal("native close failed to release writer")
		}
		t.Fatal("context cancellation blocked on full native pipe until external Close")
	}
	c.mu.Lock()
	n := len(c.pending)
	c.mu.Unlock()
	if n != 0 {
		t.Fatalf("pending leaked: %d", n)
	}
	select {
	case <-c.writeStopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stalled canceled request did not retire transport")
	}
	if diagnostic.CodeOf(c.Err()) != diagnostic.LSPTransport {
		t.Fatal(c.Err())
	}
	if _, e := c.SendRequest("next", nil); diagnostic.CodeOf(e) != diagnostic.LSPTransport {
		t.Fatalf("broken stream reused: %v", e)
	}
	_ = c.Close()
}

func TestLSPCanceledGateWaitKeepsSessionUsable(t *testing.T) {
	buf := &memWriteCloser{Buffer: new(bytes.Buffer)}
	c := &LSPClient{stdin: buf, pending: make(map[int]chan *LSPMessage)}
	c.initTransport()
	c.writeGate <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if err := c.SendNotificationContext(ctx, "must-not-send", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	<-c.writeGate
	if buf.Len() != 0 {
		t.Fatal("cancelled notification wrote bytes")
	}
	if err := c.SendNotification("alive", nil); err != nil {
		t.Fatal(err)
	}
	m, err := ReadLSPMessage(bufio.NewReader(bytes.NewReader(buf.Bytes())))
	if err != nil || m.Method != "alive" {
		t.Fatalf("session damaged: %+v %v", m, err)
	}
}

func TestLSPCancellationDoesNotWaitForPeerAndWorkerRetires(t *testing.T) {
	c, in, out := newPipedClient(t)
	defer in.Close()
	defer out.Close()
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := c.SendRequestContext(ctx, "workspace/symbol", nil); result <- err }()
	readWorkspaceFrame(t, bufio.NewReader(in))
	// Never consume the cancellation frame. The caller must still retire now.
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("request waited for cancellation frame")
	}
	select {
	case <-c.cancelDone:
	case <-time.After(3 * time.Second):
		t.Fatal("stalled cancellation worker leaked")
	}
	if diagnostic.CodeOf(c.transportError()) != diagnostic.LSPTransport {
		t.Fatal(c.transportError())
	}
}

func TestLSPCancellationQueueBounded(t *testing.T) {
	c, in, out := newPipedClient(t)
	defer in.Close()
	defer out.Close()
	defer c.Close()
	c.initTransport()
	c.writeGate <- struct{}{}
	for id := 1; id <= 10000; id++ {
		c.cancelRequest(id)
	}
	if n := len(c.cancelIDs); n > lspCancellationQueueSize {
		t.Fatalf("queue=%d", n)
	}
	c.stopWrites(errors.New("test teardown"))
	<-c.writeGate
	select {
	case <-c.cancelDone:
	case <-time.After(time.Second):
		t.Fatal("cancellation worker did not stop")
	}
}

func TestLSPConcurrentFramesAndNotificationOrder(t *testing.T) {
	c, in, out := newPipedClient(t)
	defer in.Close()
	defer out.Close()
	defer c.Close()
	const count = 80
	reader := bufio.NewReader(in)
	received := make(chan error, 1)
	go func() {
		ids := map[string]bool{}
		for i := 0; i < count; i++ {
			m, err := ReadLSPMessage(reader)
			if err != nil {
				received <- err
				return
			}
			if m.ID == nil || ids[string(*m.ID)] {
				received <- fmt.Errorf("duplicate/missing ID: %+v", m)
				return
			}
			ids[string(*m.ID)] = true
			if err = WriteLSPMessage(out, &LSPMessage{JSONRPC: "2.0", ID: m.ID, Result: json.RawMessage(`null`)}); err != nil {
				received <- err
				return
			}
		}
		for _, want := range []string{"textDocument/didOpen", "textDocument/didChange", "textDocument/didClose"} {
			m, err := ReadLSPMessage(reader)
			if err != nil {
				received <- err
				return
			}
			if m.Method != want {
				received <- fmt.Errorf("notification order: %s != %s", m.Method, want)
				return
			}
		}
		received <- nil
	}()
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := c.SendRequest("frame-check", strings.Repeat("z", 4096)); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	for _, method := range []string{"textDocument/didOpen", "textDocument/didChange", "textDocument/didClose"} {
		if err := c.SendNotification(method, nil); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-received:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("framing/ordering stalled")
	}
}

type shortLSPWriter struct {
	calls   int
	shortOn int
}

func (w *shortLSPWriter) Write(b []byte) (int, error) {
	w.calls++
	if w.calls == w.shortOn {
		return len(b) - 1, nil
	}
	return len(b), nil
}
func (w *shortLSPWriter) Close() error { return nil }
func TestLSPShortWriteCannotSucceed(t *testing.T) {
	for _, part := range []int{1, 2} {
		t.Run(fmt.Sprint(part), func(t *testing.T) {
			w := &shortLSPWriter{shortOn: part}
			if err := WriteLSPMessage(w, &LSPMessage{Method: "short"}); !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("false success: %v", err)
			}
			w = &shortLSPWriter{shortOn: part}
			c := &LSPClient{stdin: w, pending: make(map[int]chan *LSPMessage)}
			err := c.SendNotification("partial", nil)
			if !errors.Is(err, io.ErrShortWrite) || diagnostic.CodeOf(err) != diagnostic.LSPTransport {
				t.Fatal(err)
			}
			calls := w.calls
			if err = c.SendNotification("must-not-append", nil); diagnostic.CodeOf(err) != diagnostic.LSPTransport {
				t.Fatal(err)
			}
			if w.calls != calls {
				t.Fatal("appended to a partial frame")
			}
			_ = c.Close()
		})
	}
}

func TestLSPNotificationPrecancelledAndEncodeFailureDoNotWrite(t *testing.T) {
	w := &memWriteCloser{Buffer: new(bytes.Buffer)}
	c := &LSPClient{stdin: w}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.SendNotificationContext(ctx, "cancelled", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := c.SendNotification("invalid", make(chan int)); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if w.Len() != 0 {
		t.Fatal("invalid/cancelled frame wrote bytes")
	}
	if err := c.SendNotification("still-usable", nil); err != nil {
		t.Fatal(err)
	}
}

// The helper is a real child process with native stdin/stdout handles, but
// deliberately never reads stdin. No shell/toolchain/server dependency.
func TestLSPTransportChild(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-1] != "silk-stalled-lsp-child" {
		return
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

func TestLSPStalledChildNotificationDeadlineReapsProcess(t *testing.T) {
	c, err := LaunchLSPClient(os.Args[0], "-test.run=^TestLSPTransportChild$", "--", "silk-stalled-lsp-child")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = c.SendNotificationContext(ctx, "textDocument/didChange", strings.Repeat("q", 2<<20))
	if !errors.Is(err, context.DeadlineExceeded) || diagnostic.CodeOf(err) != diagnostic.LSPTransport {
		t.Fatalf("deadline not preserved: %v", err)
	}
	finished := make(chan struct{})
	go func() { _ = c.Close(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not reap stalled child")
	}
	if c.cmd.ProcessState == nil {
		t.Fatal("child was killed without Wait/reaping")
	}
	select {
	case <-c.done:
	default:
		t.Fatal("native read loop did not retire")
	}
	select {
	case _, open := <-c.Notifications():
		if open {
			t.Fatal("notification consumer left alive")
		}
	default:
		t.Fatal("notifications not closed")
	}
}

func TestLSPTerminalErrorSnapshotAndIntentionalClose(t *testing.T) {
	c := &LSPClient{stdin: &shortLSPWriter{shortOn: 1}, pending: make(map[int]chan *LSPMessage)}
	if c.Err() != nil {
		t.Fatal("healthy session has failure")
	}
	_ = c.SendNotification("partial", nil)
	err := c.Err()
	if diagnostic.CodeOf(err) != diagnostic.LSPTransport || !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
	d := err.(*diagnostic.Error)
	d.Message = "caller mutation"
	if c.Err().(*diagnostic.Error).Message == d.Message {
		t.Fatal("SDK exposed mutable terminal error")
	}
	_ = c.Close()
	if diagnostic.CodeOf(c.Err()) != diagnostic.LSPTransport {
		t.Fatal("Close overwrote failure")
	}
	clean := &LSPClient{stdin: &memWriteCloser{Buffer: new(bytes.Buffer)}}
	_ = clean.Close()
	if clean.Err() != nil {
		t.Fatal("intentional Close reported failure")
	}
	stalled, in, out := newPipedClient(t)
	defer in.Close()
	defer out.Close()
	_ = stalled.Close()
	if stalled.Err() != nil {
		t.Fatal("graceful shutdown deadline reported as crash")
	}
}

// A peer can consume the full frame before Write returns its acknowledgement.
// Force that exact scheduling edge rather than relying on a timer race.
type lspDelayedAck struct {
	io.WriteCloser
	consumed, release chan struct{}
	once              sync.Once
}

func (w *lspDelayedAck) Write(p []byte) (int, error) {
	n, e := w.WriteCloser.Write(p)
	if bytes.HasPrefix(p, []byte("{")) && e == nil {
		w.once.Do(func() { close(w.consumed); <-w.release })
	}
	return n, e
}
func TestLSPCancelAfterPeerReadBeforeWriteAckKeepsSession(t *testing.T) {
	c, in, out := newPipedClient(t)
	defer in.Close()
	defer out.Close()
	defer c.Close()
	delay := &lspDelayedAck{WriteCloser: c.stdin, consumed: make(chan struct{}), release: make(chan struct{})}
	c.stdin = delay
	defer func() {
		select {
		case <-delay.release:
		default:
			close(delay.release)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, e := c.SendRequestContext(ctx, "workspace/symbol", nil); result <- e }()
	reader := bufio.NewReader(in)
	req := readWorkspaceFrame(t, reader)
	<-delay.consumed
	cancel()
	select {
	case e := <-result:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled request waited for write acknowledgement")
	}
	close(delay.release)
	m := readWorkspaceFrame(t, reader)
	var p struct {
		ID json.RawMessage `json:"id"`
	}
	if DecodeParams(m, &p) != nil || m.Method != "$/cancelRequest" || string(p.ID) != string(*req.ID) {
		t.Fatalf("cancel after frame lost: %+v", m)
	}
	if c.Err() != nil {
		t.Fatalf("complete frame incorrectly tore down session: %v", c.Err())
	}
	next := make(chan error, 1)
	go func() { _, e := c.SendRequest("still-usable", nil); next <- e }()
	req2 := readWorkspaceFrame(t, reader)
	if e := WriteLSPMessage(out, &LSPMessage{JSONRPC: "2.0", ID: req2.ID, Result: json.RawMessage(`null`)}); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-next:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("subsequent request blocked")
	}
}

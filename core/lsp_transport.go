package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/uk0/silk/diagnostic"
)

const lspCancellationQueueSize = 64
const lspCancellationWriteTimeout = 500 * time.Millisecond

// Lazy initialization also supports the in-package pipe/buffer test clients.
// No goroutine is created per request, waiter, or cancellation notification.
func (c *LSPClient) initTransport() {
	c.writeOnce.Do(func() {
		c.writeJobs = make(chan *lspWriteJob)
		c.writeWorkerDone = make(chan struct{})
		c.writeGate = make(chan struct{}, 1)
		c.writeStopped = make(chan struct{})
		c.cancelIDs = make(chan int, lspCancellationQueueSize)
		c.cancelDone = make(chan struct{})
		go c.runWrites()
	})
}

// Err returns a snapshot of the first unexpected terminal transport failure.
// It is safe on any goroutine. Healthy sessions and explicit Close return nil;
// notifications closure + Err lets hosts report disconnects even when the
// failed request was superseded. The diagnostic is copied, not shared mutable UI state.
func (c *LSPClient) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var d *diagnostic.Error
	if errors.As(c.writeErr, &d) {
		snapshot := *d
		return &snapshot
	}
	return nil
}

func (c *LSPClient) transportError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.writeErr != nil {
		return c.writeErr
	}
	return errors.New("lspclient: closed")
}

// stopWrites never waits for a writer or the read loop. The first terminal
// cause is kept so subsequent actions explain why the session must restart.
func (c *LSPClient) stopWrites(err error) bool {
	c.initTransport()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.writeErr != nil {
		return false
	}
	c.writeErr = err
	close(c.writeStopped)
	return true
}

// A partly written JSON-RPC frame cannot be resumed as a fresh message. Abort
// the session instead of appending another header to its unfinished body.
func (c *LSPClient) abortTransport(cause error) {
	var err error = diagnostic.New(diagnostic.LSPTransport, "Language-server transport interrupted", cause)
	c.mu.Lock()
	closing := c.closed
	c.mu.Unlock()
	if closing {
		err = errors.New("lspclient: closed")
	}
	if !c.stopWrites(err) {
		return
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.stdoutPipe != nil {
		_ = c.stdoutPipe.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	// One terminal cleanup per client, not one goroutine per blocked caller.
	go c.Close()
}

func (c *LSPClient) writeFrame(m *LSPMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
	defer cancel()
	return c.writeFrameContext(ctx, m)
}

// A single writer owns the pipe. Request cancellation must be able to return
// before Write acknowledges a frame already consumed by the peer (io.Pipe and
// native pipes both allow this ordering). A canceled active request therefore
// gets a bounded drain grace, then protocol cancellation after its complete
// frame, or session teardown if the peer still refuses to read.
const lspWriteDrainTimeout = 500 * time.Millisecond

type lspWriteJob struct {
	ctx                                     context.Context
	header                                  string
	body                                    []byte
	requestID                               int
	result                                  chan error
	mu                                      sync.Mutex
	started, finished, canceled, cancelSent bool
	err                                     error
}

func (j *lspWriteJob) requestCancel(c *LSPClient) {
	j.mu.Lock()
	j.canceled = true
	send := j.finished && j.started && j.err == nil && j.requestID != 0 && !j.cancelSent
	if send {
		j.cancelSent = true
	}
	j.mu.Unlock()
	if send {
		c.cancelRequest(j.requestID)
	}
}
func (j *lspWriteJob) finish(c *LSPClient, err error) {
	j.mu.Lock()
	j.finished = true
	j.err = err
	send := j.canceled && j.started && err == nil && j.requestID != 0 && !j.cancelSent
	if send {
		j.cancelSent = true
	}
	j.mu.Unlock()
	if send {
		c.cancelRequest(j.requestID)
	}
	j.result <- err
}

func (c *LSPClient) writeFrameContext(ctx context.Context, m *LSPMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	header, body, err := marshalLSPFrame(m)
	if err != nil {
		return err
	}
	j := &lspWriteJob{ctx: ctx, header: header, body: body, result: make(chan error, 1)}
	if m.ID != nil && m.Method != "" {
		_ = json.Unmarshal(*m.ID, &j.requestID)
	}
	c.initTransport()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.writeStopped:
		return c.transportError()
	case c.writeJobs <- j:
	}
	select {
	case err := <-j.result:
		return err
	case <-c.writeStopped:
		return c.transportError()
	case <-ctx.Done():
		if j.requestID != 0 {
			j.requestCancel(c)
			return ctx.Err()
		}
		// Notifications/replies have no request cancellation protocol. Retire an
		// interrupted active write so a host cannot continue with unknown doc state.
		j.mu.Lock()
		active := j.started && !j.finished
		finished := j.finished
		err := j.err
		j.mu.Unlock()
		if finished {
			return err
		}
		if active {
			c.abortTransport(ctx.Err())
			return c.transportError()
		}
		return ctx.Err()
	}
}

func (c *LSPClient) runWrites() {
	defer close(c.writeWorkerDone)
	for {
		select {
		case <-c.writeStopped:
			return
		case j := <-c.writeJobs:
			c.writeJob(j)
		}
	}
}
func (c *LSPClient) writeJob(j *lspWriteJob) {
	select {
	case <-j.ctx.Done():
		j.finish(c, j.ctx.Err())
		return
	case <-c.writeStopped:
		j.finish(c, c.transportError())
		return
	case c.writeGate <- struct{}{}:
	}
	defer func() { <-c.writeGate }()
	j.mu.Lock()
	if err := j.ctx.Err(); err != nil {
		j.mu.Unlock()
		j.finish(c, err)
		return
	}
	select {
	case <-c.writeStopped:
		j.mu.Unlock()
		j.finish(c, c.transportError())
		return
	default:
	}
	j.started = true
	j.mu.Unlock()
	written, callbackDone := make(chan struct{}), make(chan struct{})
	stop := context.AfterFunc(j.ctx, func() {
		defer close(callbackDone)
		if j.requestID != 0 {
			timer := time.NewTimer(lspWriteDrainTimeout)
			defer timer.Stop()
			select {
			case <-written:
				return
			case <-c.writeStopped:
				return
			case <-timer.C:
			}
		}
		c.abortTransport(j.ctx.Err())
	})
	err := writeLSPFrame(c.stdin, j.header, j.body)
	close(written)
	if !stop() {
		<-callbackDone
	}
	if err != nil {
		c.abortTransport(err)
		err = c.transportError()
	}
	j.finish(c, err)
}

// Best-effort protocol cancellation must not keep the canceled caller alive.
// One bounded worker serializes these small frames with all other messages.
// On saturation redundant cancellation advice is dropped, never didChange or
// another state-bearing notification. A blocked cancellation write has its own
// short deadline and retires the broken session, without per-ID goroutines.
func (c *LSPClient) cancelRequest(id int) {
	c.initTransport()
	select {
	case <-c.writeStopped:
		return
	default:
	}
	c.cancelOnce.Do(func() { go c.runCancellations() })
	select {
	case <-c.writeStopped:
	case c.cancelIDs <- id:
	default:
	}
}
func (c *LSPClient) runCancellations() {
	defer close(c.cancelDone)
	for {
		select {
		case <-c.writeStopped:
			return
		case <-c.done:
			return
		case id := <-c.cancelIDs:
			note, err := NewNotification("$/cancelRequest", map[string]int{"id": id})
			if err != nil {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), lspCancellationWriteTimeout)
			_ = c.writeFrameContext(ctx, note)
			cancel()
		}
	}
}

// SendNotificationContext waits for a complete frame write, not a server reply.
// Sequential successful calls preserve document notification ordering. As for
// requests, cancellation before writing is harmless; interrupted writes retire
// the session with SILK-LSP003 because the peer may have read a partial frame.
func (c *LSPClient) SendNotificationContext(ctx context.Context, method string, params interface{}) error {
	ctx, cancel := context.WithTimeout(ctx, defaultRequestTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if method == "" {
		return errors.New("lspclient: empty method")
	}
	c.mu.Lock()
	closed := c.closed
	transportErr := c.writeErr
	c.mu.Unlock()
	if transportErr != nil {
		return transportErr
	}
	if closed {
		return errors.New("lspclient: closed")
	}
	note, err := NewNotification(method, params)
	if err != nil {
		return fmt.Errorf("lspclient: build notification %s: %w", method, err)
	}
	if err := c.writeFrameContext(ctx, note); err != nil {
		return fmt.Errorf("lspclient: write notification %s: %w", method, err)
	}
	return nil
}

package main

import (
	"context"
	"sync"
	"time"

	"github.com/uk0/silk/core"
)

type lspDocumentTransport interface {
	DidOpen(string, string, string, int) error
	DidChange(string, int, string) error
	DidClose(string) error
	CompletionContext(context.Context, string, int, int) ([]core.LSPCompletionItem, error)
}
type lspSnapshot struct {
	text               string
	version, line, col int
	complete           bool
}

// One worker per open document, one latest snapshot, one in-flight completion.
// No UI objects are read from the worker. All wire operations share wireMu,
// including explicit-command flushes, so didChange can never precede didOpen
// or arrive after didClose. Superseded snapshots are intentionally coalesced.
type lspDocument struct {
	client           lspDocumentTransport
	uri              string
	mu               sync.Mutex
	latest           lspSnapshot
	cancel           context.CancelFunc
	closed           bool
	wireMu           sync.Mutex
	opened           bool
	sent             int
	wake, stop, done chan struct{}
	previous         <-chan struct{}
	deliver          func(lspSnapshot, []core.LSPCompletionItem)
}

func newLSPDocument(c lspDocumentTransport, uri, text string, previous <-chan struct{}, deliver func(lspSnapshot, []core.LSPCompletionItem)) *lspDocument {
	d := &lspDocument{client: c, uri: uri, latest: lspSnapshot{text: text, version: 1}, wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}), previous: previous, deliver: deliver}
	go d.run()
	return d
}
func (d *lspDocument) update(s lspSnapshot) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || s.version <= d.latest.version {
		return
	}
	d.latest = s
	if d.cancel != nil {
		d.cancel()
	}
	select {
	case d.wake <- struct{}{}:
	default:
	}
}
func (d *lspDocument) close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		if d.cancel != nil {
			d.cancel()
		}
		close(d.stop)
	}
}
func (d *lspDocument) sync() (lspSnapshot, error) {
	if d.previous != nil {
		select {
		case <-d.previous:
		case <-d.stop:
			return lspSnapshot{}, context.Canceled
		}
	}
	d.wireMu.Lock()
	defer d.wireMu.Unlock()
	d.mu.Lock()
	s, closed := d.latest, d.closed
	d.mu.Unlock()
	if closed {
		return s, context.Canceled
	}
	if !d.opened {
		if err := d.client.DidOpen(d.uri, "go", s.text, s.version); err != nil {
			return s, err
		}
		d.opened = true
		d.sent = s.version
	} else if s.version > d.sent {
		if err := d.client.DidChange(d.uri, s.version, s.text); err != nil {
			return s, err
		}
		d.sent = s.version
	}
	return s, nil
}
func (d *lspDocument) run() {
	defer close(d.done)
	defer func() {
		d.wireMu.Lock()
		defer d.wireMu.Unlock()
		if d.opened {
			_ = d.client.DidClose(d.uri)
		}
		d.mu.Lock()
		d.latest.text = ""
		d.deliver = nil
		d.mu.Unlock()
	}()
	select {
	case d.wake <- struct{}{}:
	default:
	}
	for {
		select {
		case <-d.stop:
			return
		case <-d.wake:
		}
		timer := time.NewTimer(90 * time.Millisecond)
	quiet:
		for {
			select {
			case <-d.stop:
				timer.Stop()
				return
			case <-d.wake:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(90 * time.Millisecond)
			case <-timer.C:
				break quiet
			}
		}
		s, err := d.sync()
		if err != nil || !s.complete {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		d.mu.Lock()
		if d.closed || d.latest.version != s.version {
			d.mu.Unlock()
			cancel()
			continue
		}
		d.cancel = cancel
		d.mu.Unlock()
		items, err := d.client.CompletionContext(ctx, d.uri, s.line, s.col)
		cancel()
		d.mu.Lock()
		current := !d.closed && d.latest.version == s.version
		deliver := d.deliver
		d.cancel = nil
		d.mu.Unlock()
		if err == nil && current && deliver != nil {
			deliver(s, items)
		}
	}
}

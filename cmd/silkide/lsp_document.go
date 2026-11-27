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

// Optional: SDK/custom transports implementing only full sync remain compatible.
type lspIncrementalDocumentTransport interface {
	DidChangeFrom(string, int, string, string) error
}

type lspSignatureTransport interface {
	SignatureHintContext(context.Context, string, int, int) (core.LSPActiveSignature, error)
}

type lspSnapshot struct {
	text                string
	version, line, col  int
	complete, signature bool
	signatureEpoch      uint64
}

// One worker per open document with one latest snapshot. At most one
// completion and one signature request from that snapshot are in flight.
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
	sentText         string // wireMu-owned, exactly the last successful wire snapshot
	wake, stop, done chan struct{}
	previous         <-chan struct{}
	deliver          func(lspSnapshot, []core.LSPCompletionItem, error)
	deliverSignature func(lspSnapshot, core.LSPActiveSignature)
}

func newLSPDocument(c lspDocumentTransport, uri, text string, previous <-chan struct{}, deliver func(lspSnapshot, []core.LSPCompletionItem, error)) *lspDocument {
	return newLSPDocumentWithSignatures(c, uri, text, previous, deliver, nil)
}

func newLSPDocumentWithSignatures(c lspDocumentTransport, uri, text string, previous <-chan struct{}, deliver func(lspSnapshot, []core.LSPCompletionItem, error), deliverSignature func(lspSnapshot, core.LSPActiveSignature)) *lspDocument {
	d := &lspDocument{client: c, uri: uri, latest: lspSnapshot{text: text, version: 1}, wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}), previous: previous, deliver: deliver, deliverSignature: deliverSignature}
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
		d.sent, d.sentText = s.version, s.text
	} else if s.version > d.sent {
		var err error
		if incremental, ok := d.client.(lspIncrementalDocumentTransport); ok {
			err = incremental.DidChangeFrom(d.uri, s.version, d.sentText, s.text)
		} else {
			err = d.client.DidChange(d.uri, s.version, s.text)
		}
		if err != nil {
			return s, err
		}
		d.sent, d.sentText = s.version, s.text
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
		d.sentText = ""
		d.mu.Lock()
		d.latest.text = ""
		d.deliver = nil
		d.deliverSignature = nil
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
		d.fetch(s, err)
	}
}

// Completion and signature help are independently useful. A slow server
// response for one does not hold back delivery of the other, and a newer
// snapshot cancels both before the worker admits another request.
func (d *lspDocument) fetch(s lspSnapshot, syncErr error) {
	signatureClient, supportsSignature := d.client.(lspSignatureTransport)
	wantSignature := s.signature && supportsSignature && d.deliverSignature != nil
	if !s.complete && !wantSignature {
		return
	}
	if syncErr != nil {
		if s.complete {
			d.finish(s, nil, syncErr)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	d.mu.Lock()
	if d.closed || d.latest.version != s.version {
		d.mu.Unlock()
		cancel()
		return
	}
	d.cancel = cancel
	d.mu.Unlock()
	defer func() {
		cancel()
		d.mu.Lock()
		d.cancel = nil
		d.mu.Unlock()
	}()

	getSignature := func() {
		hint, err := signatureClient.SignatureHintContext(ctx, d.uri, s.line, s.col)
		if err == nil && ctx.Err() == nil && d.current(s) {
			d.deliverSignature(s, hint)
		}
	}
	if !s.complete {
		getSignature()
		return
	}
	var signatureDone chan struct{}
	if wantSignature {
		signatureDone = make(chan struct{})
		go func() {
			defer close(signatureDone)
			getSignature()
		}()
	}
	items, err := d.client.CompletionContext(ctx, d.uri, s.line, s.col)
	d.finish(s, items, err)
	if signatureDone != nil {
		<-signatureDone
	}
}

func (d *lspDocument) current(s lspSnapshot) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.closed && d.latest.version == s.version
}

// Never calls host code while holding mu. Hosts also check editor identity,
// cursor and version on the UI queue before any rendering.
func (d *lspDocument) finish(s lspSnapshot, items []core.LSPCompletionItem, err error) {
	if d.deliver != nil && d.current(s) {
		d.deliver(s, items, err)
	}
}

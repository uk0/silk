package main

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

type incrementalDocumentProbe struct {
	*documentProbe
	lock     sync.Mutex
	baseline []string
	texts    []string
	versions []int
	fail     bool
	entered  chan struct{}
	release  chan struct{}
}

func (p *incrementalDocumentProbe) DidChangeFrom(_ string, v int, previous, text string) error {
	p.lock.Lock()
	p.baseline = append(p.baseline, previous)
	p.texts = append(p.texts, text)
	p.versions = append(p.versions, v)
	fail, entered, release := p.fail, p.entered, p.release
	p.fail = false
	p.entered = nil
	p.release = nil
	p.lock.Unlock()
	if entered != nil {
		close(entered)
		<-release
	}
	if fail {
		return errors.New("probe rejected before writing")
	}
	return nil
}

// These tests drive explicit command flushes deterministically. No automatic
// debounce runner competes for the same snapshot; its lifecycle is covered
// separately below and by the existing burst/cancel/reopen tests.
func syncOnlyLSPDocument(p lspDocumentTransport, text string) *lspDocument {
	return &lspDocument{client: p, uri: "file:///a.go", latest: lspSnapshot{text: text, version: 1}, wake: make(chan struct{}, 1), stop: make(chan struct{})}
}

func TestLSPDocumentIncrementalUsesLastSuccessfulWireSnapshot(t *testing.T) {
	p := &incrementalDocumentProbe{documentProbe: &documentProbe{requests: make(chan int, 10)}}
	d := syncOnlyLSPDocument(p, "initial")
	defer d.close()
	if _, err := d.sync(); err != nil {
		t.Fatal(err)
	}
	for v := 2; v <= 1001; v++ {
		d.update(lspSnapshot{text: "coalesced", version: v})
	}
	if _, err := d.sync(); err != nil {
		t.Fatal(err)
	}
	p.lock.Lock()
	p.fail = true
	p.lock.Unlock()
	d.update(lspSnapshot{text: "rejected", version: 1002})
	if _, err := d.sync(); err == nil {
		t.Fatal("failed notification reported success")
	}
	d.update(lspSnapshot{text: "recovery", version: 1003})
	if _, err := d.sync(); err != nil {
		t.Fatal(err)
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	want := []string{"initial", "coalesced", "coalesced"}
	if strings.Join(p.baseline, "|") != strings.Join(want, "|") || len(p.versions) != 3 || p.versions[0] != 1001 || p.versions[2] != 1003 {
		t.Fatal("local/failed revision became wire baseline", p.baseline, p.versions)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.events {
		if strings.HasPrefix(s, "change:") {
			t.Fatal("optional transport still sent a full update", p.events)
		}
	}
}

func TestLSPDocumentUpdateDuringWireSendKeepsOrderedBaseline(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	p := &incrementalDocumentProbe{documentProbe: &documentProbe{requests: make(chan int, 10)}, entered: entered, release: release}
	d := syncOnlyLSPDocument(p, "old")
	defer d.close()
	if _, err := d.sync(); err != nil {
		t.Fatal(err)
	}
	d.update(lspSnapshot{text: "middle", version: 2})
	done := make(chan error, 1)
	go func() { _, err := d.sync(); done <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("wire send did not start")
	}
	d.update(lspSnapshot{text: "latest", version: 3})
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := d.sync(); err != nil {
		t.Fatal(err)
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	if strings.Join(p.baseline, "|") != "old|middle" || strings.Join(p.texts, "|") != "middle|latest" {
		t.Fatal("in-flight send drifted baseline", p.baseline, p.texts)
	}
}

func TestLSPDocumentCloseReleasesWireTextAndRetainsLegacyFallback(t *testing.T) {
	p := &documentProbe{requests: make(chan int, 10)}
	d := newLSPDocument(p, "file:///a.go", strings.Repeat("large", 10000), nil, nil)
	if _, err := d.sync(); err != nil {
		t.Fatal(err)
	}
	d.update(lspSnapshot{text: "small", version: 2})
	if _, err := d.sync(); err != nil {
		t.Fatal(err)
	}
	d.close()
	waitDocument(t, d)
	d.wireMu.Lock()
	retained := d.sentText
	d.wireMu.Unlock()
	if retained != "" {
		t.Fatal("closed document retained its last wire source")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.events) != 3 || p.events[1] != "change:2:small" || p.events[2] != "close" {
		t.Fatal("legacy full transport changed", p.events)
	}
}

// Compile-time verification: production SDK clients expose the optional path.
var _ lspIncrementalDocumentTransport = (*core.LSPClient)(nil)

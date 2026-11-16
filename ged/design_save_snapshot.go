package ged

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/internal/fileops"
)

var ErrSaveSuperseded = errors.New("design save: captured state or save intent changed")

type designSaveIntent struct{ marker byte }

// PreparedDesignSave is a UI-owned receipt. It retains a live Undo checkpoint;
// DO NOT pass this receipt, its scene or widgets to a worker. Pass Writer() only.
// The host must additionally validate its canvas/workbench/lifetime ownership
// before calling CompleteSave. This SDK receipt does not own a native window.
type PreparedDesignSave struct {
	intent   *designSaveIntent
	previous string
	revision uint64
	history  gui.UndoCheckpoint
	writer   *DesignWriteSnapshot
}

// DesignWriteSnapshot is an independent immutable document/path plus a cached,
// once-only physical write result. It contains no scene, widget or UI callback.
// A bounded worker may retain this object after a UI receipt is retired.
type DesignWriteSnapshot struct {
	path        string
	doc         *core.TDoc
	once        sync.Once
	err         error
	written     atomic.Bool
	fingerprint core.FileFingerprint // published by written's release/acquire
}

func (p *PreparedDesignSave) Writer() *DesignWriteSnapshot { return p.writer }
func (w *DesignWriteSnapshot) Path() string                { return w.path }

// DocumentCopy returns an independent document for companion generation. It
// never exposes the writer's private document: TDoc lookups may build indexes,
// and consumers must not share a mutable tree with the physical encoder.
func (w *DesignWriteSnapshot) DocumentCopy() *core.TDoc { return w.doc.Clone() }

// Write is synchronous and safe for concurrent duplicate delivery: physical I/O
// runs at most once, with the same cached result thereafter. It never marks a
// scene clean. UI retirement/timeout cannot forcibly interrupt blocked OS I/O;
// the host must retain its physical-worker slot until Write really returns.
func (w *DesignWriteSnapshot) Write() error {
	return w.writeWith(func(doc *core.TDoc, path string) error {
		v, err := fileops.WriteAtomicRecorded(path, 0644, func(out io.Writer) error { return doc.Save1(out, false) })
		if err == nil {
			w.fingerprint = v
		}
		return err
	})
}

// WriteAgainst is the once-only writer with an explicit observed disk baseline.
// Like Write, repeated calls share the FIRST physical result, not a new attempt
// or a new comparison. Hosts must not reuse a snapshot to retry another intent.
func (w *DesignWriteSnapshot) WriteAgainst(expected core.FileFingerprint) error {
	return w.writeWith(func(doc *core.TDoc, path string) error {
		version, err := doc.SaveFileAtomicAgainst(path, expected)
		if err == nil {
			w.fingerprint = version
		}
		return err
	})
}

func (w *DesignWriteSnapshot) WrittenFingerprint() (core.FileFingerprint, bool) {
	if !w.written.Load() {
		return core.FileFingerprint{}, false
	}
	return w.fingerprint, w.fingerprint.Known()
}

// A per-call boundary for deterministic tests; production always uses Write.
// Unlike a mutable package hook, it cannot redirect another snapshot's writer.
func (w *DesignWriteSnapshot) writeWith(write func(*core.TDoc, string) error) error {
	w.once.Do(func() {
		w.err = errors.New("design save: physical writer did not return")
		defer func() {
			if p := recover(); p != nil {
				w.err = fmt.Errorf("design save: physical writer panicked: %v", p)
			}
		}()
		w.err = write(w.doc, w.path)
		if w.err == nil {
			w.written.Store(true)
		}
	})
	return w.err
}

// PrepareSave commits pending drafts and captures an independent document on UI.
// It performs no document-file I/O or native path chooser and does not adopt filename or
// a clean marker. Each new prepare supersedes previous receipts, including when
// preparation fails. Authored changes must use commands/NotifyDesignChanged as
// required by the designer change-notification contract; raw widget writes that
// bypass that door cannot be tracked. A worker receives only receipt.Writer().
func (s *GedScene) PrepareSave(filename string) (*PreparedDesignSave, error) {
	if s.saving || s.committingEdits {
		return nil, errors.New("design save: reentrant save during pending edit commit")
	}
	s.saving = true
	defer func() { s.saving = false }()
	return s.prepareSave(filename)
}

func (s *GedScene) prepareSave(filename string) (*PreparedDesignSave, error) {
	intent := new(designSaveIntent)
	s.saveIntent = intent
	if filename == "" {
		return nil, errors.New("design save: empty destination")
	}
	previous, before := s.filename, s.UndoStack()
	stack, ok := before.(*gui.UndoStack)
	if !ok || !stack.Checkpoint().IsCurrent(before) {
		return nil, errors.New("design save: history is executing or cannot be checkpointed")
	}
	if filepath.Ext(filename) == "" {
		filename += DefaultDesignExt
	}
	doc, err := s.SaveDesignWithPendingEdits()
	if err != nil {
		return nil, err
	}
	if s.filename != previous || s.saveIntent != intent || s.UndoStack() != before {
		return nil, ErrSaveSuperseded
	}
	history := stack.Checkpoint()
	if !history.IsCurrent(before) || s.HasPendingEdits() {
		return nil, ErrSaveSuperseded
	}
	return &PreparedDesignSave{intent: intent, previous: previous, revision: s.designRevision,
		history: history, writer: &DesignWriteSnapshot{path: filename, doc: doc}}, nil
}

// CompleteSave never waits. True means the already-written snapshot was adopted
// as this scene's saved state. False may still mean disk was written, but the
// receipt no longer owns the UI state: retain newer edits/path/history. Hosts
// must not resume quit/open/New merely because the worker Write returned nil.
// This consumes a successful receipt exactly once. Like PrepareSave, UI only.
func (s *GedScene) CompleteSave(p *PreparedDesignSave) bool {
	if s.saving || s.committingEdits {
		return false
	}
	s.saving = true
	defer func() { s.saving = false }()
	return s.completeSave(p)
}

func (s *GedScene) completeSave(p *PreparedDesignSave) bool {
	if !s.preparedSaveCurrent(p) || !p.writer.written.Load() {
		return false
	}
	s.saveIntent = nil
	s.SetFilename(p.writer.path)
	s.SetTitle(filepath.Base(p.writer.path))
	s.UndoStack().SetClean()
	return true
}

// PreparedSaveCurrent checks a UI-owned receipt without requiring its writer to
// have run. Hosts use it after capturing additional companion/property values:
// reentrant getters must not pair one document with a later code snapshot.
// Like PrepareSave/CompleteSave this method is UI-only, not a worker predicate.
func (s *GedScene) PreparedSaveCurrent(p *PreparedDesignSave) bool {
	return !s.saving && !s.committingEdits && s.preparedSaveCurrent(p)
}

func (s *GedScene) preparedSaveCurrent(p *PreparedDesignSave) bool {
	return p != nil && p.writer != nil && s.saveIntent == p.intent &&
		s.filename == p.previous && s.designRevision == p.revision &&
		p.history.IsCurrent(s.UndoStack()) && !s.HasPendingEdits()
}

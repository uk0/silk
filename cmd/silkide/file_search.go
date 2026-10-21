package main

import (
	"context"
	"sync"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

const fileFinderLimit = 500

type fileSearchResult struct {
	files      []fileEntry
	total      int
	err        error
	generation uint64
	owner      *fileSearchSession
}
type fileSearchSession struct {
	root, query    string
	ctx            context.Context
	cancel         context.CancelFunc
	rankCancel     context.CancelFunc
	generation     uint64
	changed        time.Time
	dirty, scanned bool
	files          []fileEntry
	scanErr        error
	deliver        func(fileSearchResult)
}

// One service for the application, not a new scan goroutine for every dialog.
// Even a stalled filesystem syscall followed by repeated close/reopen leaves
// at most one running scan and one replaceable current session. Scan results
// live for one dialog only; reopening rescans so external file changes appear.
// All GUI deliveries are coalesced to one queued callback and generation checked.
type fileSearchService struct {
	mu              sync.Mutex
	current         *fileSearchSession
	running, posted bool
	result          *fileSearchResult
	wake            chan struct{}
	debounce        time.Duration
	scan            func(context.Context, string) ([]fileEntry, error)
	rank            func([]fileEntry, string) []fileEntry
	post            func(func())
}

var quickOpenSearch = newFileSearchService(scanFileFinder, rankFiles, gui.Post)

func newFileSearchService(scan func(context.Context, string) ([]fileEntry, error), rank func([]fileEntry, string) []fileEntry, post func(func())) *fileSearchService {
	return &fileSearchService{scan: scan, rank: rank, post: post, wake: make(chan struct{}, 1), debounce: 75 * time.Millisecond}
}
func scanFileFinder(ctx context.Context, root string) ([]fileEntry, error) {
	opts := core.ProjectScanOptions{}
	for name := range fileFinderSkipDirs {
		opts.ExcludeDirs = append(opts.ExcludeDirs, name)
	}
	for ext := range fileFinderSkipExts {
		opts.ExcludeExtensions = append(opts.ExcludeExtensions, ext)
	}
	result, err := core.ScanProjectFiles(ctx, root, opts)
	var files []fileEntry
	for _, f := range result.Files {
		files = append(files, fileEntry{Path: f.Path, Display: f.Relative})
	}
	return files, err
}
func (w *fileSearchService) open(root string, deliver func(fileSearchResult)) *fileSearchSession {
	w.mu.Lock()
	defer w.mu.Unlock()
	if old := w.current; old != nil {
		old.cancel()
		old.deliver = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &fileSearchSession{root: root, ctx: ctx, cancel: cancel, dirty: true, generation: 1, deliver: deliver}
	w.current = s
	w.result = nil
	w.startLocked()
	return s
}
func (w *fileSearchService) startLocked() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
	if !w.running {
		w.running = true
		go w.run()
	}
}
func (w *fileSearchService) update(s *fileSearchSession, query string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.current != s {
		return
	}
	s.query = query
	s.generation++
	s.changed = time.Now()
	s.dirty = true
	w.result = nil
	if s.rankCancel != nil {
		s.rankCancel()
	}
	w.startLocked()
}
func (w *fileSearchService) close(s *fileSearchSession) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.current != s {
		return
	}
	s.cancel()
	s.deliver = nil
	s.files = nil
	w.current = nil
	w.result = nil
}
func (w *fileSearchService) run() {
	for {
		w.mu.Lock()
		s := w.current
		if s == nil {
			w.running = false
			w.mu.Unlock()
			return
		}
		if !s.scanned {
			w.mu.Unlock()
			files, err := w.scan(s.ctx, s.root)
			w.mu.Lock()
			if w.current != s {
				w.mu.Unlock()
				continue
			}
			s.files, s.scanErr, s.scanned = files, err, true
		}
		if !s.dirty {
			w.running = false
			w.mu.Unlock()
			return
		}
		remaining := time.Until(s.changed.Add(w.debounce))
		if remaining > 0 {
			w.mu.Unlock()
			timer := time.NewTimer(remaining)
			select {
			case <-s.ctx.Done():
			case <-w.wake:
			case <-timer.C:
			}
			timer.Stop()
			continue
		}
		query, generation, files, scanErr := s.query, s.generation, s.files, s.scanErr
		ctx, cancel := context.WithCancel(s.ctx)
		s.rankCancel = cancel
		s.dirty = false
		w.mu.Unlock()
		ranked := w.rank(files, query)
		r := fileSearchResult{files: ranked[:min(len(ranked), fileFinderLimit)], total: len(ranked), err: scanErr, generation: generation, owner: s}
		w.mu.Lock()
		valid := w.current == s && s.generation == generation && ctx.Err() == nil
		cancel()
		s.rankCancel = nil
		post := false
		if valid {
			w.result = &r
			post = !w.posted
			w.posted = true
		}
		w.mu.Unlock()
		if post {
			w.post(w.flush)
		}
	}
}
func (w *fileSearchService) flush() {
	w.mu.Lock()
	r := w.result
	w.result = nil
	w.posted = false
	var deliver func(fileSearchResult)
	if r != nil && w.current == r.owner && r.owner.generation == r.generation {
		deliver = r.owner.deliver
	}
	w.mu.Unlock()
	if deliver != nil {
		deliver(*r)
	}
}

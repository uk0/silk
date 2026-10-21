package settings

import (
	"context"

	"github.com/uk0/silk/diagnostic"
)

// syncFlight's result is published by closing done. There is at most one
// worker per Settings instance; requests arriving during I/O coalesce.
type syncFlight struct {
	done chan struct{}
	err  error
}

// RequestSync schedules pending settings without waiting for encoding or disk
// I/O. It does not load an untouched store. A worker snapshots the latest
// in-memory state; subsequent requests coalesce rather than spawning workers.
// Failures are available through Status and Flush. A failed write is not retried
// in a loop: call RequestSync/Sync/Flush again after addressing the cause.
func (s *Settings) RequestSync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.requestSyncLocked()
	return err
}

// Sync preserves the synchronous SDK contract: success means the actual writer
// has finished. It does not hold the settings mutex during disk I/O. Like first
// Value/SetValue access, initial lazy loading is synchronous; preload before
// entering latency-sensitive UI callbacks.
func (s *Settings) Sync() error { return s.Flush(context.Background()) }

// Flush requests a save and joins the current worker including coalesced
// requests. Call this after stopping producers at shutdown. A context deadline
// ends only the wait, NOT filesystem I/O: the worker may still commit later.
// Mutations without a corresponding save request remain dirty for a later save.
func (s *Settings) Flush(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return s.waitError(err)
	}
	s.mu.Lock()
	s.loadOnce()
	f, err := s.requestSyncLocked()
	s.mu.Unlock()
	if err != nil || f == nil {
		return err
	}
	select {
	case <-f.done:
		return f.err
	default:
	}
	select {
	case <-f.done:
		return f.err
	case <-ctx.Done():
		return s.waitError(ctx.Err())
	}
}

func (s *Settings) waitError(err error) error {
	return diagnostic.New(diagnostic.SettingsWait, "settings flush wait ended", err).At(diagnostic.Location{File: s.Path()})
}

func (s *Settings) requestSyncLocked() (*syncFlight, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if s.path == "" {
		s.dirty = false
		s.lastErr = nil
		return nil, nil
	}
	if !s.dirty {
		return s.flight, nil
	}
	s.pending = true
	if s.flight == nil {
		s.flight = &syncFlight{done: make(chan struct{})}
		go s.runWriter(s.flight)
	}
	return s.flight, nil
}

func (s *Settings) runWriter(f *syncFlight) {
	for {
		s.mu.Lock()
		snapshot, version, path := s.doc.Clone(), s.version, s.path
		write := s.writeSnapshot
		s.pending = false
		s.mu.Unlock()
		if write == nil {
			write = writeSettingsSnapshot
		}
		err := write(path, snapshot)
		if err != nil {
			err = diagnostic.New(diagnostic.SettingsWrite, "cannot persist settings", err).At(diagnostic.Location{File: path})
		}
		s.mu.Lock()
		s.lastErr = err
		if err == nil && s.version == version {
			s.dirty = false
		}
		if err == nil && s.pending && s.dirty {
			s.mu.Unlock()
			continue
		}
		s.pending = false
		f.err = err
		s.flight = nil
		close(f.done) // closed only after the real write/close/replace returned
		s.mu.Unlock()
		return
	}
}

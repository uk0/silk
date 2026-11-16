// Package hotreload bridges silk/fswatch with silk/decl so designer-
// authored .silkui files can be edited at runtime and trigger an
// in-process widget tree rebuild without restarting the host process.
//
// Architecture:
//
//  1. Caller registers a path via Reloader.Watch(path).
//  2. fswatch.Watcher polls the path and emits Modified events.
//  3. The reload loop reads the event, re-parses the file via
//     core.LoadTDocFile + decl.FromTDoc, and dispatches a Reload
//     callback carrying the fresh *decl.Node tree.
//  4. The host application's callback is responsible for the actual
//     widget swap — typically via (*decl.Node).Build() and an
//     app-specific Replace() that rewires parent/children.
//
// New produces a fresh AST. NewInvalidations instead reports only paths, for
// hosts that must capture UI-owned edit/lifetime authority before reading.
// The package deliberately stops before widget mutation — performing
// the widget tree swap is host policy (some apps want a full re-Build,
// others a smarter diff that preserves scroll positions / focused
// fields).
//
// Concurrency contract: Reload/error callbacks run serially on one bounded
// worker, NOT the GLFW main thread. Hosts that touch GLFW state
// inside the callback must marshal the work onto the main thread (e.g.
// via gui.RunOnMain or a channel they drain in their event loop). The
// package documentation calls this out so users don't run a Build()
// off-thread and crash on first GL call.
//
// Debouncing: rapid successive writes (editor save batches, atomic
// rename + write) are coalesced inside a small debounce window
// (default 100ms). Without this every Vim-style save would fire two
// reloads (one for the temp file rename, one for the actual write).
package hotreload

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/fswatch"
)

// ReloadFunc is the host callback invoked when a watched file
// successfully re-parses. The first argument is the path that triggered
// the reload; the second is the fresh AST. The host owns deciding how
// to apply it (full rebuild, partial diff, etc.).
//
// If the host returns an error, it is forwarded to the optional
// ErrorFunc so users can surface reload failures in their UI without
// stopping the watcher.
type ReloadFunc func(path string, tree *decl.Node) error

// InvalidationFunc reports a changed path without reading or parsing its data.
// Like ReloadFunc it runs on the serialized worker, never on the UI thread.
// Hosts use this when preparation must follow UI-owned admission (for example,
// capture document/edit identity before an asynchronous read). A notification
// is not a snapshot or permission to replace a host's current document.
type InvalidationFunc func(path string) error

// ErrorFunc reports current parse/load/host-callback failures on the same
// serialized worker. Superseded/unwatched/closed input failures are retired.
// ErrPendingLimit uses an empty path and aggregates rejected events. A hook
// panic is logged and contained; returning has no side effect on later jobs.
type ErrorFunc func(path string, err error)

// Options configures a Reloader at construction time. Zero values
// behave reasonably: Debounce defaults to 100ms; PollInterval defaults
// to fswatch.New()'s 500ms.
type Options struct {
	// MaxPending bounds distinct queued/active paths. Default 1024. Existing
	// paths coalesce to their latest event even at capacity. Overflow increments
	// Stats.Rejected and reports ErrPendingLimit through serialized onError.
	MaxPending int

	// Debounce coalesces rapid Modified events on the same path into a
	// single rebuild. Useful for editors that write atomically (rename
	// temp + chmod + replace) and produce a burst of fs events.
	Debounce time.Duration

	// PollInterval overrides the underlying fswatch poll cadence.
	PollInterval time.Duration

	// AllowedExt restricts which file extensions trigger a rebuild.
	// Empty means "all files at registered paths". When set, only
	// files whose extension matches one of the entries (case-
	// insensitive, leading '.' optional) are reloaded — useful when
	// a host watches a directory containing both .silkui and unrelated
	// scratch files.
	AllowedExt []string
}

// Reloader is the entry point. Construct via New or NewInvalidations, register paths via
// Watch, and Stop on shutdown.
//
// Reloader is safe for concurrent calls to Watch/Unwatch/Stop, but the
// callbacks run on a single internal goroutine — host callback code
// does not need its own locking *between* reload calls.
type Reloader struct {
	w            *fswatch.Watcher
	onReload     ReloadFunc
	onInvalidate InvalidationFunc
	onError      ErrorFunc
	debounce     time.Duration
	allowedExt   map[string]struct{}
	limit        int
	load         func(string) (*decl.Node, error)

	mu                       sync.Mutex
	closed                   bool
	live                     int
	busy                     bool
	sequence                 uint64
	pending                  map[string]*reloadEntry
	roots                    map[string]*watchRequest
	rejected, reportRejected uint64
	reportDue                time.Time
	stop, done, wake         chan struct{}
	jobs                     chan reloadJob
}

// Stats is a memory-only snapshot. Pending includes the active path; Active
// means a serialized worker job is admitted, not that shutdown has completed.
type Stats struct {
	Pending        int
	Active, Closed bool
	Rejected       uint64
}

func (r *Reloader) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Stats{len(r.pending), r.busy, r.closed, r.rejected}
}

var ErrPendingLimit = errors.New("hotreload: pending path limit reached")

// New constructs a Reloader with the given handlers and options. onReload
// is required; onError may be nil (failures are silently swallowed in
// that case).
func New(onReload ReloadFunc, onError ErrorFunc, opts Options) (*Reloader, error) {
	if onReload == nil {
		return nil, errors.New("hotreload.New: onReload is required")
	}
	return newReloader(onReload, nil, onError, opts)
}

// NewInvalidations retains New's debouncing, bounded coalescing and retirement
// contracts, but does not open, decode or build an AST for changed files.
// Watch still registers a filesystem baseline; WatchFileState can reuse one
// already captured by a host's background reader. Hosts must revoke any work
// they have marshaled after callback return when their own UI owner retires.
func NewInvalidations(onChange InvalidationFunc, onError ErrorFunc, opts Options) (*Reloader, error) {
	if onChange == nil {
		return nil, errors.New("hotreload.NewInvalidations: onChange is required")
	}
	return newReloader(nil, onChange, onError, opts)
}

func newReloader(onReload ReloadFunc, onInvalidate InvalidationFunc, onError ErrorFunc, opts Options) (*Reloader, error) {
	if opts.MaxPending < 0 {
		return nil, errors.New("hotreload.New: negative MaxPending")
	}
	limit := opts.MaxPending
	if limit == 0 {
		limit = 1024
	}
	w := fswatch.New()
	if opts.PollInterval > 0 {
		w.SetPollInterval(opts.PollInterval)
	}
	debounce := opts.Debounce
	if debounce <= 0 {
		debounce = 100 * time.Millisecond
	}
	allowed := make(map[string]struct{})
	for _, ext := range opts.AllowedExt {
		allowed[normaliseExt(ext)] = struct{}{}
	}
	r := &Reloader{w: w, onReload: onReload, onInvalidate: onInvalidate, onError: onError, debounce: debounce, allowedExt: allowed, limit: limit,
		load: loadReloadTree, pending: make(map[string]*reloadEntry), roots: make(map[string]*watchRequest),
		stop: make(chan struct{}), done: make(chan struct{}), wake: make(chan struct{}, 1), jobs: make(chan reloadJob, 1), live: 2}
	go r.loop()
	go r.worker()
	return r, nil
}

// Watch registers a path with the underlying watcher. The path may be
// a file or a directory; directory watches fire events for immediate
// children. Returns the underlying fswatch.AddPath error verbatim so
// callers can react to permission / not-found problems.
func (r *Reloader) Watch(path string) error {
	return r.watch(path, func(abs string) error { return r.w.AddPath(abs) })
}

// WatchFileState registers a worker-captured regular-file baseline without
// synchronous filesystem I/O. The first poll still detects intervening edits.
func (r *Reloader) WatchFileState(path string, modified time.Time, size int64) error {
	return r.watch(path, func(abs string) error { return r.w.AddFileState(abs, modified, size) })
}

// Unwatch retires the root and its pending file/directory work. Already admitted
// callbacks may finish; blocked reads cannot publish after the registration is
// removed or replaced, including unwatch/re-watch of the same path.
func (r *Reloader) Unwatch(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	r.mu.Lock()
	root := r.roots[abs]
	delete(r.roots, abs)
	if root != nil {
		finishWatch(root, fswatch.ErrSuperseded)
		for p, e := range r.pending {
			if e.root == root {
				delete(r.pending, p)
			}
		}
	}
	r.w.RemovePath(abs)
	r.mu.Unlock()
	r.signal()
}

// Close permanently revokes new callback admission and registration, without
// joining OS I/O or an already admitted host callback. Safe from a callback.
// UI hosts must also retire callbacks they have already marshaled to their UI.
func (r *Reloader) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	close(r.stop)
	for _, root := range r.roots {
		finishWatch(root, fswatch.ErrClosed)
	}
	r.roots = nil
	r.pending = nil
	r.reportRejected = 0
	// A queued, not yet received job need not retain its source after Close.
	select {
	case <-r.jobs:
		r.busy = false
	default:
	}
	r.w.Close()
	r.mu.Unlock()
}

// Wait proves that the event loop, worker/callback and watcher poller physically
// ended. It does not cancel on timeout and cannot forcibly stop an OS syscall.
// Call Close first, and never wait/Stop from this reloader's own callback.
func (r *Reloader) Wait(ctx context.Context) error {
	if ctx == nil {
		return errors.New("hotreload: nil wait context")
	}
	select {
	case <-r.done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return r.w.Wait(ctx)
}

// Stop is Close plus an unbounded physical join. Concurrent calls all join the
// same lifetime. Interactive hosts should Close then Wait off-loop with a bound.
func (r *Reloader) Stop() { r.Close(); _ = r.Wait(context.Background()) }

func loadReloadTree(path string) (*decl.Node, error) {
	doc, err := core.LoadTDocFile(path)
	if err != nil {
		return nil, err
	}
	return decl.FromTDoc(doc)
}

// extAllowed returns true when r.allowedExt is empty (no filter) or
// path's extension matches one of the configured entries.
func (r *Reloader) extAllowed(path string) bool {
	if len(r.allowedExt) == 0 {
		return true
	}
	ext := normaliseExt(filepath.Ext(path))
	_, ok := r.allowedExt[ext]
	return ok
}

// normaliseExt lower-cases and strips a leading '.' so users can pass
// either ".silkui" or "silkui" or "SILKUI" interchangeably.
func normaliseExt(ext string) string {
	ext = strings.ToLower(ext)
	if strings.HasPrefix(ext, ".") {
		return ext[1:]
	}
	return ext
}

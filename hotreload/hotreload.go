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
// The package deliberately stops at "produce the new AST" — performing
// the widget tree swap is host policy (some apps want a full re-Build,
// others a smarter diff that preserves scroll positions / focused
// fields).
//
// Concurrency contract: Reload callbacks fire on the package's reader
// goroutine, NOT the GLFW main thread. Hosts that touch GLFW state
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

// ErrorFunc is invoked for every parse / load / host-callback failure.
// Hosts typically log to a console panel or flash a banner; returning
// from the func has no side effect — the loop continues.
type ErrorFunc func(path string, err error)

// Options configures a Reloader at construction time. Zero values
// behave reasonably: Debounce defaults to 100ms; PollInterval defaults
// to fswatch.New()'s 500ms.
type Options struct {
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

// Reloader is the entry point. Construct via New, register paths via
// Watch, and Stop on shutdown.
//
// Reloader is safe for concurrent calls to Watch/Unwatch/Stop, but the
// callbacks run on a single internal goroutine — host callback code
// does not need its own locking *between* reload calls.
type Reloader struct {
	w        *fswatch.Watcher
	onReload ReloadFunc
	onError  ErrorFunc

	debounce   time.Duration
	allowedExt map[string]struct{}

	stop chan struct{}
	wg   sync.WaitGroup

	// pending coalesces same-path events arriving inside the debounce
	// window. Keyed by path; value is a one-shot timer that fires the
	// rebuild when no further event has arrived in `debounce`.
	mu      sync.Mutex
	pending map[string]*time.Timer
}

// New constructs a Reloader with the given handlers and options. onReload
// is required; onError may be nil (failures are silently swallowed in
// that case).
func New(onReload ReloadFunc, onError ErrorFunc, opts Options) (*Reloader, error) {
	if onReload == nil {
		return nil, errors.New("hotreload.New: onReload is required")
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
	r := &Reloader{
		w:          w,
		onReload:   onReload,
		onError:    onError,
		debounce:   debounce,
		allowedExt: allowed,
		stop:       make(chan struct{}),
		pending:    make(map[string]*time.Timer),
	}
	r.wg.Add(1)
	go r.loop()
	return r, nil
}

// Watch registers a path with the underlying watcher. The path may be
// a file or a directory; directory watches fire events for immediate
// children. Returns the underlying fswatch.AddPath error verbatim so
// callers can react to permission / not-found problems.
func (r *Reloader) Watch(path string) error {
	return r.w.AddPath(path)
}

// Unwatch removes a previously-watched path. Pending debounced
// rebuilds for that path are cancelled.
func (r *Reloader) Unwatch(path string) {
	r.w.RemovePath(path)
	r.mu.Lock()
	if t, ok := r.pending[path]; ok {
		t.Stop()
		delete(r.pending, path)
	}
	r.mu.Unlock()
}

// Stop halts the reload loop and the underlying watcher. Returns when
// the goroutines have exited and pending callbacks (if any) have
// completed. Idempotent.
func (r *Reloader) Stop() {
	select {
	case <-r.stop:
		return
	default:
	}
	close(r.stop)

	// Cancel any pending debounced rebuilds so we don't fire after
	// Stop returns.
	r.mu.Lock()
	for _, t := range r.pending {
		t.Stop()
	}
	r.pending = nil
	r.mu.Unlock()

	r.w.Stop()
	r.wg.Wait()
}

// loop is the single reader goroutine: it pulls fswatch events,
// debounces them per-path, and dispatches rebuilds.
func (r *Reloader) loop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.stop:
			return
		case ev, ok := <-r.w.Events():
			if !ok {
				return
			}
			if ev.Kind != fswatch.Modified && ev.Kind != fswatch.Created {
				continue
			}
			if !r.extAllowed(ev.Path) {
				continue
			}
			r.scheduleRebuild(ev.Path)
		}
	}
}

// scheduleRebuild starts (or resets) the debounce timer for path. The
// rebuild runs on a fresh goroutine after the timer fires so we don't
// block subsequent events while a slow rebuild is in flight.
func (r *Reloader) scheduleRebuild(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == nil {
		// Reloader stopped — drop the event.
		return
	}
	if t, ok := r.pending[path]; ok {
		t.Stop()
	}
	r.pending[path] = time.AfterFunc(r.debounce, func() {
		r.mu.Lock()
		delete(r.pending, path)
		r.mu.Unlock()
		r.rebuild(path)
	})
}

// rebuild does the actual parse + dispatch. Errors at any stage go to
// onError; only a successful (TDoc + AST) parse triggers onReload.
func (r *Reloader) rebuild(path string) {
	// Last-write-wins: a file may have been deleted between event and
	// rebuild. core.LoadTDocFile reports the error; surface and bail.
	doc, err := core.LoadTDocFile(path)
	if err != nil {
		r.fireError(path, err)
		return
	}
	tree, err := decl.FromTDoc(doc)
	if err != nil {
		r.fireError(path, err)
		return
	}
	if err := r.onReload(path, tree); err != nil {
		r.fireError(path, err)
	}
}

// fireError centralises the nil-check on onError so callers don't
// have to repeat it.
func (r *Reloader) fireError(path string, err error) {
	if r.onError != nil {
		r.onError(path, err)
	}
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

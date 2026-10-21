package gui

import (
	"container/list"
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/internal/fileops"
)

// One application-wide scheduler, at most two subprocess-bearing jobs, one
// latest request per editor, and one queued GUI delivery for the entire service.
// No timers/polling/permanent goroutines when idle. Cancellation never starts a
// replacement worker before the old query has actually returned.
type gitStatusService struct {
	mu             sync.Mutex
	queue          list.List
	workers, limit int
	scan           func(context.Context, string) (map[int]GitLineStatus, error)
	post           func(func())
	deliveries     map[*gitStatusSlot]gitStatusResult
	posted         bool
	idle           chan struct{}
	active         map[*gitStatusSlot]bool
	stopping       bool
	held           map[*gitStatusSlot]bool
	suspensions    map[*GitPathSuspension]string
}
type gitStatusSlot struct {
	service *gitStatusService
	apply   func(string, map[int]GitLineStatus, error)
	latest  *gitStatusJob // all fields below owned by service.mu
	running *gitStatusJob
	queued  *list.Element
	retired bool
}
type gitStatusJob struct {
	path   string
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{} // created when scan starts, closed only after it returns
}
type gitStatusResult struct {
	job    *gitStatusJob
	status map[int]GitLineStatus
	err    error
}

func newGitStatusService(limit int, scan func(context.Context, string) (map[int]GitLineStatus, error), post func(func())) *gitStatusService {
	idle := make(chan struct{})
	close(idle)
	return &gitStatusService{limit: max(1, limit), scan: scan, post: post, deliveries: map[*gitStatusSlot]gitStatusResult{}, idle: idle, active: map[*gitStatusSlot]bool{}, held: map[*gitStatusSlot]bool{}, suspensions: map[*GitPathSuspension]string{}}
}

var editorGitService = newGitStatusService(2, GitDiffContext, Post)

func (s *gitStatusService) subscribe(apply func(string, map[int]GitLineStatus, error)) *gitStatusSlot {
	return &gitStatusSlot{service: s, apply: apply}
}
func (slot *gitStatusSlot) request(path string) bool {
	s := slot.service
	s.mu.Lock()
	defer s.mu.Unlock()
	if slot.retired || s.stopping {
		return false
	}
	if slot.latest != nil {
		slot.latest.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	slot.latest = &gitStatusJob{path: path, ctx: ctx, cancel: cancel}
	delete(s.deliveries, slot)
	s.enqueueLocked(slot)
	s.startLocked()
	return true
}
func (slot *gitStatusSlot) close() {
	s := slot.service
	s.mu.Lock()
	defer s.mu.Unlock()
	slot.retired = true
	if slot.latest != nil {
		slot.latest.cancel()
		slot.latest = nil
	}
	if slot.running != nil {
		slot.running.cancel()
	}
	if slot.queued != nil {
		s.queue.Remove(slot.queued)
		slot.queued = nil
	}
	delete(s.deliveries, slot)
	delete(s.held, slot)
}
func (s *gitStatusService) startLocked() {
	n := min(s.limit-s.workers, s.queue.Len())
	if n > 0 && s.workers == 0 {
		s.idle = make(chan struct{})
	}
	for range n {
		s.workers++
		go s.run()
	}
}
func (s *gitStatusService) run() {
	for {
		s.mu.Lock()
		front := s.queue.Front()
		if front == nil {
			s.workers--
			if s.workers == 0 {
				close(s.idle)
			}
			s.mu.Unlock()
			return
		}
		slot := front.Value.(*gitStatusSlot)
		s.queue.Remove(front)
		slot.queued = nil
		job := slot.latest
		slot.running = job
		job.done = make(chan struct{})
		s.active[slot] = true
		s.mu.Unlock()
		status, err := s.scan(job.ctx, job.path)
		job.cancel()
		s.mu.Lock()
		slot.running = nil
		close(job.done)
		delete(s.active, slot)
		post := false
		if !slot.retired && slot.latest == job {
			s.deliveries[slot] = gitStatusResult{job, status, err}
			if !s.posted {
				s.posted = true
				post = true
			}
		} else {
			s.enqueueLocked(slot)
		}
		s.mu.Unlock()
		if post {
			s.post(s.drain)
		}
	}
}

// Called only by the host UI dispatcher. Apply may close/rebind another editor;
// recheck each result immediately before calling it, not just once per batch.
func (s *gitStatusService) drain() {
	s.mu.Lock()
	batch := s.deliveries
	s.deliveries = map[*gitStatusSlot]gitStatusResult{}
	s.posted = false
	s.mu.Unlock()
	for slot, result := range batch {
		s.mu.Lock()
		valid := !s.stopping && !slot.retired && slot.latest == result.job
		if valid && s.pathHeldLocked(result.job.path) {
			// A previous callback in this detached batch may have reserved a path.
			// Its already-snapshotted results still need to honor that reservation.
			ctx, cancel := context.WithCancel(context.Background())
			slot.latest = &gitStatusJob{path: result.job.path, ctx: ctx, cancel: cancel}
			s.enqueueLocked(slot)
			valid = false
		}
		s.mu.Unlock()
		if valid {
			slot.apply(result.job.path, result.status, result.err)
		}
	}
}

// RefreshGitStatus schedules a saved-file diff; never runs Git or waits on I/O
// on the UI thread. Results from older paths/requests or canceled editors cannot
// repaint the current gutter. Errors clear markers instead of leaving stale data.
func (e *CodeEditor) RefreshGitStatus() {
	if e.filePath == "" {
		e.CancelGitStatus()
		e.gitStatus = nil
		e.gitErr = nil
		e.Update()
		return
	}
	if e.gitSlot == nil {
		service := e.gitService
		if service == nil {
			service = editorGitService
		}
		var slot *gitStatusSlot
		slot = service.subscribe(func(path string, status map[int]GitLineStatus, err error) {
			if e.gitSlot != slot || e.filePath != path {
				return
			}
			e.gitLoading = false
			e.gitErr = err
			if err != nil {
				status = nil
			}
			e.gitStatus = status
			e.Update()
			if e.cbGitStatusChanged != nil {
				e.cbGitStatusChanged()
			}
		})
		e.gitSlot = slot
	}
	e.gitErr = nil
	e.gitLoading = e.gitSlot.request(e.filePath)
}

// CancelGitStatus retires pending work/results without waiting for a subprocess.
// Hosts must call it when permanently removing an editor or before event-loop
// shutdown. A later RefreshGitStatus or SetFilePath starts a fresh subscription.
func (e *CodeEditor) CancelGitStatus() {
	if e.gitSlot != nil {
		e.gitSlot.close()
		e.gitSlot = nil
	}
	e.gitLoading = false
}
func (e *CodeEditor) GitStatusLoading() bool { return e.gitLoading }
func (e *CodeEditor) GitStatusError() error {
	if d, ok := e.gitErr.(*diagnostic.Error); ok {
		copy := *d
		return &copy
	}
	return e.gitErr
}

// SigGitStatusChanged runs on the UI thread after the newest accepted result,
// including successful empty results and errors. Cancellation itself is silent.
func (e *CodeEditor) SigGitStatusChanged(fn func()) { e.cbGitStatusChanged = fn }

// ShutdownEditorGit permanently stops the process-wide gutter service. Call
// after the GUI event loop returns, not from an input/close handler. Cancellation
// alone does not mean a process is reaped: this waits for actual worker exit.
// It is idempotent. The context bounds waiting on uninterruptible OS startup.
func ShutdownEditorGit(ctx context.Context) error { return editorGitService.shutdown(ctx) }
func (s *gitStatusService) shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.stopping = true
	retire := func(slot *gitStatusSlot) {
		slot.retired = true
		if slot.latest != nil {
			slot.latest.cancel()
			slot.latest = nil
		}
		if slot.running != nil {
			slot.running.cancel()
		}
		slot.queued = nil
	}
	for element := s.queue.Front(); element != nil; element = element.Next() {
		retire(element.Value.(*gitStatusSlot))
	}
	s.queue.Init()
	for slot := range s.active {
		retire(slot)
	}
	for slot := range s.deliveries {
		retire(slot)
	}
	for slot := range s.held {
		retire(slot)
	}
	s.held = map[*gitStatusSlot]bool{}
	s.suspensions = map[*GitPathSuspension]string{}
	s.deliveries = map[*gitStatusSlot]gitStatusResult{}
	idle := s.idle
	s.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GitPathSuspension protects a path and its descendants from new editor-gutter
// Git scans. Wait joins previously active scans, including retired/closed editors;
// canceling their context alone does not release a Windows child process's CWD.
// Keep the suspension until the filesystem transaction and UI path migration have
// finished, then Release. Wait and Release are safe from background goroutines;
// Release is idempotent. Lexical paths only: aliases/symlinks are not resolved.
type GitPathSuspension struct {
	service *gitStatusService
	pending []<-chan struct{}
}

// SuspendEditorGitPath immediately prevents matching work from starting and
// cancels active matching jobs. It does not wait or globally stop the service.
// Acquire on the UI thread to serialize with result callbacks; call Wait off the
// UI thread before mutating the filesystem. Requests during
// suspension coalesce to each live editor's latest request; unrelated paths run
// normally. Release resumes pending work after the host has rebound/closed paths.
func SuspendEditorGitPath(path string) (*GitPathSuspension, error) {
	return editorGitService.suspendPath(path)
}

func (s *gitStatusService) suspendPath(path string) (*GitPathSuspension, error) {
	if strings.TrimSpace(path) == "" {
		return nil, diagnostic.New(diagnostic.FileName, "cannot suspend an empty Git path", nil)
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return nil, diagnostic.New(diagnostic.FileName, "invalid Git suspension path", err)
	}
	token := &GitPathSuspension{service: s}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suspensions[token] = filepath.Clean(root)
	for slot := range s.active {
		job := slot.running
		if !gitPathWithin(job.path, root) {
			continue
		}
		token.pending = append(token.pending, job.done)
		job.cancel()
		if !slot.retired && slot.latest == job {
			// Do not deliver the cancellation as an editor failure, nor resume a
			// canceled context. The fresh latest request stays held until release.
			ctx, cancel := context.WithCancel(context.Background())
			slot.latest = &gitStatusJob{path: job.path, ctx: ctx, cancel: cancel}
		}
	}
	for slot, result := range s.deliveries {
		if !gitPathWithin(result.job.path, root) {
			continue
		}
		delete(s.deliveries, slot)
		if !slot.retired && slot.latest == result.job {
			ctx, cancel := context.WithCancel(context.Background())
			slot.latest = &gitStatusJob{path: result.job.path, ctx: ctx, cancel: cancel}
			s.enqueueLocked(slot)
		}
	}
	for el := s.queue.Front(); el != nil; {
		next := el.Next()
		s.enqueueLocked(el.Value.(*gitStatusSlot))
		el = next
	}
	return token, nil
}

func gitPathWithin(path, root string) bool {
	if path == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	_, ok := fileops.RebasePath(abs, root, root)
	return ok
}

// Move pending requests between the runnable and held sets in O(1), apart from
// the usually single path-reservation check. Held work never occupies a worker.
func (s *gitStatusService) enqueueLocked(slot *gitStatusSlot) {
	if slot.retired || s.stopping || slot.running != nil || slot.latest == nil {
		return
	}
	if s.pathHeldLocked(slot.latest.path) {
		if slot.queued != nil {
			s.queue.Remove(slot.queued)
			slot.queued = nil
		}
		s.held[slot] = true
		return
	}
	delete(s.held, slot)
	if slot.queued == nil {
		slot.queued = s.queue.PushBack(slot)
	}
}

func (s *gitStatusService) pathHeldLocked(path string) bool {
	for _, root := range s.suspensions {
		if gitPathWithin(path, root) {
			return true
		}
	}
	return false
}

func (p *GitPathSuspension) Wait(ctx context.Context) error {
	for _, done := range p.pending {
		// Already-reaped jobs need no waiting, even if the caller canceled late.
		select {
		case <-done:
			continue
		default:
		}
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (p *GitPathSuspension) Release() {
	s := p.service
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, held := s.suspensions[p]; !held {
		return
	}
	delete(s.suspensions, p)
	for slot := range s.held {
		s.enqueueLocked(slot)
	}
	s.startLocked()
}

package main

import (
	"context"
	"errors"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

// UI-thread controller: one running query and one latest pending request.
// Workers only query and post; every field below is owned by UI dispatch.
// No timer/polling goroutine; a missing tool/non-repo waits for explicit Retry.
type gitPaneRefresh[T any] struct {
	query                                           func(context.Context, string) (T, error)
	post                                            func(func())
	loading                                         func()
	apply                                           func(string, T, error)
	key                                             string
	seq                                             uint64
	running, pending, forcePending, blocked, closed bool
	cancel                                          context.CancelFunc
	done                                            <-chan struct{}
}

func (s *gitPaneRefresh[T]) request(key string, force bool) {
	if s.closed || key == "" {
		return
	}
	if key != s.key {
		s.key = key
		s.blocked = false
		if s.cancel != nil {
			s.cancel()
		}
	}
	if s.blocked && !force {
		return
	}
	s.blocked = false
	s.seq++
	s.pending = true
	s.forcePending = s.forcePending || force
	s.loading()
	s.start()
}
func (s *gitPaneRefresh[T]) start() {
	if s.closed || s.running || !s.pending {
		return
	}
	key, seq := s.key, s.seq
	s.running = true
	s.pending = false
	s.forcePending = false
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	s.cancel = cancel
	done := make(chan struct{})
	s.done = done
	go func() {
		defer close(done)
		result, err := s.query(ctx, key)
		cancel()
		s.post(func() {
			if s.closed {
				return
			}
			s.running = false
			s.cancel = nil
			terminal := errors.Is(err, core.ErrGitUnavailable) || errors.Is(err, core.ErrGitNotRepository)
			if key == s.key {
				// Availability failure applies across same-root automatic save bursts.
				// An explicit Retry made while running retains one fresh attempt.
				if terminal && !s.forcePending {
					s.pending = false
					s.blocked = true
					s.apply(key, result, err)
				} else if seq == s.seq {
					s.apply(key, result, err)
				}
			}
			s.start()
		})
	}()
}
func (s *gitPaneRefresh[T]) close() {
	if s == nil {
		return
	}
	s.closed = true
	s.pending = false
	if s.cancel != nil {
		s.cancel()
	}
}
func (s *gitPaneRefresh[T]) wait(ctx context.Context) error {
	if s == nil || s.done == nil {
		return nil
	}
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type gitChangesSnapshot struct {
	root    string
	entries []core.GitStatusEntry
}
type gitHistorySnapshot struct {
	root    string
	commits []core.GitCommit
}

var changesRefresh *gitPaneRefresh[gitChangesSnapshot]
var historyRefresh *gitPaneRefresh[gitHistorySnapshot]
var changesRefreshOwner *ged.GitChangesPanel
var historyRefreshOwner *ged.GitLogPanel
var gitChangesRoot, gitHistoryRoot string

func gitPanelFailure(err error) ged.PanelLoadState {
	title, detail := "Git query failed", "Check Toolchain Status and repository access, then retry."
	switch {
	case errors.Is(err, core.ErrGitUnavailable):
		title = "Git unavailable"
		detail = "Git is optional and is not bundled. Configure Git in PATH, then retry; restart SilkIDE after changing the system PATH."
	case errors.Is(err, core.ErrGitNotRepository):
		title = "No Git repository"
		detail = "This folder is not a Git working tree. Open an existing repository, or initialize one yourself, then retry."
	case errors.Is(err, context.DeadlineExceeded):
		title = "Git query timed out"
	case errors.Is(err, core.ErrGitReadLimit):
		title = "Git result too large"
		detail = "The result exceeded the display budget; no partial or clean-looking snapshot was accepted."
	}
	return ged.PanelLoadState{Phase: ged.PanelFailed, Title: i18n.T(title), Detail: i18n.T(detail), RetryLabel: i18n.T("Retry")}
}
func refreshGitChanges(canvas *ged.GedView) { refreshGitChangesWith(canvas, false) }
func refreshGitChangesWith(canvas *ged.GedView, force bool) {
	panel := globalGitChanges
	if panel == nil {
		return
	}
	dir := projectDir(canvas)
	if dir == "" {
		return
	}
	if changesRefresh == nil || changesRefreshOwner != panel {
		changesRefresh.close()
		changesRefreshOwner = panel
		lastError := ""
		changesRefresh = &gitPaneRefresh[gitChangesSnapshot]{
			query: func(ctx context.Context, dir string) (gitChangesSnapshot, error) {
				root, err := core.GitRepositoryRootContext(ctx, dir)
				if err != nil {
					return gitChangesSnapshot{}, err
				}
				rows, err := core.GitStatusPorcelainContext(ctx, root)
				return gitChangesSnapshot{root, rows}, err
			}, post: gui.Post,
			loading: func() {
				panel.SetLoadState(ged.PanelLoadState{Phase: ged.PanelLoading, Title: i18n.T("Refreshing Git changes"), Detail: i18n.T("Editing remains available while Git runs."), RetryLabel: i18n.T("Retry")})
			},
			apply: func(key string, result gitChangesSnapshot, err error) {
				if globalGitChanges != panel || projectDir(canvas) != key {
					return
				}
				if err != nil {
					gitChangesRoot = ""
					panel.Clear()
					panel.SetLoadState(gitPanelFailure(err))
					if lastError != err.Error() {
						lastError = err.Error()
						logEvent(ged.LogWarn, "Git changes: "+lastError)
					}
					return
				}
				lastError = ""
				gitChangesRoot = result.root
				panel.SetEntries(result.entries)
				if len(result.entries) == 0 {
					panel.SetLoadState(ged.PanelLoadState{Phase: ged.PanelEmpty, Title: i18n.T("Working tree clean"), Detail: i18n.T("Refresh to check for external changes."), RetryLabel: i18n.T("Refresh")})
				}
			},
		}
	}
	if changesRefresh.key != dir {
		gitChangesRoot = ""
	}
	changesRefresh.request(dir, force)
}
func refreshGitLog(canvas *ged.GedView) { refreshGitLogWith(canvas, false) }
func refreshGitLogWith(canvas *ged.GedView, force bool) {
	panel := globalGitLog
	if panel == nil {
		return
	}
	dir := projectDir(canvas)
	if dir == "" {
		return
	}
	if historyRefresh == nil || historyRefreshOwner != panel {
		historyRefresh.close()
		historyRefreshOwner = panel
		lastError := ""
		historyRefresh = &gitPaneRefresh[gitHistorySnapshot]{
			query: func(ctx context.Context, dir string) (gitHistorySnapshot, error) {
				root, err := core.GitRepositoryRootContext(ctx, dir)
				if err != nil {
					return gitHistorySnapshot{}, err
				}
				rows, err := core.GitShortLogContext(ctx, root, 100)
				return gitHistorySnapshot{root, rows}, err
			}, post: gui.Post,
			loading: func() {
				panel.SetLoadState(ged.PanelLoadState{Phase: ged.PanelLoading, Title: i18n.T("Refreshing Git history"), Detail: i18n.T("Editing remains available while Git runs."), RetryLabel: i18n.T("Retry")})
			},
			apply: func(key string, result gitHistorySnapshot, err error) {
				if globalGitLog != panel || projectDir(canvas) != key {
					return
				}
				if err != nil {
					gitHistoryRoot = ""
					panel.Clear()
					panel.SetLoadState(gitPanelFailure(err))
					if lastError != err.Error() {
						lastError = err.Error()
						logEvent(ged.LogWarn, "Git history: "+lastError)
					}
					return
				}
				lastError = ""
				gitHistoryRoot = result.root
				panel.SetCommits(result.commits)
				if len(result.commits) == 0 {
					panel.SetLoadState(ged.PanelLoadState{Phase: ged.PanelEmpty, Title: i18n.T("No commits yet"), Detail: i18n.T("Refresh to check for external changes."), RetryLabel: i18n.T("Refresh")})
				}
			},
		}
	}
	if historyRefresh.key != dir {
		gitHistoryRoot = ""
	}
	historyRefresh.request(dir, force)
}
func closeGitPaneRefreshers() { changesRefresh.close(); historyRefresh.close() }
func waitGitPaneRefreshers(ctx context.Context) error {
	return errors.Join(changesRefresh.wait(ctx), historyRefresh.wait(ctx))
}

// Never execute a row action against a newer workspace or an obsolete snapshot.
func gitChangesRootFor(canvas *ged.GedView) string {
	if changesRefresh == nil || globalGitChanges == nil {
		return ""
	}
	if changesRefresh.key != projectDir(canvas) {
		refreshGitChangesWith(canvas, false)
		return ""
	}
	if globalGitChanges.LoadState().Phase != ged.PanelReady {
		return ""
	}
	return gitChangesRoot
}
func gitHistoryRootFor(canvas *ged.GedView) string {
	if historyRefresh == nil || globalGitLog == nil {
		return ""
	}
	if historyRefresh.key != projectDir(canvas) {
		refreshGitLogWith(canvas, false)
		return ""
	}
	if globalGitLog.LoadState().Phase != ged.PanelReady {
		return ""
	}
	return gitHistoryRoot
}

package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

func gitTestPosted(t *testing.T, ch <-chan func()) func() {
	t.Helper()
	select {
	case f := <-ch:
		return f
	case <-time.After(3 * time.Second):
		t.Fatal("no UI delivery")
		return nil
	}
}
func TestGitPaneRefreshCoalescesAndBlocksMissing(t *testing.T) {
	queue := make(chan func(), 8)
	gate := make(chan struct{})
	var calls atomic.Int32
	var applied []string
	s := &gitPaneRefresh[int]{query: func(ctx context.Context, k string) (int, error) {
		calls.Add(1)
		<-gate
		return 0, core.ErrGitUnavailable
	}, post: func(f func()) { queue <- f }, loading: func() {}, apply: func(k string, _ int, err error) { applied = append(applied, k) }}
	s.request("one", false)
	for i := 0; i < 10000; i++ {
		s.request("one", false)
	}
	close(gate)
	gitTestPosted(t, queue)()
	if calls.Load() != 1 || len(applied) != 1 || s.running || s.pending || !s.blocked {
		t.Fatalf("calls=%d applied=%v state=%+v", calls.Load(), applied, s)
	}
	for i := 0; i < 10000; i++ {
		s.request("one", false)
	}
	if calls.Load() != 1 {
		t.Fatal("missing Git retried on each save")
	}
	s.query = func(context.Context, string) (int, error) { calls.Add(1); return 7, nil }
	s.request("one", true)
	gitTestPosted(t, queue)()
	if calls.Load() != 2 || s.blocked || len(applied) != 2 {
		t.Fatal("explicit retry failed")
	}
	s.close()
	s.request("one", true)
	if calls.Load() != 2 {
		t.Fatal("closed service restarted")
	}
}
func TestGitPaneRefreshLatestRootCancellationAndClose(t *testing.T) {
	queue := make(chan func(), 8)
	started := make(chan string, 8)
	release := make(chan struct{})
	var concurrent, peak atomic.Int32
	var applied []string
	s := &gitPaneRefresh[int]{query: func(ctx context.Context, k string) (int, error) {
		n := concurrent.Add(1)
		defer concurrent.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		started <- k
		if k == "old" {
			<-ctx.Done()
			<-release
		}
		return 1, nil
	}, post: func(f func()) { queue <- f }, loading: func() {}, apply: func(k string, _ int, _ error) { applied = append(applied, k) }}
	s.request("old", false)
	if <-started != "old" {
		t.Fatal("not started")
	}
	s.request("intermediate", false)
	s.request("new", false)
	select {
	case k := <-started:
		t.Fatal("overlapping query", k)
	default:
	}
	close(release)
	gitTestPosted(t, queue)()
	if <-started != "new" {
		t.Fatal("intermediate not coalesced")
	}
	delivery := gitTestPosted(t, queue)
	if len(applied) != 0 || peak.Load() != 1 {
		t.Fatal("old workspace accepted or overlap", applied, peak.Load())
	}
	s.close()
	delivery()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.wait(ctx); err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Fatal("closed UI mutated")
	}
}
func TestGitPaneRefreshPendingManualRetrySurvivesFailure(t *testing.T) {
	queue := make(chan func(), 8)
	gate := make(chan struct{})
	var calls atomic.Int32
	var got error
	s := &gitPaneRefresh[int]{query: func(context.Context, string) (int, error) {
		if calls.Add(1) == 1 {
			<-gate
			return 0, core.ErrGitUnavailable
		}
		return 1, nil
	}, post: func(f func()) { queue <- f }, loading: func() {}, apply: func(_ string, _ int, err error) { got = err }}
	s.request("repo", false)
	s.request("repo", true)
	close(gate)
	gitTestPosted(t, queue)()
	gitTestPosted(t, queue)()
	if calls.Load() != 2 || got != nil || s.blocked {
		t.Fatal("manual retry dropped")
	}
	if gitPanelFailure(core.ErrGitUnavailable).Title == gitPanelFailure(errors.New("failure")).Title {
		t.Fatal("tool absence mislabeled")
	}
	s.close()
}

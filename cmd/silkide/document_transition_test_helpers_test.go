package main

import (
	"context"
	"testing"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/gui"
)

var testTransitionPosts = map[*workbench]chan func(){}

func installTestTransitions(t *testing.T, w *workbench) *documentTransitionController {
	t.Helper()
	if c := w.transitions; c != nil {
		return c
	}
	posts := make(chan func(), 512)
	testTransitionPosts[w] = posts
	q := async.NewWriteQueue[designWriteResult]()
	c := newTransitionController(w, transitionSettings{queue: q, post: func(f func()) { posts <- f }, afterModal: func(f func()) { posts <- f }})
	t.Cleanup(func() {
		c.Close()
		q.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := q.Wait(ctx); err != nil {
			t.Error("transition physical cleanup", err)
		}
		delete(testTransitionPosts, w)
	})
	return c
}

// Existing interaction regressions still invoke actual callbacks; only the
// assertion waits for their owned completion. Never pump/join a parent prompt
// from a nested gesture; the live modal loop owns that continuation.
func closeTestTab(t *testing.T, tabs *gui.TabWidget, index int) bool {
	t.Helper()
	if index < 0 || index >= tabs.Count() {
		return tabs.TabBar().CloseTab(index)
	}
	target := tabs.Stack().Page(index)
	tabs.TabBar().CloseTab(index)
	if w := globalWorkbench; !dirtyPromptUp && w != nil && w.tabs == tabs {
		pumpTestTransitions(t, w)
	}
	for i := 0; i < tabs.Count(); i++ {
		if tabs.Stack().Page(i) == target {
			return false
		}
	}
	return true
}
func pressTestTransitionShortcut(t *testing.T, mod uint8, key int) {
	t.Helper()
	press := gui.ShortcutHandler(mod, key)
	if press == nil {
		t.Fatal("missing shortcut")
	}
	press()
	if w := globalWorkbench; !dirtyPromptUp && w != nil {
		pumpTestTransitions(t, w)
	}
}

func pumpTestTransitions(t *testing.T, w *workbench) {
	pumpTestTransitionUntil(t, w, func() bool { return w.transitions == nil || w.transitions.active == nil })
}
func pumpTestTransitionUntil(t *testing.T, w *workbench, done func() bool) {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for !done() {
		select {
		case f := <-testTransitionPosts[w]:
			f()
		case <-timer.C:
			t.Fatal("transition UI delivery timed out")
		}
	}
}

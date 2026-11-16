package main

import (
	"context"
	"testing"
	"time"

	"github.com/uk0/silk/ged"
)

var testDesignOpenPosts = map[*workbench]chan func(){}

func installTestDesignOpener(t *testing.T, w *workbench) {
	t.Helper()
	posts := make(chan func(), 256)
	testDesignOpenPosts[w] = posts
	w.designOpen = newDesignOpenController(w, designOpenSettings{
		post:       func(f func()) { posts <- f },
		afterModal: func(f func()) { posts <- f },
		// Model/controller tests do not start global autosave/watch services.
		// Real watcher installation is checked separately and in native replay.
		installed: func(*ged.GedView, string, string, designOpenSnapshot) {},
	})
	owner := w.designOpen
	t.Cleanup(func() {
		owner.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := owner.jobs.Wait(ctx); err != nil {
			t.Error(err)
		}
		delete(testDesignOpenPosts, w)
	})
}

func pumpTestDesignOpenUntil(t *testing.T, w *workbench, done func() bool) {
	t.Helper()
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	for !done() {
		select {
		case f := <-testDesignOpenPosts[w]:
			f()
		case <-timer.C:
			t.Fatal("design open UI delivery timeout")
		}
	}
}

func pumpTestDesignOpens(t *testing.T, w *workbench) {
	t.Helper()
	if w == nil || w.designOpen == nil {
		return
	}
	pumpTestDesignOpenUntil(t, w, func() bool { return !w.designOpen.Busy() })
}

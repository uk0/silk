package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/gui"
)

func TestDiffDialogLoadReturnsAndCloseFencesQueuedWork(t *testing.T) {
	queue := make(chan func(), 2)
	c := newDiffDialog("QA", nil, func(fn func()) { queue <- fn })
	defer c.close()
	started, release := make(chan struct{}), make(chan struct{})
	c.start(func(ctx context.Context) (diffSource, error) {
		close(started)
		<-release
		return diffSource{"stale", "stale"}, nil
	})
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("loader not started")
	}
	if c.view.DiffPending() || len(c.view.DiffRows()) != 0 || c.next.IsEnabled() {
		t.Fatal("loading rendered stale data")
	}
	before := c.status.Text()
	c.close()
	close(release)
	select {
	case fn := <-queue:
		fn()
	case <-time.After(3 * time.Second):
		t.Fatal("load completion not dispatched")
	}
	if c.status.Text() != before || len(c.view.DiffRows()) != 0 || c.view.DiffPending() || c.ctx.Err() == nil {
		t.Fatal("late load updated a closed dialog")
	}
	if c.dialog.Parent() != nil {
		t.Fatal("closed dialog retained by owner")
	}
}
func TestDiffDialogErrorAndNavigationControls(t *testing.T) {
	queue := make(chan func(), 1)
	c := newDiffDialog("QA", nil, func(fn func()) { queue <- fn })
	defer c.close()
	c.start(func(context.Context) (diffSource, error) { return diffSource{}, errors.New("fixture read error") })
	select {
	case fn := <-queue:
		fn()
	case <-time.After(3 * time.Second):
		t.Fatal("error not delivered")
	}
	if !strings.Contains(c.status.Text(), "fixture read error") || c.next.IsEnabled() {
		t.Fatal("read error hidden")
	}
	c.view.SetTexts("a\nb\nc\nd", "a\nX\nc\nY")
	if !c.next.IsEnabled() || !c.previous.IsEnabled() || !strings.Contains(c.status.Text(), "4") || !strings.Contains(c.status.Text(), "2") {
		t.Fatal("missing ready controls/summary")
	}
	c.next.Action().Trigger(nil)
	if c.view.ActiveChangeRow() != 1 {
		t.Fatal("next did not find first change")
	}
	c.next.Action().Trigger(nil)
	if c.view.ActiveChangeRow() != 3 {
		t.Fatal("next did not advance")
	}
	c.previous.Action().Trigger(nil)
	if c.view.ActiveChangeRow() != 1 {
		t.Fatal("previous did not return")
	}
	c.view.SetTexts("same", "same")
	if c.next.IsEnabled() || c.previous.IsEnabled() {
		t.Fatal("unchanged text has enabled change controls")
	}
	for _, width := range []float64{420, 820, 1200} {
		c.dialog.SetSize(width, 600)
		c.dialog.Layout()
		_, vy, _, vh := c.view.Bounds()
		_, sy, _, sh := c.status.Bounds()
		if vh < 300 || vy < sy+sh {
			t.Fatalf("viewer did not use remaining height: width=%v height=%v y=%v", width, vh, vy)
		}
		for _, button := range []*gui.Button{c.previous, c.next} {
			x, _, w, _ := button.Bounds()
			if x < 0 || x+w > width {
				t.Fatal("navigation buttons escape")
			}
		}
	}
}
func TestDiffSavedAndPatchLoadersPreserveInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "中文 file.txt")
	saved := "a\r\n😀\n"
	current := "a\r\n未保存\n"
	if err := os.WriteFile(path, []byte(saved), 0600); err != nil {
		t.Fatal(err)
	}
	src, err := loadSavedDiff(context.Background(), path, current)
	if err != nil || src.oldText != saved || src.newText != current {
		t.Fatalf("saved source %+v %v", src, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loadSavedDiff(ctx, path, current); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored")
	}
	if _, err := loadSavedDiff(context.Background(), path+".missing", current); err == nil {
		t.Fatal("missing file hidden")
	}
	patch := "diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1,2 +1,2 @@\n context\n-old\n+new\n"
	src, err = loadPatchDiff(context.Background(), patch)
	if err != nil || src.oldText != "context\nold" || src.newText != "context\nnew" {
		t.Fatalf("patch %+v %v", src, err)
	}
	if _, err := loadPatchDiff(context.Background(), ""); err == nil {
		t.Fatal("empty patch looked successful")
	}
	if _, err := loadPatchDiff(ctx, patch); !errors.Is(err, context.Canceled) {
		t.Fatal("patch cancellation ignored")
	}
}

func TestDiffDialogEmptyComparisonIsNeutral(t *testing.T) {
	queue := make(chan func(), 1)
	c := newDiffDialog("QA", nil, func(fn func()) { queue <- fn })
	defer c.close()
	c.start(func(ctx context.Context) (diffSource, error) { return loadPatchDiff(ctx, "") })
	select {
	case fn := <-queue:
		fn()
	case <-time.After(3 * time.Second):
		t.Fatal("missing result")
	}
	if strings.Contains(c.status.Text(), "failed") || !strings.Contains(c.status.Text(), "text changes") || c.view.DiffPending() || c.view.DiffError() != nil || c.view.DiffSummary().Rows != 0 || c.next.IsEnabled() || c.previous.IsEnabled() {
		t.Fatalf("non-neutral empty state: %q", c.status.Text())
	}
}

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/diagnostic"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func TestFileFinderOpensDuringBlockedScanAndCancels(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	w := newFileSearchService(func(ctx context.Context, _ string) ([]fileEntry, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil, ctx.Err()
	}, rankFiles, func(fn func()) { t.Error("cancelled scan posted GUI work") })
	f := newFileFinder(nil, "root", w)
	receiveSymbolTest(t, started)
	if f.status.Text() != i18n.T("Indexing project files...") || f.list.Count() != 0 {
		t.Fatal("missing nonblocking indexing state")
	}
	f.input.SetText("typed during I/O")
	f.choose(0)
	if f.selected != nil {
		t.Fatal("Enter accepted pending rows")
	}
	f.close()
	receiveSymbolTest(t, stopped)
	waitFileSearch(t, w, func() bool { return !w.running })
}
func TestFileFinderVisibleLimitStaleEnterAndSelectedRow(t *testing.T) {
	posts := make(chan func(), 4)
	w := newFileSearchService(func(context.Context, string) ([]fileEntry, error) {
		files := make([]fileEntry, 501)
		for i := range files {
			files[i] = fileEntry{Path: fmt.Sprintf("/p/f%03d.go", i), Display: fmt.Sprintf("f%03d.go", i)}
		}
		return files, nil
	}, rankFiles, func(fn func()) { posts <- fn })
	f := newFileFinder(nil, "root", w)
	defer f.close()
	receiveSymbolTest(t, posts)()
	if f.list.Count() != fileFinderLimit || !strings.Contains(f.status.Text(), "501") {
		t.Fatalf("limit not visible: %d %s", f.list.Count(), f.status.Text())
	}
	f.dialog.SetSize(560, 480)
	f.dialog.Layout()
	if f.list.Height() < 200 || f.list.Y()+f.list.Height() > f.status.Y()+1 {
		t.Fatal("results overlap status/footer")
	}
	f.input.SetText("f00")
	f.choose(0)
	if f.selected != nil || f.list.Count() != 0 {
		t.Fatal("old Enter stayed actionable")
	}
	receiveSymbolTest(t, posts)()
	f.list.SetActiveIndex(1)
	expected := f.list.Item(1).Data.(fileEntry)
	f.input.OnKeyDown(gui.KeyEnter, false)
	if f.selected == nil || *f.selected != expected {
		t.Fatalf("input Enter ignored selection: %+v", f.selected)
	}
}
func TestFileFinderShowsPartialIndexAndEmptyErrors(t *testing.T) {
	oldLog := globalLog
	globalLog = nil
	defer func() { globalLog = oldLog }()
	for _, code := range []diagnostic.Code{diagnostic.FileScan, diagnostic.FileScanLimit} {
		t.Run(string(code), func(t *testing.T) {
			posts := make(chan func(), 4)
			cause := diagnostic.New(code, "unreadable or truncated", errors.New("fixture cause"))
			w := newFileSearchService(func(context.Context, string) ([]fileEntry, error) { return []fileEntry{{Display: "visible.go"}}, cause }, rankFiles, func(fn func()) { posts <- fn })
			w.debounce = time.Millisecond
			f := newFileFinder(nil, "root", w)
			defer f.close()
			receiveSymbolTest(t, posts)()
			if f.list.Count() != 1 || !strings.Contains(f.status.Text(), string(code)) {
				t.Fatal("partial files/error lost")
			}
			f.input.SetText("no-match")
			receiveSymbolTest(t, posts)()
			if f.list.Count() != 0 || !strings.Contains(f.status.Text(), string(code)) {
				t.Fatal("empty result falsely claims complete index")
			}
		})
	}
	posts := make(chan func(), 2)
	w := newFileSearchService(func(context.Context, string) ([]fileEntry, error) { return nil, nil }, rankFiles, func(fn func()) { posts <- fn })
	f := newFileFinder(nil, "root", w)
	defer f.close()
	receiveSymbolTest(t, posts)()
	if f.status.Text() != i18n.T("No files found") || f.list.ActiveIndex() != -1 {
		t.Fatal("missing empty state")
	}
}

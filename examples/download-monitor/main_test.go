package main

import (
	"context"
	"io"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/httpx"
)

func TestMonitorCompactLayoutAndCancellation(t *testing.T) {
	f := newMonitorForm("http://example.invalid")
	defer f.dispose()
	for _, s := range [][2]float64{{840, 340}, {558, 303}, {400, 320}, {180, 100}} {
		f.SetSize(s[0], s[1])
		f.OnResize()
		for _, w := range f.Children() {
			x, y, width, height := w.Bounds()
			if x < 0 || y < 0 || x+width > s[0] || y+height > s[1] {
				t.Fatal("out of bounds", s, w)
			}
		}
	}
	f.SetSize(558, 303)
	f.OnResize()
	if !f.note.IsVisible() {
		t.Fatal("footer should fit medium window")
	}
	_, y, _, height := f.note.Bounds()
	if height != 26 || y+height > 291 {
		t.Fatal("footer clipped", y, height)
	}
	f.SetSize(400, 320)
	f.OnResize()
	if f.note.IsVisible() {
		t.Fatal("compact layout should hide optional footer")
	}
	if _, _, _, h := f.progress.Bounds(); h < 82 {
		t.Fatal("compact layout lost progress details", h)
	}
	f.cancel()
	if f.progress.State() != gui.TransferCancelled || f.task != nil || f.latest != nil {
		t.Fatal("cancel state")
	}
	f.dispose()
	f.start()
	if f.task != nil {
		t.Fatal("closed form starts task")
	}
}
func TestDemoServerErrorAndCancelledStream(t *testing.T) {
	base, stop, err := demoServer()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	var written int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := httpx.Default().DownloadTo(ctx, httpx.Request{URL: base + "/known"}, io.Discard, httpx.DownloadOptions{OnProgress: func(p httpx.DownloadProgress) {
		written = p.Received
		if written > 0 {
			cancel()
		}
	}})
	if err == nil || result.TotalBytes() != demoBytes || written == 0 {
		t.Fatal(result, err, written)
	}
	result, err = httpx.Default().DownloadTo(context.Background(), httpx.Request{URL: base + "/error"}, io.Discard, httpx.DownloadOptions{})
	if err == nil || result.StatusCode() != 503 || result.BytesWritten() != 0 {
		t.Fatal(result, err)
	}
}

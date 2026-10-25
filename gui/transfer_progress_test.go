package gui

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/httpx"
	"github.com/uk0/silk/paint"
)

func TestTransferProgressPrecisionStateAndIdle(t *testing.T) {
	v := core.New("gui.TransferProgress").(*TransferProgress)
	if v.State() != TransferIdle || v.TotalBytes() != -1 || !v.ShowBytes() {
		t.Fatal("factory defaults")
	}
	before := HasActiveAnimations()
	var states []string
	v.SigStateChanged(func(s string) { states = append(states, s) })
	v.SetProgress(9007199254740993, math.MaxInt64)
	if v.ReceivedBytes() != 9007199254740993 {
		t.Fatal("lost integer precision")
	}
	if err := v.SetState(TransferRunning); err != nil {
		t.Fatal(err)
	}
	_ = v.SetState(TransferRunning)
	v.SetProgress(math.MaxInt64-1, math.MaxInt64)
	if strings.HasPrefix(v.Summary(), "100%") {
		t.Fatal("rounding falsely completed transfer")
	}
	if len(states) != 1 || HasActiveAnimations() != before {
		t.Fatal("duplicate signal or heartbeat", states)
	}
	if err := v.SetState("bad"); err == nil || v.State() != TransferRunning {
		t.Fatal("invalid state accepted")
	}
	v.SetProgress(10, 10)
	if v.State() != TransferRunning {
		t.Fatal("byte progress declared checksum success")
	}
	v.SetProgress(0, 0)
	if ratio, _ := v.Fraction(); ratio != 0 {
		t.Fatal(ratio)
	}
	_ = v.SetState(TransferSucceeded)
	if ratio, _ := v.Fraction(); ratio != 1 {
		t.Fatal(ratio)
	}
	v.SetProgress(-9, -20)
	if v.ReceivedBytes() != 0 || v.TotalBytes() != -1 {
		t.Fatal("invalid counts")
	}
	v.Reset()
	if v.State() != TransferIdle {
		t.Fatal(v.State())
	}
	v.SetShowBytes(false)
	if strings.Contains(v.Summary(), " B") {
		t.Fatal(v.Summary())
	}
	if got := transferTitle(strings.Repeat("中", 10000) + "\n"); len([]rune(got)) > 129 {
		t.Fatal("unbounded title")
	}
	if strings.Contains(transferTitle("a\nb\tc"), "\n") {
		t.Fatal("multiline title")
	}
}
func TestTransferDownloadUsesCoalescedUIQueue(t *testing.T) {
	resetUIQueue()
	defer resetUIQueue()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, strings.Repeat("x", 1<<20))
	}))
	defer server.Close()
	v := NewTransferProgress()
	_ = v.SetState(TransferRunning)
	updates := 0
	// Post is shared with background services (including Git status). Count this
	// producer's dispatches, not unrelated callbacks already in the UI queue.
	var posted atomic.Int32
	latest, err := async.NewLatest(func(fn func()) {
		posted.Add(1)
		Post(fn)
	}, func(p httpx.DownloadProgress) { updates++; v.SetProgress(p.Received, p.Total) })
	if err != nil {
		t.Fatal(err)
	}
	defer latest.Close()
	unrelatedRan := false
	Post(func() { unrelatedRan = true })
	future, err := httpx.Default().DownloadToAsync(context.Background(), httpx.Request{URL: server.URL}, io.Discard, httpx.DownloadOptions{ProgressInterval: time.Nanosecond, OnProgress: func(p httpx.DownloadProgress) { latest.Publish(p) }})
	if err != nil {
		t.Fatal(err)
	}
	result, err := future.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v.ReceivedBytes() != 0 || updates != 0 {
		t.Fatal("worker mutated GUI")
	}
	if got := posted.Load(); got != 1 {
		t.Fatal("progress flooded queue", got)
	}
	drainUITasks()
	if !unrelatedRan {
		t.Fatal("shared UI callback was not dispatched")
	}
	if updates != 1 || v.ReceivedBytes() != result.BytesWritten() || v.TotalBytes() != -1 {
		t.Fatal(updates, v.ReceivedBytes())
	}
	latest.Publish(httpx.DownloadProgress{Received: 7, Total: 9})
	latest.Close()
	_ = v.SetState(TransferCancelled)
	drainUITasks()
	if v.State() != TransferCancelled || v.ReceivedBytes() != 1<<20 {
		t.Fatal("late progress changed cancelled view")
	}
}
func TestProgressBarRejectsNonFiniteValue(t *testing.T) {
	v := NewProgressBar()
	for _, tc := range []struct{ in, want float64 }{{math.NaN(), 0}, {math.Inf(1), 1}, {math.Inf(-1), 0}, {.5, .5}} {
		v.SetValue(tc.in)
		if v.Value() != tc.want {
			t.Fatal(tc, v.Value())
		}
	}
}

func TestTransferProgressPaintClipsAndUsesStateColors(t *testing.T) {
	v := NewTransferProgress()
	v.SetLabel(strings.Repeat("很长的标题", 1000))
	v.SetProgress(50, 100)
	for _, state := range []TransferState{TransferRunning, TransferSucceeded, TransferFailed, TransferCancelled} {
		_ = v.SetState(state)
		for _, size := range [][2]int{{360, 82}, {120, 50}, {20, 12}} {
			v.SetSize(float64(size[0]), float64(size[1]))
			pix := paint.NewPixmap(size[0]+20, size[1]+20)
			g := pix.NewPainter()
			v.Draw(g)
			paint.ClosePainter(g)
			img, err := pix.Image()
			if err != nil {
				t.Fatal(err)
			}
			for _, point := range [][2]int{{size[0] + 2, 5}, {5, size[1] + 2}} {
				if _, _, _, a := img.At(point[0], point[1]).RGBA(); a != 0 {
					t.Fatal("paint escaped widget bounds", size, point)
				}
			}
			if size[0] == 360 {
				want := Theme().Accent
				palette := paint.CurrentIconPalette()
				switch state {
				case TransferSucceeded:
					want = palette.Success
				case TransferFailed:
					want = palette.Danger
				case TransferCancelled:
					want = palette.Disabled
				}
				if got := paint.ColorModel.Convert(img.At(20, 66)).(paint.Color); got != want {
					t.Fatal("progress color", state, got, want)
				}
			}
			paint.CloseSurface(pix)
		}
	}
}

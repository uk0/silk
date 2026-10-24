// Download Monitor demonstrates a streaming writer, coalesced UI progress and
// a designer-ready TransferProgress. Demo bytes go to io.Discard, NOT a file.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/httpx"
)

type monitorForm struct {
	gui.Form
	url                *gui.Edit
	startButton        *gui.Button
	presets            []*gui.Button
	progress           *gui.TransferProgress
	status, hash, note *gui.Label
	expectedHash       string
	task               *async.Future[*httpx.DownloadResult]
	latest             *async.Latest[httpx.DownloadProgress]
	generation         uint64
	closed             bool
}

func newMonitorForm(base string) *monitorForm {
	f := new(monitorForm)
	f.Form.Init(f)
	f.SetTitle("Silk Download Monitor - streaming SDK")
	f.url = gui.NewEdit()
	f.url.SetText(base + "/known")
	button := func(text string, callback func()) *gui.Button {
		b := gui.NewButton()
		b.SetText(text)
		b.SetTextVisible(true)
		b.SetParent(f)
		b.Action().BindFunc0(callback)
		return b
	}
	f.startButton = button("Download", f.start)
	f.startButton.SetIconName("transfer-progress")
	for _, preset := range []struct{ name, path, hash string }{{"Known size", "/known", ""}, {"Unknown size", "/unknown", ""}, {"Bad checksum", "/known", strings.Repeat("0", 64)}, {"HTTP error", "/error", ""}} {
		f.presets = append(f.presets, button(preset.name, func() { f.url.SetText(base + preset.path); f.expectedHash = preset.hash }))
	}
	f.progress = gui.NewTransferProgress()
	f.progress.SetLabel("Download · streaming to io.Discard")
	f.status = gui.NewLabel("Ready. Click Download; click again to cancel.")
	f.hash = gui.NewLabel("SHA-256: —")
	f.note = gui.NewLabel("Local demo · no files created · no background polling")
	for _, w := range []gui.IWidget{f.url, f.progress, f.status, f.hash, f.note} {
		w.SetParent(f)
	}
	return f
}
func (f *monitorForm) start() {
	if f.closed {
		return
	}
	if f.task != nil {
		f.cancel()
		return
	}
	f.generation++
	id := f.generation
	f.progress.Reset()
	_ = f.progress.SetState(gui.TransferRunning)
	f.status.SetText("Receiving in background; progress notifications are coalesced.")
	f.hash.SetText("SHA-256: calculating…")
	f.startButton.SetText("Cancel")
	latest, _ := async.NewLatest(gui.Post, func(p httpx.DownloadProgress) {
		if !f.closed && f.generation == id {
			f.progress.SetProgress(p.Received, p.Total)
		}
	})
	f.latest = latest
	future, err := httpx.Default().DownloadToAsync(context.Background(), httpx.Request{URL: f.url.Text()}, io.Discard, httpx.DownloadOptions{
		MaxBytes: 16 << 20, ExpectedSHA256: f.expectedHash, OnProgress: func(p httpx.DownloadProgress) { latest.Publish(p) },
	})
	if err != nil {
		latest.Close()
		f.latest = nil
		f.startButton.SetText("Download")
		f.status.SetText(err.Error())
		f.hash.SetText("SHA-256: unavailable")
		_ = f.progress.SetState(gui.TransferFailed)
		return
	}
	f.task = future
	_ = future.OnComplete(gui.Post, func(result *httpx.DownloadResult, err error) {
		latest.Close()
		if f.closed || f.generation != id {
			return
		}
		f.task = nil
		f.latest = nil
		f.startButton.SetText("Download")
		if result != nil {
			f.progress.SetProgress(result.BytesWritten(), result.TotalBytes())
			if result.SHA256() != "" {
				f.hash.SetText("SHA-256: " + result.SHA256())
			}
		}
		if err != nil {
			if result == nil || result.SHA256() == "" {
				f.hash.SetText("SHA-256: incomplete")
			}
			state := gui.TransferFailed
			if errors.Is(err, context.Canceled) {
				state = gui.TransferCancelled
			}
			_ = f.progress.SetState(state)
			f.status.SetText(err.Error())
			return
		}
		_ = f.progress.SetState(gui.TransferSucceeded)
		f.status.SetText(fmt.Sprintf("Complete: %d bytes streamed; no response-sized buffer or file.", result.BytesWritten()))
	})
}
func (f *monitorForm) cancel() {
	f.generation++
	if f.latest != nil {
		f.latest.Close()
		f.latest = nil
	}
	if f.task != nil {
		f.task.Cancel()
		f.task = nil
	}
	f.startButton.SetText("Download")
	_ = f.progress.SetState(gui.TransferCancelled)
	f.status.SetText("Cancelled; late updates cannot overwrite this state.")
	f.hash.SetText("SHA-256: incomplete")
}
func (f *monitorForm) dispose() {
	f.closed = true
	f.generation++
	if f.latest != nil {
		f.latest.Close()
	}
	if f.task != nil {
		f.task.Cancel()
	}
}
func (f *monitorForm) OnResize() {
	if f.url == nil {
		return
	}
	w, h := f.Size()
	pad, gap := 12.0, 8.0
	width := max(0, w-2*pad)
	place := func(child gui.IWidget, x, y, cw, ch float64) {
		x = max(0, min(w, x))
		y = max(0, min(h, y))
		child.SetBounds(x, y, max(0, min(w-x, cw)), max(0, min(h-y, ch)))
	}
	buttonW := min(132, width*.3)
	place(f.url, pad, pad, max(0, width-buttonW-gap), 32)
	place(f.startButton, max(pad, w-pad-buttonW), pad, buttonW, 32)
	columns := 4
	if width < 520 {
		columns = 2
	}
	rows := (len(f.presets) + columns - 1) / columns
	presetW := max(0, (width-float64(columns-1)*gap)/float64(columns))
	for i, b := range f.presets {
		y := 52 + float64(i/columns)*38
		b.SetVisible(y+30 <= h-pad)
		place(b, pad+float64(i%columns)*(presetW+gap), y, presetW, 30)
	}
	progressY := 52 + float64(rows)*38 + 6
	// Prefer readable progress and result lines over a half-clipped footer.
	showNote := h >= progressY+82+122
	bottom := 86.0 // status + digest, gaps and bottom padding
	if showNote {
		bottom += 36
	}
	progressH := max(0, min(96, h-progressY-bottom))
	place(f.progress, pad, progressY, width, progressH)
	y := progressY + progressH + 12
	for i, label := range []*gui.Label{f.status, f.hash, f.note} {
		visible := y+26 <= h-pad && (i != 2 || showNote)
		label.SetVisible(visible)
		place(label, pad, y, width, 26)
		y += 36
	}
}

const demoBytes = 2 << 20

func demoServer() (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	block := []byte(strings.Repeat("Silk transfer. ", 2048))
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/error" {
			http.Error(w, "demonstration failure", http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path != "/known" && r.URL.Path != "/unknown" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		if r.URL.Path == "/known" {
			w.Header().Set("Content-Length", fmt.Sprint(demoBytes))
		}
		w.(http.Flusher).Flush()
		for remaining := demoBytes; remaining > 0; {
			n := min(len(block), remaining)
			if _, err := w.Write(block[:n]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			remaining -= n
			select {
			case <-r.Context().Done():
				return
			case <-time.After(40 * time.Millisecond):
			}
		}
	})}
	go func() { _ = server.Serve(listener) }()
	return "http://" + listener.Addr().String(), func() { _ = server.Close() }, nil
}
func main() {
	defer core.Close()
	url, stop, err := demoServer()
	if err != nil {
		panic(err)
	}
	defer stop()
	f := newMonitorForm(url)
	defer f.dispose()
	f.AttachWindow(gui.WtForm)
	f.Window().SetSize(840, 340)
	f.Window().MoveToCenter()
	f.Show()
	core.EventLoop()
}

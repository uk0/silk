// JSON Inspector is a local-only API browser demonstrating jsonx, httpx,
// gui.RunAsync and the reusable JSONView. Click Fetch to perform a request.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/httpx"
	"github.com/uk0/silk/jsonx"
)

type inspectorForm struct {
	gui.Form
	url, path                          *gui.Edit
	fetch, locate, copyPath, copyValue *gui.Button
	status                             *gui.Label
	viewer                             *gui.JSONView
	task                               *async.Future[*jsonx.Document]
	sequence                           uint64
	closed                             bool
}

func newInspectorForm(url string) *inspectorForm {
	f := new(inspectorForm)
	f.Form.Init(f)
	f.SetTitle("Silk JSON Inspector - SDK example")
	f.url = gui.NewEdit()
	f.url.SetText(url)
	f.path = gui.NewEdit()
	f.path.SetText("/data/name")
	button := func(text, icon string) *gui.Button {
		b := gui.NewButton()
		b.SetText(text)
		b.SetTextVisible(true)
		b.SetIconName(icon)
		b.SetParent(f)
		return b
	}
	f.fetch = button("Fetch JSON", "http-request")
	f.locate = button("Locate", "search")
	f.copyPath = button("Copy path", "function")
	f.copyValue = button("Copy JSON", "json-view")
	f.status = gui.NewLabel("Ready - local demo, large arrays expand in groups. Ctrl/Cmd+C copies JSON.")
	f.viewer = gui.NewJSONView()
	f.viewer.SetJSONText(`{"hint":"Click Fetch JSON; use /data/items/1999/id to test grouped navigation"}`)
	for _, w := range []gui.IWidget{f.url, f.path, f.status, f.viewer} {
		w.SetParent(f)
	}
	f.fetch.Action().BindFunc0(f.start)
	f.locate.Action().BindFunc0(func() {
		if err := f.viewer.SelectPointer(f.path.Text()); err != nil {
			f.status.SetText(err.Error())
		}
	})
	f.copyPath.Action().BindFunc0(func() {
		if err := f.viewer.CopySelectedPointer(); err != nil {
			f.status.SetText(err.Error())
		} else {
			f.status.SetText("JSON Pointer copied")
		}
	})
	f.copyValue.Action().BindFunc0(func() {
		if err := f.viewer.CopySelectedJSON(); err != nil {
			f.status.SetText(err.Error())
		} else {
			f.status.SetText("Selected JSON copied")
		}
	})
	f.viewer.SigSelected(func(pointer string) {
		f.path.SetText(pointer)
		node, _ := f.viewer.SelectedNode()
		f.status.SetText(fmt.Sprintf("%s · %s", node.Kind(), node.Preview(110)))
	})
	f.viewer.SigError(func(message string) { f.status.SetText(message) })
	return f
}
func (f *inspectorForm) start() {
	if f.closed {
		return
	}
	if f.task != nil {
		f.sequence++
		f.task.Cancel()
		f.task = nil
		f.fetch.SetText("Fetch JSON")
		f.status.SetText("Cancelled; previous data retained")
		return
	}
	url := f.url.Text()
	f.sequence++
	id := f.sequence
	f.fetch.SetText("Cancel")
	f.status.SetText("Fetching and parsing in background…")
	future, err := gui.RunAsync(context.Background(), nil, func(ctx context.Context) (*jsonx.Document, error) {
		r, err := httpx.Default().Get(ctx, url)
		if err != nil {
			return nil, err
		}
		doc, err := r.Document()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return doc, err
	}, func(doc *jsonx.Document, err error) {
		if f.closed || id != f.sequence {
			return
		}
		f.task = nil
		f.fetch.SetText("Fetch JSON")
		if err != nil {
			f.status.SetText("Request failed: " + err.Error())
			return
		}
		if err = f.viewer.SetDocument(doc); err != nil {
			f.status.SetText(err.Error())
			return
		}
		f.status.SetText(fmt.Sprintf("Loaded %d nodes. Expand branches or locate a JSON Pointer.", doc.NodeCount()))
	})
	if err != nil {
		f.fetch.SetText("Fetch JSON")
		f.status.SetText(err.Error())
		return
	}
	f.task = future
}
func (f *inspectorForm) dispose() {
	f.closed = true
	f.sequence++
	if f.task != nil {
		f.task.Cancel()
	}
	f.viewer.Close()
}
func (f *inspectorForm) OnResize() {
	if f.url == nil {
		return
	}
	w, h := f.Size()
	margin, gap, row := 12.0, 8.0, 32.0
	place := func(child gui.IWidget, x, y, cw, ch float64) {
		x = max(0, min(w, x))
		y = max(0, min(h, y))
		child.SetBounds(x, y, max(0, min(w-x, cw)), max(0, min(h-y, ch)))
	}
	width := max(0, w-2*margin)
	buttonW := min(144, width*.3)
	place(f.url, margin, margin, max(0, width-buttonW-gap), row)
	place(f.fetch, max(margin, w-margin-buttonW), margin, buttonW, row)
	y := margin + row + gap
	place(f.path, margin, y, max(0, width-buttonW-gap), row)
	place(f.locate, max(margin, w-margin-buttonW), y, buttonW, row)
	y += row + gap
	copyW := min(144, max(0, (width-gap)/2))
	place(f.copyPath, margin, y, copyW, row)
	place(f.copyValue, margin+copyW+gap, y, copyW, row)
	y += row + gap
	place(f.status, margin, y, width, 24)
	y += 32
	place(f.viewer, margin, y, width, max(0, h-y-margin))
}
func demoServer() (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/slow") {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
		if strings.HasSuffix(r.URL.Path, "/invalid") {
			fmt.Fprint(w, "{broken")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		items := make([]map[string]any, 2000)
		for i := range items {
			items[i] = map[string]any{"id": i, "ready": i%2 == 0}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"name": "Silk 中文😀", "count": int64(9007199254740993), "a/b": map[string]any{"~": "escaped pointer"}, "items": items}})
	})}
	go func() { _ = server.Serve(listener) }()
	return "http://" + listener.Addr().String() + "/json", func() { _ = server.Close() }, nil
}
func main() {
	defer core.Close()
	url, stop, err := demoServer()
	if err != nil {
		panic(err)
	}
	defer stop()
	f := newInspectorForm(url)
	defer f.dispose()
	f.AttachWindow(gui.WtForm)
	f.Window().SetSize(840, 600)
	f.Window().MoveToCenter()
	f.Show()
	core.EventLoop()
}

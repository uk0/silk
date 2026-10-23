// HTTP Tool demonstrates composing edits, selectors, an HTTP action and a result
// editor. Its initial URL is a local demo server; nothing external is contacted
// unless the user explicitly changes the URL and sends a request.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

type httpTool struct {
	gui.Form
	url, pointer, body *gui.Edit
	method, kind       *gui.ComboBox
	send               *gui.HTTPButton
	output             *gui.CodeEditor
	status             *gui.Label
	labels             []*gui.Label
}

func newHTTPTool(url string) *httpTool {
	f := new(httpTool)
	f.Form.Init(f)
	f.SetTitle("Silk HTTP Tool - SDK example")
	f.url = gui.NewEdit()
	f.url.SetText(url)
	f.pointer = gui.NewEdit()
	f.pointer.SetText("/data")
	f.body = gui.NewEdit()
	f.body.SetText(`{"name":"Silk"}`)
	f.method = gui.NewComboBox()
	f.method.SetItems([]string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"})
	f.method.SetText("GET")
	f.kind = gui.NewComboBox()
	f.kind.SetItems([]string{"str", "json", "int", "float", "bool"})
	f.kind.SetText("json")
	f.send = gui.NewHTTPButton()
	f.send.SetText("Send request")
	f.output = gui.NewCodeEditor()
	f.status = gui.NewLabel("Local demo ready. /data/count -> int; /data/name -> str.")
	for _, label := range []string{"URL", "Method / Type", "JSON Pointer", "Body"} {
		l := gui.NewLabel(label)
		l.SetParent(f)
		f.labels = append(f.labels, l)
	}
	for _, w := range []gui.IWidget{f.url, f.pointer, f.body, f.method, f.kind, f.send, f.output, f.status} {
		w.SetParent(f)
	}
	f.send.Action().BindFunc0(func() {
		if f.send.IsBusy() {
			return
		}
		f.send.SetURL(f.url.Text())
		f.send.SetMethod(f.method.Text())
		f.send.SetResultType(f.kind.Text())
		f.send.SetJSONPointer(f.pointer.Text())
		f.send.SetBody("")
		f.send.SetHeadersJSON("")
		if f.method.Text() != "GET" && f.method.Text() != "HEAD" {
			f.send.SetBody(f.body.Text())
			f.send.SetHeadersJSON(`{"Content-Type":"application/json"}`)
		}
	})
	f.send.SigBusyChanged(func(busy bool) {
		if busy {
			f.status.SetText("Request running - click again to cancel")
			f.send.SetText("Cancel request")
		} else {
			f.send.SetText("Send request")
		}
	})
	f.send.SigResult(func(value string) {
		f.output.SetText(value)
		f.status.SetText(fmt.Sprintf("HTTP %d - %s result", f.send.LastResponse().StatusCode(), f.send.ResultType()))
	})
	f.send.SigError(func(message string) {
		f.output.SetText(message)
		f.status.SetText("Request failed (no value delivered)")
	})
	return f
}
func (f *httpTool) OnResize() {
	if f.url == nil {
		return
	}
	w, h := f.Size()
	place := func(child gui.IWidget, x, y, cw, ch float64) {
		x = max(0, min(w, x))
		y = max(0, min(h, y))
		child.SetBounds(x, y, max(0, min(w-x, cw)), max(0, min(h-y, ch)))
	}
	margin := 12.0
	labelW := 108.0
	row := 32.0
	gap := 8.0
	inputX := margin + labelW
	inputW := max(0, w-inputX-margin)
	for i, label := range f.labels {
		place(label, margin, margin+float64(i)*(row+gap), max(0, min(labelW-6, w-margin*2)), row)
	}
	place(f.url, inputX, margin, inputW, row)
	place(f.method, inputX, margin+row+gap, max(0, (inputW-gap)/2), row)
	place(f.kind, inputX+max(0, (inputW-gap)/2)+gap, margin+row+gap, max(0, (inputW-gap)/2), row)
	place(f.pointer, inputX, margin+2*(row+gap), inputW, row)
	place(f.body, inputX, margin+3*(row+gap), inputW, row)
	y := margin + 4*(row+gap)
	place(f.send, margin, y, max(0, min(180, w-margin*2)), row)
	place(f.status, margin, y+row+gap, max(0, w-margin*2), 24)
	y += row + gap + 32
	place(f.output, margin, y, max(0, w-margin*2), max(0, h-y-margin))
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
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/error") {
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"name": "Silk 中文😀", "count": int64(9007199254740993), "ratio": 1.25, "ready": true, "method": r.Method}})
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
	form := newHTTPTool(url)
	defer form.send.Close()
	form.AttachWindow(gui.WtForm)
	form.Window().SetSize(780, 520)
	form.Window().MoveToCenter()
	form.Show()
	core.EventLoop()
}

package gui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
)

func pumpHTTP(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		drainUITasks()
		if done() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("HTTP UI completion timed out")
}
func TestHTTPButtonTypedResultUIQueueAndAction(t *testing.T) {
	resetUIQueue()
	defer resetUIQueue()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer runtime" {
			t.Error("missing runtime header")
		}
		fmt.Fprint(w, `{"s":"中文😀","n":9223372036854775807,"f":1.25,"b":true}`)
	}))
	defer server.Close()
	b := core.New("gui.HTTPButton").(*HTTPButton)
	if !b.IsTextVisible() {
		t.Fatal("HTTP action label is hidden behind its icon")
	}
	defer b.Close()
	b.SetURL(server.URL)
	b.SetHeadersJSON(`{"Authorization":"design"}`)
	b.SetHeader("Authorization", "Bearer runtime")
	if requests.Load() != 0 {
		t.Fatal("constructor or setters did I/O")
	}
	for _, tc := range []struct{ kind, path, want string }{{"str", "/s", "中文😀"}, {"int", "/n", "9223372036854775807"}, {"float", "/f", "1.25"}, {"bool", "/b", "true"}, {"json", "/n", "9223372036854775807"}} {
		got := ""
		var phases []bool
		var typed any
		b.SigResult(func(v string) { got = v })
		b.SigBusyChanged(func(v bool) { phases = append(phases, v) })
		b.SigIntResult(func(v int64) { typed = v })
		b.SigFloatResult(func(v float64) { typed = v })
		b.SigBoolResult(func(v bool) { typed = v })
		// Existing OnClick configuration must not replace the internal HTTP action.
		b.Action().BindFunc0(func() { b.SetResultType(tc.kind); b.SetJSONPointer(tc.path) })
		b.Action().Trigger(b)
		if !b.IsBusy() || got != "" {
			t.Fatal("completion bypassed UI queue")
		}
		if err := b.Start(context.Background()); !errors.Is(err, async.ErrBusy) {
			t.Fatal(err)
		}
		pumpHTTP(t, func() bool { return !b.IsBusy() })
		if got != tc.want || b.LastError() != nil || b.LastResponse().StatusCode() != 200 || !reflect.DeepEqual(phases, []bool{true, false}) {
			t.Fatal(tc, got, b.LastError(), phases)
		}
		switch tc.kind {
		case "int":
			if typed != int64(9223372036854775807) {
				t.Fatal(typed)
			}
		case "float":
			if typed != 1.25 {
				t.Fatal(typed)
			}
		case "bool":
			if typed != true {
				t.Fatal(typed)
			}
		}
	}
}
func TestHTTPButtonErrorsCancelDetachAndStaleCompletion(t *testing.T) {
	resetUIQueue()
	defer resetUIQueue()
	slowStarted := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			slowStarted <- struct{}{}
			<-r.Context().Done()
			return
		}
		if r.URL.Path == "/error" {
			w.WriteHeader(500)
		}
		fmt.Fprint(w, `{"n":1}`)
	}))
	defer server.Close()
	b := NewHTTPButton()
	defer b.Close()
	b.SetURL(server.URL + "/slow")
	errorsSeen, results := 0, 0
	b.SigError(func(string) { errorsSeen++ })
	b.SigResult(func(string) { results++ })
	if err := b.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-slowStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("server not reached")
	}
	b.Cancel()
	if b.IsBusy() || errorsSeen != 1 || !errors.Is(b.LastError(), context.Canceled) {
		t.Fatal(b.IsBusy(), errorsSeen, b.LastError())
	}
	b.SetURL(server.URL)
	b.SetResultType("int")
	b.SetJSONPointer("/n")
	if err := b.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	pumpHTTP(t, func() bool { return !b.IsBusy() })
	if results != 1 || errorsSeen != 1 || b.LastError() != nil {
		t.Fatal("stale cancellation overwrote newer result", results, errorsSeen, b.LastError())
	}
	b.SetURL(server.URL + "/error")
	if err := b.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	pumpHTTP(t, func() bool { return !b.IsBusy() })
	if errorsSeen != 2 || b.LastResponse().StatusCode() != 500 {
		t.Fatal(errorsSeen)
	}
	b.SetURL(server.URL)
	b.SetJSONPointer("/missing")
	if err := b.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	pumpHTTP(t, func() bool { return !b.IsBusy() })
	if errorsSeen != 3 || results != 1 {
		t.Fatal("conversion failure treated as success")
	}
	b.SetHeadersJSON(`{"X":12}`)
	if err := b.Start(context.Background()); err == nil || b.IsBusy() {
		t.Fatal("invalid headers admitted")
	}
	b.SetHeadersJSON("")
	parent := NewForm()
	b.SetParent(parent)
	b.SetURL(server.URL + "/slow")
	if err := b.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-slowStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("server not reached")
	}
	before := errorsSeen
	b.Detach()
	if b.IsBusy() {
		t.Fatal("detach did not retire operation")
	}
	// A reparented button may be reused; a closed one may not.
	b.SetParent(parent)
	b.SetURL(server.URL)
	b.SetJSONPointer("/n")
	if err := b.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	b.Close()
	drainUITasks()
	if errorsSeen != before || results != 1 {
		t.Fatal("closed/detached control delivered stale callback")
	}
	if err := b.Start(context.Background()); !errors.Is(err, async.ErrClosed) {
		t.Fatal(err)
	}
}
func TestHTTPButtonQueuedCompletionCloseAndReentrantCancel(t *testing.T) {
	resetUIQueue()
	defer resetUIQueue()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	defer server.Close()
	b := NewHTTPButton()
	defer b.Close()
	b.SetURL(server.URL)
	delivered := 0
	b.SigResult(func(string) { delivered++ })
	if err := b.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Wait for work to enqueue its completion, without running UI callbacks.
	deadline := time.Now().Add(3 * time.Second)
	for {
		uiTaskMu.Lock()
		n := len(uiTasks)
		uiTaskMu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no completion")
		}
		time.Sleep(time.Millisecond)
	}
	b.Close()
	drainUITasks()
	if delivered != 0 || b.IsBusy() {
		t.Fatal("close accepted queued result")
	}
	c := NewHTTPButton()
	defer c.Close()
	c.SetURL(server.URL)
	c.SigBusyChanged(func(v bool) {
		if v {
			c.Cancel()
		}
	})
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.IsBusy() || !errors.Is(c.LastError(), context.Canceled) {
		t.Fatal("busy callback cancellation failed")
	}
}
func TestRunAsyncDispatchesOnlyOnUIQueue(t *testing.T) {
	resetUIQueue()
	defer resetUIQueue()
	complete := false
	f, err := RunAsync(context.Background(), nil, func(context.Context) (string, error) { return "done", nil }, func(v string, err error) {
		if v != "done" || err != nil {
			t.Error(v, err)
		}
		complete = true
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := f.Await(ctx); err != nil {
		t.Fatal(err)
	}
	if complete {
		t.Fatal("worker accessed GUI callback")
	}
	pumpHTTP(t, func() bool { return complete })
}

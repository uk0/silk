package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/httpx"
	"github.com/uk0/silk/paint"
)

func init() { core.RegisterFactory("gui.HTTPButton", core.TypeOf((*HTTPButton)(nil))) }

// HTTPButton is an explicit, asynchronous request action. Constructing, loading
// or editing its properties NEVER performs I/O. Click again while busy to cancel.
// All methods/signals are UI-thread-only. Call Close when its host is disposed;
// direct detach/reparent cancels automatically. No response/secrets are persisted.
type HTTPButton struct {
	Button
	url, method, headersJSON, body, resultType, jsonPointer string
	timeoutMS, maxResponseKB                                int
	runtimeHeaders                                          http.Header
	busy, closed                                            bool
	generation                                              uint64
	cancel                                                  context.CancelFunc
	lastResponse                                            *httpx.Response
	lastError                                               error
	result                                                  func(string)
	intResult                                               func(int64)
	floatResult                                             func(float64)
	boolResult                                              func(bool)
	failed                                                  func(string)
	busyChanged                                             func(bool)
}

func NewHTTPButton() *HTTPButton { b := new(HTTPButton); b.Init(b); return b }
func (b *HTTPButton) Init(self IWidget) {
	b.Button.Init(self)
	b.method = "GET"
	b.resultType = "str"
	b.timeoutMS = 30000
	b.maxResponseKB = 4096
	b.SetAction(&httpButtonAction{execute: func() {
		if b.busy {
			b.Cancel()
			return
		}
		_ = b.Start(context.Background())
	}})
	b.SetText("HTTP")
	b.SetTextVisible(true)
	b.SetIconName("http-request")
}

type httpButtonAction struct {
	Action
	execute func()
}

func (a *httpButtonAction) Trigger(sender interface{}) {
	if !a.IsEnabled() {
		return
	}
	a.Action.Trigger(sender)
	if a.IsEnabled() && a.execute != nil {
		a.execute()
	}
}
func (b *HTTPButton) URL() string                   { return b.url }
func (b *HTTPButton) SetURL(s string)               { b.url = s }
func (b *HTTPButton) Method() string                { return b.method }
func (b *HTTPButton) SetMethod(s string)            { b.method = strings.ToUpper(strings.TrimSpace(s)) }
func (b *HTTPButton) HeadersJSON() string           { return b.headersJSON }
func (b *HTTPButton) SetHeadersJSON(s string)       { b.headersJSON = s }
func (b *HTTPButton) Body() string                  { return b.body }
func (b *HTTPButton) SetBody(s string)              { b.body = s }
func (b *HTTPButton) ResultType() string            { return b.resultType }
func (b *HTTPButton) SetResultType(s string)        { b.resultType = strings.ToLower(strings.TrimSpace(s)) }
func (b *HTTPButton) JSONPointer() string           { return b.jsonPointer }
func (b *HTTPButton) SetJSONPointer(s string)       { b.jsonPointer = s }
func (b *HTTPButton) TimeoutMS() int                { return b.timeoutMS }
func (b *HTTPButton) SetTimeoutMS(v int)            { b.timeoutMS = max(1, min(300000, v)) }
func (b *HTTPButton) MaxResponseKB() int            { return b.maxResponseKB }
func (b *HTTPButton) SetMaxResponseKB(v int)        { b.maxResponseKB = max(1, min(65536, v)) }
func (b *HTTPButton) IsBusy() bool                  { return b.busy }
func (b *HTTPButton) LastResponse() *httpx.Response { return b.lastResponse }
func (b *HTTPButton) LastError() error              { return b.lastError }

// SetHeader overrides a design-time header at runtime. Use this for credentials;
// runtime headers are not enumerated, saved or emitted by the designer.
func (b *HTTPButton) SetHeader(name, value string) {
	if b.runtimeHeaders == nil {
		b.runtimeHeaders = make(http.Header)
	}
	b.runtimeHeaders.Set(name, value)
}
func (b *HTTPButton) RemoveHeader(name string) { b.runtimeHeaders.Del(name) }

// SigResult receives formatted text for every successful extraction. Typed
// signals additionally deliver int64/float64/bool without reparsing that text.
func (b *HTTPButton) SigResult(fn func(string))       { b.result = fn }
func (b *HTTPButton) SigIntResult(fn func(int64))     { b.intResult = fn }
func (b *HTTPButton) SigFloatResult(fn func(float64)) { b.floatResult = fn }
func (b *HTTPButton) SigBoolResult(fn func(bool))     { b.boolResult = fn }
func (b *HTTPButton) SigError(fn func(string))        { b.failed = fn }
func (b *HTTPButton) SigBusyChanged(fn func(bool))    { b.busyChanged = fn }
func (b *HTTPButton) setBusy(v bool) {
	if b.busy == v {
		return
	}
	b.busy = v
	b.Update()
	if b.busyChanged != nil {
		b.busyChanged(v)
	}
}
func (b *HTTPButton) fail(err error) error {
	b.lastError = err
	if b.failed != nil {
		b.failed(err.Error())
	}
	return err
}

type httpButtonResult struct {
	response *httpx.Response
	value    any
}

// Start returns admission/configuration errors immediately and also signals
// them via SigError. Request/HTTP/type errors arrive asynchronously. While busy
// it returns async.ErrBusy without disturbing the active job or emitting errors.
func (b *HTTPButton) Start(ctx context.Context) error {
	if b.closed {
		return async.ErrClosed
	}
	if b.busy {
		return async.ErrBusy
	}
	if !b.IsEnabled() {
		return errors.New("HTTPButton: disabled")
	}
	if ctx == nil {
		return b.fail(errors.New("HTTPButton: nil context"))
	}
	switch b.resultType {
	case "str", "json", "int", "float", "bool":
	default:
		return b.fail(errors.New("HTTPButton: result type must be str, json, int, float or bool"))
	}
	headers := make(http.Header)
	if strings.TrimSpace(b.headersJSON) != "" {
		var values map[string]any
		if err := json.Unmarshal([]byte(b.headersJSON), &values); err != nil || values == nil {
			return b.fail(errors.New("HTTPButton: headers must be a JSON object of strings"))
		}
		for name, value := range values {
			s, ok := value.(string)
			if !ok {
				return b.fail(errors.New("HTTPButton: header values must be strings"))
			}
			headers.Set(name, s)
		}
	}
	for k, v := range b.runtimeHeaders {
		headers[k] = append([]string(nil), v...)
	}
	request := httpx.Request{URL: b.url, Method: b.method, Header: headers, Body: []byte(b.body)}
	kind, pointer := b.resultType, b.jsonPointer
	client, err := httpx.NewClient(httpx.Options{Timeout: time.Duration(b.timeoutMS) * time.Millisecond, MaxResponseBytes: int64(b.maxResponseKB) * 1024})
	if err != nil {
		return b.fail(err)
	}
	child, cancel := context.WithCancel(ctx)
	root, owner := b.RootWidget(), b.OwnerWindow()
	b.generation++
	generation := b.generation
	f, err := async.Submit(child, nil, func(ctx context.Context) (httpButtonResult, error) {
		response, err := client.Do(ctx, request)
		out := httpButtonResult{response: response}
		if err != nil {
			return out, err
		}
		out.value, err = response.Extract(kind, pointer)
		return out, err
	})
	if err != nil {
		cancel()
		return b.fail(err)
	}
	b.cancel = cancel
	b.lastResponse = nil
	b.lastError = nil
	// Install notification before the busy signal, whose user callback may cancel,
	// close or even pump a nested event loop. GUI state is never read by work.
	_ = f.OnComplete(Post, func(out httpButtonResult, err error) {
		cancel()
		if b.closed || b.generation != generation {
			return
		}
		b.cancel = nil
		if b.RootWidget() != root || (owner != nil && b.OwnerWindow() != owner) {
			b.cancelPending(false)
			return
		}
		b.lastResponse = out.response
		b.lastError = err
		b.setBusy(false)
		if b.closed || b.generation != generation {
			return
		}
		if err != nil {
			b.fail(err)
			return
		}
		if b.result != nil {
			b.result(fmt.Sprint(out.value))
		}
		if b.closed || b.generation != generation {
			return
		}
		switch v := out.value.(type) {
		case int64:
			if b.intResult != nil {
				b.intResult(v)
			}
		case float64:
			if b.floatResult != nil {
				b.floatResult(v)
			}
		case bool:
			if b.boolResult != nil {
				b.boolResult(v)
			}
		}
	})
	b.setBusy(true)
	return nil
}

// Cancel invalidates already-queued completions and reports cancellation once.
func (b *HTTPButton) Cancel() { b.cancelPending(true) }
func (b *HTTPButton) cancelPending(notify bool) {
	b.generation++
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	wasBusy := b.busy
	generation := b.generation
	if notify {
		b.setBusy(false)
		if wasBusy && !b.closed && b.generation == generation {
			b.fail(context.Canceled)
		}
	} else {
		b.busy = false
		b.Update()
	}
}

// Close is idempotent, permanently disables requests and suppresses callbacks.
func (b *HTTPButton) Close() {
	if !b.closed {
		b.closed = true
		b.cancelPending(false)
	}
}
func (b *HTTPButton) SetParent(parent IWidget) {
	if parent != b.Parent() {
		b.cancelPending(false)
	}
	b.Button.SetParent(parent)
}
func (b *HTTPButton) Detach() { b.SetParent(nil) }
func (b *HTTPButton) Draw(g paint.Painter) {
	b.Button.Draw(g)
	if b.busy && b.Width() > 8 && b.Height() > 6 {
		g.SetBrush1(Theme().Accent)
		g.Rectangle(4, b.Height()-3, b.Width()-8, 2)
		g.Fill()
	}
}
func (b *HTTPButton) EnumProperties(list core.IPropertyList) {
	b.Button.EnumProperties(list)
	list.AddProperty("URL", b.URL, b.SetURL)
	list.AddProperty("HTTP Method", b.Method, b.SetMethod)
	list.AddProperty("Headers JSON", b.HeadersJSON, b.SetHeadersJSON)
	list.AddProperty("Request Body", b.Body, b.SetBody)
	list.AddProperty("Result Type", b.ResultType, b.SetResultType)
	list.AddProperty("JSON Pointer", b.JSONPointer, b.SetJSONPointer)
	list.AddProperty("Timeout ms", b.TimeoutMS, b.SetTimeoutMS)
	list.AddProperty("Max Response KB", b.MaxResponseKB, b.SetMaxResponseKB)
}

package core

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// 单元测试形态跟 dlv_test.go 对齐:
//   - 纯函数 (routeMessage / 写出形状) 直接喂构造好的对象, 不依赖子进程
//   - 子进程级别的 gopls 烟雾测试 gate 在 exec.LookPath("gopls") 上
//   - 路由级集成走 io.Pipe + in-process 假服务器, 不需要任何外部二进制

// -----------------------------------------------------------------------------
// routeMessage: 路由决策的纯单元测试
// -----------------------------------------------------------------------------

func TestLSPClientRouteMessage_ResolvesPending(t *testing.T) {
	pending := map[int]chan *LSPMessage{}
	ch := make(chan *LSPMessage, 1)
	pending[3] = ch
	notif := make(chan *LSPMessage, 1)

	id := json.RawMessage(`3`)
	m := &LSPMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  json.RawMessage(`{"capabilities":{}}`),
	}
	routeMessage(m, pending, notif)

	select {
	case got := <-ch:
		if got != m {
			t.Errorf("delivered message pointer mismatch")
		}
	default:
		t.Fatal("pending channel did not receive response")
	}
	if len(notif) != 0 {
		t.Errorf("notif chan got %d msgs, want 0", len(notif))
	}
}

func TestLSPClientRouteMessage_ResolvesPendingError(t *testing.T) {
	pending := map[int]chan *LSPMessage{}
	ch := make(chan *LSPMessage, 1)
	pending[7] = ch
	notif := make(chan *LSPMessage, 1)

	id := json.RawMessage(`7`)
	m := &LSPMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Error:   &LSPError{Code: -32601, Message: "Method not found"},
	}
	routeMessage(m, pending, notif)

	select {
	case got := <-ch:
		if got.Error == nil || got.Error.Code != -32601 {
			t.Errorf("unexpected error payload: %+v", got.Error)
		}
	default:
		t.Fatal("pending channel did not receive error response")
	}
}

func TestLSPClientRouteMessage_PushesNotification(t *testing.T) {
	pending := map[int]chan *LSPMessage{}
	notif := make(chan *LSPMessage, 1)

	m := &LSPMessage{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params:  json.RawMessage(`{"uri":"file:///a.go","diagnostics":[]}`),
	}
	routeMessage(m, pending, notif)

	select {
	case got := <-notif:
		if got.Method != "textDocument/publishDiagnostics" {
			t.Errorf("notif method = %q", got.Method)
		}
	default:
		t.Fatal("notification channel did not receive message")
	}
}

func TestLSPClientRouteMessage_UnknownID(t *testing.T) {
	pending := map[int]chan *LSPMessage{}
	notif := make(chan *LSPMessage, 1)

	id := json.RawMessage(`999`)
	m := &LSPMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  json.RawMessage(`{}`),
	}
	// 不应 panic
	routeMessage(m, pending, notif)

	if len(notif) != 0 {
		t.Errorf("notif chan got unexpected message")
	}
}

func TestLSPClientRouteMessage_StringID(t *testing.T) {
	pending := map[int]chan *LSPMessage{}
	notif := make(chan *LSPMessage, 1)

	id := json.RawMessage(`"some-string-id"`)
	m := &LSPMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  json.RawMessage(`{}`),
	}
	routeMessage(m, pending, notif)
	if len(notif) != 0 {
		t.Errorf("notif got unexpected msg from string-id response")
	}
}

func TestLSPClientRouteMessage_Nil(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("routeMessage(nil) panicked: %v", r)
		}
	}()
	routeMessage(nil, nil, nil)
}

func TestLSPClientRouteMessage_NotifFullDoesNotBlock(t *testing.T) {
	pending := map[int]chan *LSPMessage{}
	notif := make(chan *LSPMessage, 1)
	// 先把 notif 填满
	notif <- &LSPMessage{Method: "first"}

	m := &LSPMessage{JSONRPC: "2.0", Method: "second"}
	done := make(chan struct{})
	go func() {
		routeMessage(m, pending, notif)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("routeMessage blocked when notif chan was full")
	}
	got := <-notif
	if got.Method != "first" {
		t.Errorf("notif head method = %q, want 'first'", got.Method)
	}
}

func TestLSPClientRouteMessage_ServerRequestIgnored(t *testing.T) {
	pending := map[int]chan *LSPMessage{}
	ch := make(chan *LSPMessage, 1)
	pending[1] = ch
	notif := make(chan *LSPMessage, 1)

	id := json.RawMessage(`1`)
	m := &LSPMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "workspace/configuration",
		Params:  json.RawMessage(`{}`),
	}
	routeMessage(m, pending, notif)
	if len(ch) != 0 {
		t.Errorf("pending got a server-initiated request; should have been ignored")
	}
	if len(notif) != 0 {
		t.Errorf("notif got a server-initiated request; should have been ignored")
	}
}

// -----------------------------------------------------------------------------
// LaunchLSPClient: 错误路径 (不真起子进程)
// -----------------------------------------------------------------------------

func TestLaunchLSPClient_NotABinary(t *testing.T) {
	start := time.Now()
	c, err := LaunchLSPClient("definitely-not-a-binary-zzz")
	elapsed := time.Since(start)
	if err == nil {
		_ = c.Close()
		t.Fatal("expected error launching nonexistent binary, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("LaunchLSPClient took %v, too slow on missing binary", elapsed)
	}
}

func TestLaunchLSPClient_EmptyCmd(t *testing.T) {
	_, err := LaunchLSPClient("")
	if err == nil {
		t.Fatal("expected error for empty server cmd, got nil")
	}
}

// -----------------------------------------------------------------------------
// SendRequest / SendNotification: closed 早退 + 输入校验
// -----------------------------------------------------------------------------

func TestLSPClientSendRequest_OnClosed(t *testing.T) {
	c := &LSPClient{closed: true, pending: map[int]chan *LSPMessage{}}
	_, err := c.SendRequest("textDocument/hover", nil)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}

func TestLSPClientSendNotification_OnClosed(t *testing.T) {
	c := &LSPClient{closed: true}
	err := c.SendNotification("textDocument/didChange", nil)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("want closed error, got %v", err)
	}
}

func TestLSPClientSendRequest_EmptyMethod(t *testing.T) {
	c := &LSPClient{pending: map[int]chan *LSPMessage{}}
	_, err := c.SendRequest("", nil)
	if err == nil {
		t.Fatal("expected error on empty method, got nil")
	}
}

func TestLSPClientSendNotification_EmptyMethod(t *testing.T) {
	c := &LSPClient{}
	err := c.SendNotification("", nil)
	if err == nil {
		t.Fatal("expected error on empty method, got nil")
	}
}

// -----------------------------------------------------------------------------
// 写出形状: SendNotification / DidOpen 真把字节写到 stdin pipe
// -----------------------------------------------------------------------------

// memWriteCloser 是个 io.WriteCloser 的内存实现, 用来当 stdin pipe 接收测试输出
type memWriteCloser struct {
	*bytes.Buffer
}

func (p *memWriteCloser) Close() error { return nil }

func TestLSPClientSendNotification_WritesValidFrame(t *testing.T) {
	buf := &memWriteCloser{Buffer: &bytes.Buffer{}}
	c := &LSPClient{
		stdin:   buf,
		pending: map[int]chan *LSPMessage{},
	}
	type p struct {
		URI string `json:"uri"`
	}
	if err := c.SendNotification("textDocument/didSave", p{URI: "file:///x.go"}); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "Content-Length: ") {
		t.Fatalf("output missing Content-Length prefix: %q", out)
	}
	if !strings.Contains(out, `"textDocument/didSave"`) {
		t.Errorf("output missing method: %q", out)
	}
	if !strings.Contains(out, `"file:///x.go"`) {
		t.Errorf("output missing uri: %q", out)
	}
}

func TestLSPClientDidOpen_Shape(t *testing.T) {
	buf := &memWriteCloser{Buffer: &bytes.Buffer{}}
	c := &LSPClient{
		stdin:   buf,
		pending: map[int]chan *LSPMessage{},
	}
	if err := c.DidOpen("file:///a.go", "go", "package main\n", 1); err != nil {
		t.Fatalf("DidOpen: %v", err)
	}
	out := buf.String()
	idx := strings.Index(out, "\r\n\r\n")
	if idx < 0 {
		t.Fatalf("output missing header terminator: %q", out)
	}
	body := out[idx+4:]
	var got struct {
		Method string `json:"method"`
		Params struct {
			TextDocument struct {
				URI        string `json:"uri"`
				LanguageID string `json:"languageId"`
				Version    int    `json:"version"`
				Text       string `json:"text"`
			} `json:"textDocument"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body not valid JSON: %v (%q)", err, body)
	}
	if got.Method != "textDocument/didOpen" {
		t.Errorf("method = %q", got.Method)
	}
	if got.Params.TextDocument.URI != "file:///a.go" {
		t.Errorf("uri = %q", got.Params.TextDocument.URI)
	}
	if got.Params.TextDocument.LanguageID != "go" {
		t.Errorf("languageId = %q", got.Params.TextDocument.LanguageID)
	}
	if got.Params.TextDocument.Version != 1 {
		t.Errorf("version = %d", got.Params.TextDocument.Version)
	}
	if got.Params.TextDocument.Text != "package main\n" {
		t.Errorf("text = %q", got.Params.TextDocument.Text)
	}
}

// -----------------------------------------------------------------------------
// Notifications channel + failAllPending
// -----------------------------------------------------------------------------

func TestLSPClientNotificationsChannel(t *testing.T) {
	c := &LSPClient{notifications: make(chan *LSPMessage, 4)}
	if c.Notifications() == nil {
		t.Fatal("Notifications() returned nil channel")
	}
	c.notifications <- &LSPMessage{Method: "test/event"}
	select {
	case m := <-c.Notifications():
		if m.Method != "test/event" {
			t.Errorf("got method %q, want test/event", m.Method)
		}
	default:
		t.Fatal("could not read from Notifications() channel")
	}
}

func TestLSPClientFailAllPending(t *testing.T) {
	c := &LSPClient{pending: map[int]chan *LSPMessage{}}
	ch1 := make(chan *LSPMessage, 1)
	ch2 := make(chan *LSPMessage, 1)
	c.pending[1] = ch1
	c.pending[2] = ch2

	c.failAllPending()

	for i, ch := range []chan *LSPMessage{ch1, ch2} {
		select {
		case m := <-ch:
			if m.Error == nil {
				t.Errorf("pending %d got msg without Error", i+1)
			}
		default:
			t.Errorf("pending %d did not receive failure stub", i+1)
		}
	}

	c.mu.Lock()
	n := len(c.pending)
	c.mu.Unlock()
	if n != 0 {
		t.Errorf("pending map size = %d after failAll, want 0", n)
	}
}

// TestLSPClientNotifications_ClosedOnReadLoopExit: server 端关掉写 pipe ->
// readLoop 收 EOF 退出 -> Notifications() 通道必须被 close, 上层的
// `for range notifs` 才能终止. 修复前该通道永不 close, 宿主每重启一次 client
// 就泄漏一个 drain goroutine (外加被换下的旧 client).
func TestLSPClientNotifications_ClosedOnReadLoopExit(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() {
		_ = srvIn.Close()
		_ = c.Close()
	}()

	// 先推一条通知, 验证 close 之前已缓冲的消息仍能被读到 (close 不吞数据)
	if err := WriteLSPMessage(srvOut, &LSPMessage{
		JSONRPC: "2.0",
		Method:  "window/logMessage",
		Params:  json.RawMessage(`{"type":3,"message":"hi"}`),
	}); err != nil {
		t.Fatalf("write notification: %v", err)
	}

	// 关 server 写端 -> client.stdout EOF -> readLoop 退出并 close(notifications)
	_ = srvOut.Close()

	// 通道必须在超时内变成 closed: 先读到缓冲的那条, 随后 receive 返回 ok=false
	deadline := time.After(2 * time.Second)
	var got []*LSPMessage
	for {
		select {
		case m, ok := <-c.Notifications():
			if !ok {
				if len(got) != 1 || got[0].Method != "window/logMessage" {
					t.Errorf("buffered notifications before close = %+v, want the single logMessage", got)
				}
				return
			}
			got = append(got, m)
		case <-deadline:
			t.Fatal("Notifications() channel never closed after read loop exit")
		}
	}
}

// -----------------------------------------------------------------------------
// Round-trip via io.Pipe + 内嵌假服务器: 覆盖完整 readLoop + SendRequest 路径
// -----------------------------------------------------------------------------

// TestLSPClientSendRequest_RoundTrip 在 in-process 上验证一条 SendRequest:
// 写到 stdin pipe -> 假服务器读出来 -> 写回响应 -> 我们这边的 readLoop 路由
// -> SendRequest 的等待方拿到响应.
func TestLSPClientSendRequest_RoundTrip(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	srvReader := bufio.NewReader(srvIn)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		for {
			m, err := ReadLSPMessage(srvReader)
			if err != nil {
				return
			}
			if m.ID == nil {
				continue // 通知不回
			}
			resp := &LSPMessage{
				JSONRPC: "2.0",
				ID:      m.ID,
				Result:  json.RawMessage(`{"ok":true}`),
			}
			if err := WriteLSPMessage(srvOut, resp); err != nil {
				return
			}
		}
	}()

	resp, err := c.SendRequest("test/echo", map[string]int{"a": 1})
	if err != nil {
		t.Fatalf("SendRequest: %v", err)
	}
	var got struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !got.OK {
		t.Errorf("result ok = false")
	}

	// 收尾: 关 pipe, 让 readLoop / fake server 都退
	_ = srvIn.Close()
	_ = srvOut.Close()
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Errorf("fake server did not exit")
	}
}

// TestLSPClientSendRequest_AbortedByReadLoopExit 服务器从来不回, 而是直接断开
// 它写给客户端的那根 pipe (模拟 gopls 子进程崩溃). 客户端这边的 readLoop
// 应该退出并把所有 pending 失败, SendRequest 立刻拿到 "connection closed" 错误.
// 这条路径覆盖 failAllPending + done chan 的语义.
func TestLSPClientSendRequest_AbortedByReadLoopExit(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() {
		_ = srvIn.Close()
		_ = c.Close()
	}()

	// drain server 端的请求字节, 否则 client 写 stdin pipe 会阻塞
	go func() {
		br := bufio.NewReader(srvIn)
		for {
			if _, err := ReadLSPMessage(br); err != nil {
				return
			}
		}
	}()

	done := make(chan error, 1)
	go func() {
		_, err := c.SendRequest("test/never-replies", nil)
		done <- err
	}()

	// 等到 pending 被注册再断 pipe
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		n := len(c.pending)
		c.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	c.mu.Lock()
	n := len(c.pending)
	c.mu.Unlock()
	if n < 1 {
		t.Fatalf("pending entry never registered (n=%d)", n)
	}

	// 关 server 的写端 -> client.stdout 收到 EOF -> readLoop 退 ->
	// failAllPending 推桩 -> SendRequest 拿到错误
	_ = srvOut.Close()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("SendRequest returned nil after server pipe closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendRequest did not unblock after server pipe closed")
	}

	c.mu.Lock()
	n = len(c.pending)
	c.mu.Unlock()
	if n != 0 {
		t.Errorf("pending after read-loop exit = %d, want 0", n)
	}
}

// TestLSPClientSendRequest_Concurrent 并发场景: 多条 SendRequest 同时跑,
// 每条要拿到自己的响应, 不串号.
func TestLSPClientSendRequest_Concurrent(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	// fake server: 把每条请求的 id 原样回送在 result.id 字段里
	srvReader := bufio.NewReader(srvIn)
	var srvWG sync.WaitGroup
	srvWG.Add(1)
	go func() {
		defer srvWG.Done()
		for {
			m, err := ReadLSPMessage(srvReader)
			if err != nil {
				return
			}
			if m.ID == nil {
				continue
			}
			// 把原始 id 嵌进 result 里, 这样客户端能验证响应跟请求 id 对应
			result := []byte(`{"echo_id":`)
			result = append(result, []byte(*m.ID)...)
			result = append(result, '}')
			resp := &LSPMessage{
				JSONRPC: "2.0",
				ID:      m.ID,
				Result:  json.RawMessage(result),
			}
			if err := WriteLSPMessage(srvOut, resp); err != nil {
				return
			}
		}
	}()

	const N = 16
	var wg sync.WaitGroup
	wg.Add(N)
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			resp, err := c.SendRequest("test/echo", nil)
			if err != nil {
				errs <- err
				return
			}
			var got struct {
				EchoID int `json:"echo_id"`
			}
			if err := json.Unmarshal(resp.Result, &got); err != nil {
				errs <- err
				return
			}
			// 服务器把请求 id 原样回填; 校验它跟响应的 ID 一致
			var respID int
			_ = json.Unmarshal(*resp.ID, &respID)
			if respID != got.EchoID {
				errs <- nil // 串号视为失败
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Errorf("SendRequest err: %v", e)
		}
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	srvWG.Wait()
}

// -----------------------------------------------------------------------------
// 类型化便利封装: Completion / Hover / Definition / References / DidChange
// 共用 newPipedClient + runFakeServer 把 in-process 假服务器复用起来
// -----------------------------------------------------------------------------

// runFakeServer 是一个通用的"假 LSP 服务器", 单线程, 读到请求后调 reply 拿响应字节
// reply 是 ((method, id, params) -> rawResult OR rawError) 的形状; nil 表示"不回".
// 返回的 done 在 server 自然退出 (pipe 关闭) 后关闭, 方便 test 收尾.
type fakeReply struct {
	Result json.RawMessage // 非 nil 则当 result; 优先级高于 Err
	Err    *LSPError       // 非 nil 则当 error
}

func runFakeServer(t *testing.T, srvIn io.ReadCloser, srvOut io.WriteCloser, reply func(method string, id *json.RawMessage, params json.RawMessage) *fakeReply) chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		br := bufio.NewReader(srvIn)
		for {
			m, err := ReadLSPMessage(br)
			if err != nil {
				return
			}
			if m.ID == nil {
				continue // 通知不回
			}
			r := reply(m.Method, m.ID, m.Params)
			if r == nil {
				continue
			}
			resp := &LSPMessage{
				JSONRPC: "2.0",
				ID:      m.ID,
				Result:  r.Result,
				Error:   r.Err,
			}
			if err := WriteLSPMessage(srvOut, resp); err != nil {
				return
			}
		}
	}()
	return done
}

func TestLSPClientCompletion_CompletionListShape(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(method string, id *json.RawMessage, _ json.RawMessage) *fakeReply {
		if method != "textDocument/completion" {
			t.Errorf("method = %q, want textDocument/completion", method)
		}
		return &fakeReply{Result: json.RawMessage(`{"isIncomplete":false,"items":[{"label":"Println","detail":"func(a ...any)","kind":3,"insertText":"Println"},{"label":"Print","kind":3}]}`)}
	})

	items, err := c.Completion("file:///a.go", 10, 4)
	if err != nil {
		t.Fatalf("Completion: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(items), items)
	}
	if items[0].Label != "Println" || items[0].Detail != "func(a ...any)" || items[0].Kind != 3 || items[0].InsertText != "Println" {
		t.Errorf("items[0] = %+v", items[0])
	}
	if items[1].Label != "Print" || items[1].Kind != 3 {
		t.Errorf("items[1] = %+v", items[1])
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientCompletion_RawArrayShape(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`[{"label":"Foo"},{"label":"Bar","detail":"int"}]`)}
	})

	items, err := c.Completion("file:///a.go", 0, 0)
	if err != nil {
		t.Fatalf("Completion: %v", err)
	}
	if len(items) != 2 || items[0].Label != "Foo" || items[1].Label != "Bar" || items[1].Detail != "int" {
		t.Fatalf("items = %+v", items)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientCompletion_NullResult(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`null`)}
	})

	items, err := c.Completion("file:///a.go", 0, 0)
	if err != nil {
		t.Fatalf("Completion: %v", err)
	}
	if items != nil {
		t.Errorf("items = %+v, want nil", items)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientHover_StringContents(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(method string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		if method != "textDocument/hover" {
			t.Errorf("method = %q", method)
		}
		return &fakeReply{Result: json.RawMessage(`{"contents":"func Println(a ...any)"}`)}
	})

	h, err := c.Hover("file:///a.go", 1, 2)
	if err != nil {
		t.Fatalf("Hover: %v", err)
	}
	if h == nil {
		t.Fatal("Hover returned nil")
	}
	if h.Contents != "func Println(a ...any)" {
		t.Errorf("Contents = %q", h.Contents)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientHover_MarkupObjectContents(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`{"contents":{"kind":"markdown","value":"func Println(a ...any)"}}`)}
	})

	h, err := c.Hover("file:///a.go", 1, 2)
	if err != nil {
		t.Fatalf("Hover: %v", err)
	}
	if h == nil || h.Contents != "func Println(a ...any)" {
		t.Errorf("Contents = %+v", h)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientHover_ArrayContents(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		// 数组里混 string 和 MarkedString 对象 (LSP 规范允许)
		return &fakeReply{Result: json.RawMessage(`{"contents":["line1",{"language":"go","value":"line2"}]}`)}
	})

	h, err := c.Hover("file:///a.go", 1, 2)
	if err != nil {
		t.Fatalf("Hover: %v", err)
	}
	if h == nil || h.Contents != "line1\nline2" {
		t.Errorf("Contents = %+v", h)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientHover_NullResult(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`null`)}
	})

	h, err := c.Hover("file:///a.go", 1, 2)
	if err != nil {
		t.Fatalf("Hover: %v", err)
	}
	if h != nil {
		t.Errorf("Hover = %+v, want nil", h)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientDefinition_SingleLocation(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(method string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		if method != "textDocument/definition" {
			t.Errorf("method = %q", method)
		}
		return &fakeReply{Result: json.RawMessage(`{"uri":"file:///b.go","range":{"start":{"line":3,"character":4},"end":{"line":3,"character":10}}}`)}
	})

	locs, err := c.Definition("file:///a.go", 1, 2)
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(locs), locs)
	}
	got := locs[0]
	if got.URI != "file:///b.go" || got.Range.Start.Line != 3 || got.Range.End.Character != 10 {
		t.Errorf("loc = %+v", got)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientDefinition_ArrayLocations(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`[
{"uri":"file:///b.go","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}}},
{"uri":"file:///c.go","range":{"start":{"line":5,"character":2},"end":{"line":5,"character":7}}}
]`)}
	})

	locs, err := c.Definition("file:///a.go", 1, 2)
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(locs) != 2 || locs[0].URI != "file:///b.go" || locs[1].URI != "file:///c.go" || locs[1].Range.Start.Line != 5 {
		t.Fatalf("locs = %+v", locs)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientDefinition_NullResult(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`null`)}
	})

	locs, err := c.Definition("file:///a.go", 1, 2)
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if locs != nil {
		t.Errorf("locs = %+v, want nil", locs)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientReferences_Locations(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var capturedParams json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(method string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		if method != "textDocument/references" {
			t.Errorf("method = %q", method)
		}
		capturedParams = append(capturedParams[:0:0], params...)
		return &fakeReply{Result: json.RawMessage(`[
{"uri":"file:///a.go","range":{"start":{"line":1,"character":2},"end":{"line":1,"character":5}}},
{"uri":"file:///b.go","range":{"start":{"line":7,"character":0},"end":{"line":7,"character":3}}}
]`)}
	})

	locs, err := c.References("file:///a.go", 1, 2, true)
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if len(locs) != 2 || locs[0].URI != "file:///a.go" || locs[1].URI != "file:///b.go" {
		t.Fatalf("locs = %+v", locs)
	}
	// 校验 includeDeclaration 真的被序列化到了 params.context 里
	var got struct {
		Context struct {
			IncludeDeclaration bool `json:"includeDeclaration"`
		} `json:"context"`
		Position LSPPosition `json:"position"`
	}
	if err := json.Unmarshal(capturedParams, &got); err != nil {
		t.Fatalf("decode captured params: %v", err)
	}
	if !got.Context.IncludeDeclaration {
		t.Errorf("includeDeclaration = false, want true")
	}
	if got.Position.Line != 1 || got.Position.Character != 2 {
		t.Errorf("position = %+v", got.Position)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientReferences_IncludeDeclFalse(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var capturedParams json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		capturedParams = append(capturedParams[:0:0], params...)
		return &fakeReply{Result: json.RawMessage(`[]`)}
	})

	if _, err := c.References("file:///a.go", 0, 0, false); err != nil {
		t.Fatalf("References: %v", err)
	}
	var got struct {
		Context struct {
			IncludeDeclaration bool `json:"includeDeclaration"`
		} `json:"context"`
	}
	if err := json.Unmarshal(capturedParams, &got); err != nil {
		t.Fatalf("decode captured params: %v", err)
	}
	if got.Context.IncludeDeclaration {
		t.Errorf("includeDeclaration = true, want false")
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientCompletion_ServerError(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Err: &LSPError{Code: -32603, Message: "internal error"}}
	})

	items, err := c.Completion("file:///a.go", 1, 2)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if items != nil {
		t.Errorf("items = %+v, want nil on error", items)
	}
	if !strings.Contains(err.Error(), "internal error") {
		t.Errorf("err = %v, want server message", err)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientDidChange_Shape(t *testing.T) {
	buf := &memWriteCloser{Buffer: &bytes.Buffer{}}
	c := &LSPClient{
		stdin:   buf,
		pending: map[int]chan *LSPMessage{},
	}
	if err := c.DidChange("file:///a.go", 7, "package main\n// changed\n"); err != nil {
		t.Fatalf("DidChange: %v", err)
	}
	out := buf.String()
	idx := strings.Index(out, "\r\n\r\n")
	if idx < 0 {
		t.Fatalf("output missing header terminator: %q", out)
	}
	body := out[idx+4:]
	var got struct {
		Method string `json:"method"`
		Params struct {
			TextDocument struct {
				URI     string `json:"uri"`
				Version int    `json:"version"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body not valid JSON: %v (%q)", err, body)
	}
	if got.Method != "textDocument/didChange" {
		t.Errorf("method = %q", got.Method)
	}
	if got.Params.TextDocument.URI != "file:///a.go" {
		t.Errorf("uri = %q", got.Params.TextDocument.URI)
	}
	if got.Params.TextDocument.Version != 7 {
		t.Errorf("version = %d", got.Params.TextDocument.Version)
	}
	if len(got.Params.ContentChanges) != 1 || got.Params.ContentChanges[0].Text != "package main\n// changed\n" {
		t.Errorf("contentChanges = %+v", got.Params.ContentChanges)
	}
}

// -----------------------------------------------------------------------------
// DocumentSymbol: hierarchical DocumentSymbol vs flat SymbolInformation 双形态
// -----------------------------------------------------------------------------

func TestLSPClientDocumentSymbol_DocumentSymbolShape(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var capturedParams json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(method string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		if method != "textDocument/documentSymbol" {
			t.Errorf("method = %q, want textDocument/documentSymbol", method)
		}
		capturedParams = append(capturedParams[:0:0], params...)
		// 一个父 (struct) 带两个子 (字段). hierarchical 形态: 有 range/selectionRange, 没 location.
		return &fakeReply{Result: json.RawMessage(`[
{"name":"Server","detail":"struct{...}","kind":23,
 "range":{"start":{"line":4,"character":0},"end":{"line":8,"character":1}},
 "selectionRange":{"start":{"line":4,"character":5},"end":{"line":4,"character":11}},
 "children":[
   {"name":"Host","detail":"string","kind":8,
    "range":{"start":{"line":5,"character":1},"end":{"line":5,"character":13}},
    "selectionRange":{"start":{"line":5,"character":1},"end":{"line":5,"character":5}}},
   {"name":"Port","detail":"int","kind":8,
    "range":{"start":{"line":6,"character":1},"end":{"line":6,"character":9}},
    "selectionRange":{"start":{"line":6,"character":1},"end":{"line":6,"character":5}}}
 ]}
]`)}
	})

	syms, err := c.DocumentSymbol("file:///a.go")
	if err != nil {
		t.Fatalf("DocumentSymbol: %v", err)
	}
	if len(syms) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(syms), syms)
	}
	parent := syms[0]
	if parent.Name != "Server" || parent.Detail != "struct{...}" || parent.Kind != 23 || parent.Line != 4 {
		t.Errorf("parent = %+v", parent)
	}
	if len(parent.Children) != 2 {
		t.Fatalf("children len = %d, want 2: %+v", len(parent.Children), parent.Children)
	}
	if parent.Children[0].Name != "Host" || parent.Children[0].Line != 5 {
		t.Errorf("children[0] = %+v", parent.Children[0])
	}
	if parent.Children[1].Name != "Port" || parent.Children[1].Line != 6 {
		t.Errorf("children[1] = %+v", parent.Children[1])
	}
	// params 只带 textDocument, 没有 position
	var gotParams struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position *LSPPosition `json:"position"`
	}
	if err := json.Unmarshal(capturedParams, &gotParams); err != nil {
		t.Fatalf("decode captured params: %v", err)
	}
	if gotParams.TextDocument.URI != "file:///a.go" {
		t.Errorf("params uri = %q", gotParams.TextDocument.URI)
	}
	if gotParams.Position != nil {
		t.Errorf("documentSymbol params carried a position: %+v", gotParams.Position)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientDocumentSymbol_SymbolInformationShape(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	// legacy 扁平形态: 每条带 location, 没有 children. Line 取 location.range.start.line.
	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`[
{"name":"main","kind":12,"location":{"uri":"file:///a.go","range":{"start":{"line":10,"character":0},"end":{"line":12,"character":1}}}},
{"name":"Config","kind":23,"location":{"uri":"file:///a.go","range":{"start":{"line":2,"character":0},"end":{"line":5,"character":1}}},"containerName":""}
]`)}
	})

	syms, err := c.DocumentSymbol("file:///a.go")
	if err != nil {
		t.Fatalf("DocumentSymbol: %v", err)
	}
	if len(syms) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(syms), syms)
	}
	if syms[0].Name != "main" || syms[0].Kind != 12 || syms[0].Line != 10 {
		t.Errorf("syms[0] = %+v", syms[0])
	}
	if syms[1].Name != "Config" || syms[1].Kind != 23 || syms[1].Line != 2 {
		t.Errorf("syms[1] = %+v", syms[1])
	}
	// SymbolInformation 形态是扁平的, Children 必须为空
	if len(syms[0].Children) != 0 || len(syms[1].Children) != 0 {
		t.Errorf("symbolInformation entries should have no children: %+v", syms)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientDocumentSymbol_NullResult(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`null`)}
	})

	syms, err := c.DocumentSymbol("file:///a.go")
	if err != nil {
		t.Fatalf("DocumentSymbol: %v", err)
	}
	if len(syms) != 0 {
		t.Errorf("syms = %+v, want empty", syms)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientDocumentSymbol_EmptyArray(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`[]`)}
	})

	syms, err := c.DocumentSymbol("file:///a.go")
	if err != nil {
		t.Fatalf("DocumentSymbol: %v", err)
	}
	if len(syms) != 0 {
		t.Errorf("syms = %+v, want empty", syms)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientDocumentSymbol_ServerError(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Err: &LSPError{Code: -32603, Message: "symbol index unavailable"}}
	})

	syms, err := c.DocumentSymbol("file:///a.go")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if syms != nil {
		t.Errorf("syms = %+v, want nil on error", syms)
	}
	if !strings.Contains(err.Error(), "symbol index unavailable") {
		t.Errorf("err = %v, want server message", err)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// -----------------------------------------------------------------------------
// WorkspaceSymbol: 项目级符号搜索 (Cmd+T). SymbolInformation vs WorkspaceSymbol
// 两种 location 形态, query 透传, null/空数组 -> 空切片, error
// -----------------------------------------------------------------------------

func TestLSPClientWorkspaceSymbol_SymbolInformationShape(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var capturedParams json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(method string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		if method != "workspace/symbol" {
			t.Errorf("method = %q, want workspace/symbol", method)
		}
		capturedParams = append(capturedParams[:0:0], params...)
		// 标准 SymbolInformation: 每条带完整 location{uri, range} + containerName.
		return &fakeReply{Result: json.RawMessage(`[
{"name":"Server","kind":23,"containerName":"core","location":{"uri":"file:///core/server.go","range":{"start":{"line":10,"character":5},"end":{"line":10,"character":11}}}},
{"name":"Serve","kind":12,"containerName":"core.Server","location":{"uri":"file:///core/server.go","range":{"start":{"line":20,"character":0},"end":{"line":22,"character":1}}}}
]`)}
	})

	syms, err := c.WorkspaceSymbol("Foo")
	if err != nil {
		t.Fatalf("WorkspaceSymbol: %v", err)
	}
	if len(syms) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(syms), syms)
	}
	if syms[0].Name != "Server" || syms[0].Kind != 23 || syms[0].ContainerName != "core" ||
		syms[0].URI != "file:///core/server.go" || syms[0].Line != 10 || syms[0].Character != 5 {
		t.Errorf("syms[0] = %+v", syms[0])
	}
	if syms[1].Name != "Serve" || syms[1].Kind != 12 || syms[1].ContainerName != "core.Server" ||
		syms[1].URI != "file:///core/server.go" || syms[1].Line != 20 {
		t.Errorf("syms[1] = %+v", syms[1])
	}
	// query 透传: params 必须是 {"query":"Foo"}
	var gotParams struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(capturedParams, &gotParams); err != nil {
		t.Fatalf("decode captured params: %v", err)
	}
	if gotParams.Query != "Foo" {
		t.Errorf("query = %q, want Foo", gotParams.Query)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// WorkspaceSymbol (现代形态): location 可能只带 uri, range 省略 -> Line/Character 落 0.
// 混一条只有 {uri} 的和一条带完整 range 的, 校验两种都解得动.
func TestLSPClientWorkspaceSymbol_WorkspaceSymbolShape(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`[
{"name":"Handler","kind":11,"containerName":"api","location":{"uri":"file:///api/h.go"}},
{"name":"Handle","kind":12,"containerName":"api","location":{"uri":"file:///api/h.go","range":{"start":{"line":3,"character":6},"end":{"line":3,"character":12}}}}
]`)}
	})

	syms, err := c.WorkspaceSymbol("Handle")
	if err != nil {
		t.Fatalf("WorkspaceSymbol: %v", err)
	}
	if len(syms) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(syms), syms)
	}
	// 只有 uri, 没有 range -> Line/Character 默认 0, URI 仍解出来.
	if syms[0].Name != "Handler" || syms[0].URI != "file:///api/h.go" ||
		syms[0].Line != 0 || syms[0].Character != 0 {
		t.Errorf("syms[0] (uri-only location) = %+v", syms[0])
	}
	// 带完整 range 的照常解出坐标.
	if syms[1].Name != "Handle" || syms[1].Line != 3 || syms[1].Character != 6 {
		t.Errorf("syms[1] (full location) = %+v", syms[1])
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientWorkspaceSymbol_NullResult(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	// 空 query 走"全部符号"路径; server 回 null -> 空切片.
	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`null`)}
	})

	syms, err := c.WorkspaceSymbol("")
	if err != nil {
		t.Fatalf("WorkspaceSymbol: %v", err)
	}
	if syms == nil || len(syms) != 0 {
		t.Errorf("syms = %+v, want empty non-nil slice", syms)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientWorkspaceSymbol_EmptyArray(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`[]`)}
	})

	syms, err := c.WorkspaceSymbol("Nope")
	if err != nil {
		t.Fatalf("WorkspaceSymbol: %v", err)
	}
	if syms == nil || len(syms) != 0 {
		t.Errorf("syms = %+v, want empty non-nil slice", syms)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientWorkspaceSymbol_ServerError(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Err: &LSPError{Code: -32603, Message: "workspace symbol index unavailable"}}
	})

	syms, err := c.WorkspaceSymbol("Foo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if syms != nil {
		t.Errorf("syms = %+v, want nil on error", syms)
	}
	if !strings.Contains(err.Error(), "workspace symbol index unavailable") {
		t.Errorf("err = %v, want server message", err)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// -----------------------------------------------------------------------------
// SignatureHelp: 签名 + 形参 + activeParameter, documentation 多态压平, null, error
// -----------------------------------------------------------------------------

func TestLSPClientSignatureHelp_Basic(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var capturedParams json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(method string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		if method != "textDocument/signatureHelp" {
			t.Errorf("method = %q, want textDocument/signatureHelp", method)
		}
		capturedParams = append(capturedParams[:0:0], params...)
		return &fakeReply{Result: json.RawMessage(`{
"signatures":[
  {"label":"Atoi(s string) (int, error)","documentation":"Atoi converts a string to an int.",
   "parameters":[{"label":"s string"},{"label":"base int"}]}
],
"activeSignature":0,
"activeParameter":1
}`)}
	})

	sh, err := c.SignatureHelp("file:///a.go", 3, 9)
	if err != nil {
		t.Fatalf("SignatureHelp: %v", err)
	}
	if sh == nil {
		t.Fatal("SignatureHelp returned nil")
	}
	if len(sh.Signatures) != 1 {
		t.Fatalf("signatures len = %d, want 1: %+v", len(sh.Signatures), sh.Signatures)
	}
	sig := sh.Signatures[0]
	if sig.Label != "Atoi(s string) (int, error)" {
		t.Errorf("label = %q", sig.Label)
	}
	if sig.Documentation != "Atoi converts a string to an int." {
		t.Errorf("documentation = %q", sig.Documentation)
	}
	if len(sig.Parameters) != 2 || sig.Parameters[0] != "s string" || sig.Parameters[1] != "base int" {
		t.Errorf("parameters = %+v", sig.Parameters)
	}
	if sh.ActiveSignature != 0 {
		t.Errorf("activeSignature = %d, want 0", sh.ActiveSignature)
	}
	if sh.ActiveParameter != 1 {
		t.Errorf("activeParameter = %d, want 1", sh.ActiveParameter)
	}
	// params 跟 hover 同形: 带 position
	var gotParams struct {
		Position LSPPosition `json:"position"`
	}
	if err := json.Unmarshal(capturedParams, &gotParams); err != nil {
		t.Fatalf("decode captured params: %v", err)
	}
	if gotParams.Position.Line != 3 || gotParams.Position.Character != 9 {
		t.Errorf("position = %+v", gotParams.Position)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientSignatureHelp_MarkupDocumentation(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	// documentation 走 MarkupContent{kind, value} 形态, 应被压平成 value
	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`{
"signatures":[
  {"label":"Println(a ...any)","documentation":{"kind":"markdown","value":"writes to stdout"},
   "parameters":[{"label":"a ...any"}]}
],
"activeSignature":0,
"activeParameter":0
}`)}
	})

	sh, err := c.SignatureHelp("file:///a.go", 1, 2)
	if err != nil {
		t.Fatalf("SignatureHelp: %v", err)
	}
	if sh == nil || len(sh.Signatures) != 1 {
		t.Fatalf("sh = %+v", sh)
	}
	if sh.Signatures[0].Documentation != "writes to stdout" {
		t.Errorf("documentation = %q, want stringified markup value", sh.Signatures[0].Documentation)
	}
	if len(sh.Signatures[0].Parameters) != 1 || sh.Signatures[0].Parameters[0] != "a ...any" {
		t.Errorf("parameters = %+v", sh.Signatures[0].Parameters)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientSignatureHelp_NullResult(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`null`)}
	})

	sh, err := c.SignatureHelp("file:///a.go", 1, 2)
	if err != nil {
		t.Fatalf("SignatureHelp: %v", err)
	}
	if sh != nil {
		t.Errorf("sh = %+v, want nil", sh)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientSignatureHelp_ServerError(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Err: &LSPError{Code: -32603, Message: "no signature here"}}
	})

	sh, err := c.SignatureHelp("file:///a.go", 1, 2)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if sh != nil {
		t.Errorf("sh = %+v, want nil on error", sh)
	}
	if !strings.Contains(err.Error(), "no signature here") {
		t.Errorf("err = %v, want server message", err)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// -----------------------------------------------------------------------------
// Formatting: []TextEdit, null -> 空切片, error
// -----------------------------------------------------------------------------

func TestLSPClientFormatting_Edits(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var capturedParams json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(method string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		if method != "textDocument/formatting" {
			t.Errorf("method = %q, want textDocument/formatting", method)
		}
		capturedParams = append(capturedParams[:0:0], params...)
		return &fakeReply{Result: json.RawMessage(`[
{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":4}},"newText":"\t"},
{"range":{"start":{"line":2,"character":0},"end":{"line":2,"character":2}},"newText":""}
]`)}
	})

	edits, err := c.Formatting("file:///a.go")
	if err != nil {
		t.Fatalf("Formatting: %v", err)
	}
	if len(edits) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(edits), edits)
	}
	if edits[0].NewText != "\t" || edits[0].Range.End.Character != 4 {
		t.Errorf("edits[0] = %+v", edits[0])
	}
	if edits[1].NewText != "" || edits[1].Range.Start.Line != 2 || edits[1].Range.End.Character != 2 {
		t.Errorf("edits[1] = %+v", edits[1])
	}
	// 校验 options 真带了 tabSize/insertSpaces, 且没有 position (formatting 不是 position-based)
	var gotParams struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Options struct {
			TabSize      int  `json:"tabSize"`
			InsertSpaces bool `json:"insertSpaces"`
		} `json:"options"`
		Position *LSPPosition `json:"position"`
	}
	if err := json.Unmarshal(capturedParams, &gotParams); err != nil {
		t.Fatalf("decode captured params: %v", err)
	}
	if gotParams.TextDocument.URI != "file:///a.go" {
		t.Errorf("params uri = %q", gotParams.TextDocument.URI)
	}
	if gotParams.Options.TabSize != 4 || gotParams.Options.InsertSpaces {
		t.Errorf("options = %+v, want {tabSize:4 insertSpaces:false}", gotParams.Options)
	}
	if gotParams.Position != nil {
		t.Errorf("formatting params carried a position: %+v", gotParams.Position)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientFormatting_NullResult(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`null`)}
	})

	edits, err := c.Formatting("file:///a.go")
	if err != nil {
		t.Fatalf("Formatting: %v", err)
	}
	if edits == nil || len(edits) != 0 {
		t.Errorf("edits = %+v, want empty non-nil slice", edits)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientFormatting_ServerError(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Err: &LSPError{Code: -32603, Message: "format failed"}}
	})

	edits, err := c.Formatting("file:///a.go")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if edits != nil {
		t.Errorf("edits = %+v, want nil on error", edits)
	}
	if !strings.Contains(err.Error(), "format failed") {
		t.Errorf("err = %v, want server message", err)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// -----------------------------------------------------------------------------
// Rename: changes map vs documentChanges 数组双形态, null, error
// -----------------------------------------------------------------------------

func TestLSPClientRename_ChangesShape(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var capturedParams json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(method string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		if method != "textDocument/rename" {
			t.Errorf("method = %q, want textDocument/rename", method)
		}
		capturedParams = append(capturedParams[:0:0], params...)
		return &fakeReply{Result: json.RawMessage(`{"changes":{"file:///a.go":[
{"range":{"start":{"line":1,"character":5},"end":{"line":1,"character":8}},"newText":"Bar"}
]}}`)}
	})

	we, err := c.Rename("file:///a.go", 1, 5, "Bar")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if we == nil {
		t.Fatal("Rename returned nil WorkspaceEdit")
	}
	if len(we.Changes) != 1 {
		t.Fatalf("changes len = %d, want 1: %+v", len(we.Changes), we.Changes)
	}
	edits := we.Changes["file:///a.go"]
	if len(edits) != 1 || edits[0].NewText != "Bar" || edits[0].Range.Start.Character != 5 {
		t.Errorf("edits = %+v", edits)
	}
	// 校验 newName 真序列化进了 params
	var gotParams struct {
		NewName  string      `json:"newName"`
		Position LSPPosition `json:"position"`
	}
	if err := json.Unmarshal(capturedParams, &gotParams); err != nil {
		t.Fatalf("decode captured params: %v", err)
	}
	if gotParams.NewName != "Bar" {
		t.Errorf("newName = %q, want Bar", gotParams.NewName)
	}
	if gotParams.Position.Line != 1 || gotParams.Position.Character != 5 {
		t.Errorf("position = %+v", gotParams.Position)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientRename_DocumentChangesShape(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	// 版本化形态: documentChanges 数组, 没有 changes map. 同一个 uri 出现两次,
	// 校验 client 把 edits 折叠到同一个 key 下.
	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`{"documentChanges":[
{"textDocument":{"uri":"file:///a.go","version":3},"edits":[
  {"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":3}},"newText":"Baz"}
]},
{"textDocument":{"uri":"file:///b.go","version":1},"edits":[
  {"range":{"start":{"line":4,"character":2},"end":{"line":4,"character":5}},"newText":"Baz"}
]},
{"textDocument":{"uri":"file:///a.go","version":3},"edits":[
  {"range":{"start":{"line":7,"character":1},"end":{"line":7,"character":4}},"newText":"Baz"}
]}
]}`)}
	})

	we, err := c.Rename("file:///a.go", 0, 0, "Baz")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if we == nil {
		t.Fatal("Rename returned nil WorkspaceEdit")
	}
	if len(we.Changes) != 2 {
		t.Fatalf("changes len = %d, want 2 (a.go + b.go): %+v", len(we.Changes), we.Changes)
	}
	// a.go 的两段 edits 被折叠到一起
	if got := we.Changes["file:///a.go"]; len(got) != 2 || got[0].NewText != "Baz" || got[1].Range.Start.Line != 7 {
		t.Errorf("a.go edits = %+v, want 2 folded edits", got)
	}
	if got := we.Changes["file:///b.go"]; len(got) != 1 || got[0].Range.Start.Line != 4 {
		t.Errorf("b.go edits = %+v", got)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientRename_NullResult(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`null`)}
	})

	we, err := c.Rename("file:///a.go", 1, 2, "X")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if we != nil {
		t.Errorf("we = %+v, want nil", we)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientRename_ServerError(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Err: &LSPError{Code: -32603, Message: "cannot rename builtin"}}
	})

	we, err := c.Rename("file:///a.go", 1, 2, "X")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if we != nil {
		t.Errorf("we = %+v, want nil on error", we)
	}
	if !strings.Contains(err.Error(), "cannot rename builtin") {
		t.Errorf("err = %v, want server message", err)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// -----------------------------------------------------------------------------
// CodeAction: 裸 Command 与 CodeAction 混排, range/context 形状, null, error
// -----------------------------------------------------------------------------

func TestLSPClientCodeAction_CommandAndAction(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var capturedParams json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(method string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		if method != "textDocument/codeAction" {
			t.Errorf("method = %q, want textDocument/codeAction", method)
		}
		capturedParams = append(capturedParams[:0:0], params...)
		// 第一项是裸 Command (有 command, 无 kind), 第二项是 CodeAction (有 kind + edit).
		return &fakeReply{Result: json.RawMessage(`[
{"title":"Organize Imports","command":"gopls.organize_imports","arguments":["file:///a.go"]},
{"title":"Extract function","kind":"refactor.extract","edit":{"changes":{
  "file:///a.go":[{"range":{"start":{"line":2,"character":0},"end":{"line":2,"character":0}},"newText":"x := 1\n"}]
}}}
]`)}
	})

	actions, err := c.CodeAction("file:///a.go", 1, 0, 3, 0)
	if err != nil {
		t.Fatalf("CodeAction: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(actions), actions)
	}
	// 裸 Command: 有 title, kind 留空, 无内联 edit, command 折进 Command 字段.
	if actions[0].Title != "Organize Imports" || actions[0].Kind != "" {
		t.Errorf("actions[0] = %+v, want bare Command with empty kind", actions[0])
	}
	if actions[0].Edit != nil {
		t.Errorf("actions[0].Edit = %+v, want nil for bare Command", actions[0].Edit)
	}
	if actions[0].Command == nil {
		t.Fatalf("actions[0].Command = nil, want bare Command surfaced")
	}
	if actions[0].Command.Title != "Organize Imports" || actions[0].Command.Command != "gopls.organize_imports" {
		t.Errorf("actions[0].Command = %+v", actions[0].Command)
	}
	if got := string(actions[0].Command.Arguments); got != `["file:///a.go"]` {
		t.Errorf("actions[0].Command.Arguments = %s, want top-level arguments", got)
	}
	// CodeAction: title + kind, 内联 edit 折叠进 Edit.Changes, 无 command.
	if actions[1].Title != "Extract function" || actions[1].Kind != "refactor.extract" {
		t.Errorf("actions[1] = %+v", actions[1])
	}
	if actions[1].Command != nil {
		t.Errorf("actions[1].Command = %+v, want nil (no command on this action)", actions[1].Command)
	}
	if actions[1].Edit == nil {
		t.Fatalf("actions[1].Edit = nil, want inline WorkspaceEdit")
	}
	if got := actions[1].Edit.Changes["file:///a.go"]; len(got) != 1 || got[0].NewText != "x := 1\n" {
		t.Errorf("actions[1].Edit.Changes = %+v", actions[1].Edit.Changes)
	}
	// 校验 range + context.diagnostics 形状
	var gotParams struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Range   LSPRange `json:"range"`
		Context struct {
			Diagnostics []json.RawMessage `json:"diagnostics"`
		} `json:"context"`
	}
	if err := json.Unmarshal(capturedParams, &gotParams); err != nil {
		t.Fatalf("decode captured params: %v", err)
	}
	if gotParams.TextDocument.URI != "file:///a.go" {
		t.Errorf("params uri = %q", gotParams.TextDocument.URI)
	}
	if gotParams.Range.Start.Line != 1 || gotParams.Range.End.Line != 3 {
		t.Errorf("range = %+v, want start.line=1 end.line=3", gotParams.Range)
	}
	if gotParams.Context.Diagnostics == nil {
		t.Errorf("context.diagnostics missing; want empty array")
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientCodeAction_NullResult(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`null`)}
	})

	actions, err := c.CodeAction("file:///a.go", 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("CodeAction: %v", err)
	}
	if actions == nil || len(actions) != 0 {
		t.Errorf("actions = %+v, want empty non-nil slice", actions)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientCodeAction_ServerError(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Err: &LSPError{Code: -32603, Message: "code action provider failed"}}
	})

	actions, err := c.CodeAction("file:///a.go", 0, 0, 1, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if actions != nil {
		t.Errorf("actions = %+v, want nil on error", actions)
	}
	if !strings.Contains(err.Error(), "code action provider failed") {
		t.Errorf("err = %v, want server message", err)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// CodeAction 形态, edit 用 changes map 形态 -> 折叠进 Edit.Changes (uri->edits).
func TestLSPClientCodeAction_EditChangesShape(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`[
{"title":"Add missing import","kind":"quickfix","edit":{"changes":{"file:///a.go":[
  {"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"newText":"import \"fmt\"\n"}
]}}}
]`)}
	})

	actions, err := c.CodeAction("file:///a.go", 5, 2, 5, 8)
	if err != nil {
		t.Fatalf("CodeAction: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(actions), actions)
	}
	a := actions[0]
	if a.Title != "Add missing import" || a.Kind != "quickfix" {
		t.Errorf("action = %+v", a)
	}
	if a.Command != nil {
		t.Errorf("Command = %+v, want nil", a.Command)
	}
	if a.Edit == nil {
		t.Fatal("Edit = nil, want inline WorkspaceEdit")
	}
	edits := a.Edit.Changes["file:///a.go"]
	if len(edits) != 1 || edits[0].NewText != "import \"fmt\"\n" || edits[0].Range.Start.Line != 0 {
		t.Errorf("Edit.Changes[a.go] = %+v", edits)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// CodeAction 形态, edit 用 documentChanges 数组形态 -> 折叠进 Edit.Changes,
// 同一个 uri 出现两次时把 edits 串接 (复用 rename 同款折叠).
func TestLSPClientCodeAction_EditDocumentChangesShape(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`[
{"title":"Fix all","kind":"source.fixAll","edit":{"documentChanges":[
  {"textDocument":{"uri":"file:///a.go","version":2},"edits":[
    {"range":{"start":{"line":1,"character":0},"end":{"line":1,"character":3}},"newText":"foo"}
  ]},
  {"textDocument":{"uri":"file:///b.go","version":1},"edits":[
    {"range":{"start":{"line":4,"character":2},"end":{"line":4,"character":5}},"newText":"bar"}
  ]},
  {"textDocument":{"uri":"file:///a.go","version":2},"edits":[
    {"range":{"start":{"line":9,"character":1},"end":{"line":9,"character":4}},"newText":"baz"}
  ]}
]}}
]`)}
	})

	actions, err := c.CodeAction("file:///a.go", 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("CodeAction: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(actions), actions)
	}
	a := actions[0]
	if a.Title != "Fix all" || a.Kind != "source.fixAll" || a.Command != nil {
		t.Errorf("action = %+v", a)
	}
	if a.Edit == nil {
		t.Fatal("Edit = nil, want folded WorkspaceEdit")
	}
	if len(a.Edit.Changes) != 2 {
		t.Fatalf("Changes len = %d, want 2 (a.go + b.go): %+v", len(a.Edit.Changes), a.Edit.Changes)
	}
	// a.go 的两段 edits 折叠到一起
	if got := a.Edit.Changes["file:///a.go"]; len(got) != 2 || got[0].NewText != "foo" || got[1].Range.Start.Line != 9 {
		t.Errorf("a.go edits = %+v, want 2 folded edits", got)
	}
	if got := a.Edit.Changes["file:///b.go"]; len(got) != 1 || got[0].NewText != "bar" {
		t.Errorf("b.go edits = %+v", got)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// 裸 Command (title + command, 无 edit) -> Title + Command 落位, Edit 留 nil.
func TestLSPClientCodeAction_BareCommand(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`[
{"title":"Regenerate cgo","command":"gopls.regenerate_cgo","arguments":[{"URI":"file:///a.go"}]}
]`)}
	})

	actions, err := c.CodeAction("file:///a.go", 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("CodeAction: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(actions), actions)
	}
	a := actions[0]
	if a.Title != "Regenerate cgo" || a.Kind != "" {
		t.Errorf("action = %+v, want bare Command", a)
	}
	if a.Edit != nil {
		t.Errorf("Edit = %+v, want nil for bare Command", a.Edit)
	}
	if a.Command == nil {
		t.Fatal("Command = nil, want bare Command surfaced")
	}
	if a.Command.Title != "Regenerate cgo" || a.Command.Command != "gopls.regenerate_cgo" {
		t.Errorf("Command = %+v", a.Command)
	}
	if got := string(a.Command.Arguments); got != `[{"URI":"file:///a.go"}]` {
		t.Errorf("Command.Arguments = %s, want top-level arguments preserved", got)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// 只有 title (+ kind) 的稀疏项 -> Edit nil, Command nil (不回归早先纯标题行为).
func TestLSPClientCodeAction_TitleOnly(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`[
{"title":"Extract variable","kind":"refactor.extract"}
]`)}
	})

	actions, err := c.CodeAction("file:///a.go", 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("CodeAction: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(actions), actions)
	}
	a := actions[0]
	if a.Title != "Extract variable" || a.Kind != "refactor.extract" {
		t.Errorf("action = %+v", a)
	}
	if a.Edit != nil || a.Command != nil {
		t.Errorf("sparse item: Edit=%+v Command=%+v, want both nil", a.Edit, a.Command)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// -----------------------------------------------------------------------------
// ExecuteCommand: workspace/executeCommand -- 执行 command-form 的 code action.
// command + arguments 透传, null result -> (nil,nil), JSON result 原样透出,
// nil arguments 必须序列化成 [] 而不是 null, error
// -----------------------------------------------------------------------------

func TestLSPClientExecuteCommand_NullResult(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var capturedParams json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(method string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		if method != "workspace/executeCommand" {
			t.Errorf("method = %q, want workspace/executeCommand", method)
		}
		capturedParams = append(capturedParams[:0:0], params...)
		// gopls 的多数命令回 null (副作用走 workspace/applyEdit).
		return &fakeReply{Result: json.RawMessage(`null`)}
	})

	args := []json.RawMessage{json.RawMessage(`"file:///a.go"`), json.RawMessage(`{"all":true}`)}
	res, err := c.ExecuteCommand("gopls.organize_imports", args)
	if err != nil {
		t.Fatalf("ExecuteCommand: %v", err)
	}
	if res != nil {
		t.Errorf("res = %s, want nil on null result", string(res))
	}
	// command + arguments 真序列化进了 params
	var got struct {
		Command   string            `json:"command"`
		Arguments []json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(capturedParams, &got); err != nil {
		t.Fatalf("decode captured params: %v", err)
	}
	if got.Command != "gopls.organize_imports" {
		t.Errorf("command = %q, want gopls.organize_imports", got.Command)
	}
	if len(got.Arguments) != 2 ||
		string(got.Arguments[0]) != `"file:///a.go"` ||
		string(got.Arguments[1]) != `{"all":true}` {
		t.Errorf("arguments = %v, want the two passed args verbatim", got.Arguments)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientExecuteCommand_JSONResult(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	// 个别命令 (查询类) 会回一个 JSON 结果, ExecuteCommand 应原样透出.
	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`{"applied":true,"count":3}`)}
	})

	res, err := c.ExecuteCommand("gopls.run_tests", []json.RawMessage{json.RawMessage(`{"URI":"file:///a_test.go"}`)})
	if err != nil {
		t.Fatalf("ExecuteCommand: %v", err)
	}
	var got struct {
		Applied bool `json:"applied"`
		Count   int  `json:"count"`
	}
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("decode result: %v (%s)", err, string(res))
	}
	if !got.Applied || got.Count != 3 {
		t.Errorf("result = %+v, want {applied:true count:3}", got)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientExecuteCommand_NilArgumentsBecomesEmptyArray(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var capturedParams json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		capturedParams = append(capturedParams[:0:0], params...)
		return &fakeReply{Result: json.RawMessage(`null`)}
	})

	if _, err := c.ExecuteCommand("gopls.tidy", nil); err != nil {
		t.Fatalf("ExecuteCommand: %v", err)
	}
	// nil arguments 必须序列化成 [] 而不是 null (gopls 对 null 会报错).
	var got struct {
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(capturedParams, &got); err != nil {
		t.Fatalf("decode captured params: %v", err)
	}
	if string(got.Arguments) != "[]" {
		t.Errorf("arguments = %s, want [] for nil input", string(got.Arguments))
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientExecuteCommand_ServerError(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Err: &LSPError{Code: -32602, Message: "unknown command"}}
	})

	res, err := c.ExecuteCommand("gopls.does_not_exist", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res != nil {
		t.Errorf("res = %s, want nil on error", string(res))
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("err = %v, want server message", err)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// -----------------------------------------------------------------------------
// DocumentHighlight: 同文件同符号高亮. 3 处命中带 Range/Kind, null -> 空切片, error
// -----------------------------------------------------------------------------

func TestLSPClientDocumentHighlight_Highlights(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var capturedParams json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(method string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		if method != "textDocument/documentHighlight" {
			t.Errorf("method = %q, want textDocument/documentHighlight", method)
		}
		capturedParams = append(capturedParams[:0:0], params...)
		// 三处出现: Text(1) / Read(2) / Write(3).
		return &fakeReply{Result: json.RawMessage(`[
{"range":{"start":{"line":1,"character":5},"end":{"line":1,"character":8}},"kind":1},
{"range":{"start":{"line":3,"character":0},"end":{"line":3,"character":3}},"kind":2},
{"range":{"start":{"line":7,"character":2},"end":{"line":7,"character":5}},"kind":3}
]`)}
	})

	hls, err := c.DocumentHighlight("file:///a.go", 1, 6)
	if err != nil {
		t.Fatalf("DocumentHighlight: %v", err)
	}
	if len(hls) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(hls), hls)
	}
	if hls[0].Kind != 1 || hls[0].Range.Start.Line != 1 || hls[0].Range.Start.Character != 5 || hls[0].Range.End.Character != 8 {
		t.Errorf("hls[0] = %+v", hls[0])
	}
	if hls[1].Kind != 2 || hls[1].Range.Start.Line != 3 {
		t.Errorf("hls[1] = %+v", hls[1])
	}
	if hls[2].Kind != 3 || hls[2].Range.Start.Line != 7 || hls[2].Range.End.Character != 5 {
		t.Errorf("hls[2] = %+v", hls[2])
	}
	// params 跟 hover 同形: 带 position + textDocument.
	var gotParams struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position LSPPosition `json:"position"`
	}
	if err := json.Unmarshal(capturedParams, &gotParams); err != nil {
		t.Fatalf("decode captured params: %v", err)
	}
	if gotParams.TextDocument.URI != "file:///a.go" {
		t.Errorf("params uri = %q", gotParams.TextDocument.URI)
	}
	if gotParams.Position.Line != 1 || gotParams.Position.Character != 6 {
		t.Errorf("position = %+v", gotParams.Position)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientDocumentHighlight_NullResult(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Result: json.RawMessage(`null`)}
	})

	hls, err := c.DocumentHighlight("file:///a.go", 0, 0)
	if err != nil {
		t.Fatalf("DocumentHighlight: %v", err)
	}
	if hls == nil || len(hls) != 0 {
		t.Errorf("hls = %+v, want empty non-nil slice", hls)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPClientDocumentHighlight_ServerError(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Err: &LSPError{Code: -32603, Message: "highlight provider failed"}}
	})

	hls, err := c.DocumentHighlight("file:///a.go", 1, 2)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if hls != nil {
		t.Errorf("hls = %+v, want nil on error", hls)
	}
	if !strings.Contains(err.Error(), "highlight provider failed") {
		t.Errorf("err = %v, want server message", err)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// newPipedClient 构造一个 *LSPClient 把它的 stdin/stdout 接到 io.Pipe 上
// 返回:
//   - c     已经跑着 readLoop 的客户端
//   - srvIn 上层"假服务器"用来读 *客户端写出的字节*
//   - srvOut 上层"假服务器"用来写 *给客户端的响应*
//
// 不真的拉子进程, 也不依赖 gopls 二进制.
func newPipedClient(t *testing.T) (*LSPClient, io.ReadCloser, io.WriteCloser) {
	t.Helper()
	// 方向 A: client.stdin -> server 读
	srvIn, clientStdin := io.Pipe()
	// 方向 B: server -> client.stdout 读
	clientStdout, srvOut := io.Pipe()

	c := &LSPClient{
		stdin:         clientStdin,
		stdout:        bufio.NewReader(clientStdout),
		pending:       make(map[int]chan *LSPMessage),
		notifications: make(chan *LSPMessage, 16),
		done:          make(chan struct{}),
	}
	go c.readLoop()
	return c, srvIn, srvOut
}

// -----------------------------------------------------------------------------
// gopls 烟雾测试: 真起子进程, 不在 PATH 上时 Skip
// -----------------------------------------------------------------------------

const goplsSmokeMainGo = `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`

func TestLSPClientSmoke_GoplsInitialize(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not on PATH, skipping LSP smoke test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH, skipping LSP smoke test")
	}

	dir := t.TempDir()
	// 一份最小工作区: go.mod + main.go. gopls 在没有 go.mod 时会以
	// "single-file mode" 跑, initialize 依旧会成功, 但 didOpen 之后的
	// 表现差很多. 我们给齐 go.mod 让握手贴近真实场景.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module lspsmoke\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(goplsSmokeMainGo), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	c, err := LaunchLSPClient("gopls")
	if err != nil {
		t.Fatalf("LaunchLSPClient: %v", err)
	}
	defer func() {
		if cerr := c.Close(); cerr != nil {
			t.Logf("Close: %v", cerr)
		}
	}()

	rootURI := "file://" + dir
	if _, err := c.Initialize(LSPInitializeParams{
		ProcessID: 0,
		RootURI:   rootURI,
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Initialize 之后的 DidOpen 不应当报错 -- 它是 notification, 写 pipe
	// 失败才会回错. 给 gopls 一点时间消化, 不强制等响应.
	if err := c.DidOpen("file://"+mainPath, "go", goplsSmokeMainGo, 1); err != nil {
		t.Errorf("DidOpen: %v", err)
	}
}


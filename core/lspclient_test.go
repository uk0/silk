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


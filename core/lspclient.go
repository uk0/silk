package core

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// LSP client: 在 core/lsp.go 提供的 framing/编解码原语之上, 加一层
// "进程控制 + 请求路由". 抽象上跟 core/dlv.go 的 DebugSession 同形态:
// 拉起一个子进程语言服务器 (例如 gopls), 在 stdin/stdout 上跑 LSP 协议,
// 把发出去的请求按 ID 挂在 pending map 上, 等读循环把对应响应送回来.
//
// 这一版只覆盖 "framework 级别" 的能力:
//   - LaunchLSPClient   拉起子进程, 起读循环
//   - Initialize        发 "initialize" 请求 + 自动 "initialized" 通知
//   - SendRequest       通用请求路由 (任意 method)
//   - SendNotification  通用单向通知
//   - DidOpen           textDocument/didOpen 便利封装 (gopls 在做任何文件
//                       级操作之前都需要它)
//   - Notifications     拿到服务器主动推过来的通知 (publishDiagnostics 等)
//   - Close             shutdown + exit, best-effort
//
// 具体的请求形状 (completion/hover/definition) 是 *下一个* commit 的事:
// 这里有意只暴露通用 SendRequest, 不预先固化具体 LSP method 的类型, 避免
// 这一层一上来就跟具体语言/具体服务器能力深度耦合.
//
// stdlib only. 不引第三方 LSP 类型包.

// LSPClient 是一个跑着的 LSP 服务器子进程 + 跟它建立的 LSP 协议会话
// 同一个 client 可以并发 SendRequest, ID 分配/pending 路由由 mu 保护,
// 实际写 stdin 的字节流由 writeMu 串行化 -- WriteLSPMessage 内部分 "header 写"
// 和 "body 写" 两步, 多 goroutine 共用一个 pipe 时必须串起来, 否则两个并发
// 请求的 header/body 会交错, 把帧搞乱.
type LSPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser

	nextID  int
	mu      sync.Mutex // guards pending + nextID + closed
	pending map[int]chan *LSPMessage
	closed  bool

	writeMu sync.Mutex // serializes WriteLSPMessage on stdin

	notifications chan *LSPMessage // 服务器主动发的 notification 缓存给上层

	done chan struct{} // 读循环退出时关闭
}

// writeFrame 在 writeMu 保护下把一条 LSP 消息原子写到 stdin
// 抽出来是因为 SendRequest / SendNotification / shutdownBestEffort 都要保证
// header 和 body 不被其他 goroutine 插队.
func (c *LSPClient) writeFrame(m *LSPMessage) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return WriteLSPMessage(c.stdin, m)
}

// LSPInitializeParams 是 "initialize" 请求体的最小子集
// LSP 规范的 InitializeParams 还有 clientCapabilities / workspaceFolders /
// trace / locale / initializationOptions 等几十个可选字段. gopls 实测下:
//   - clientCapabilities 完全省略时它依然会握手成功, 只是把所有 client 能
//     力当 false 处理 (我们不消费 hover/completion 的高级 markdown 形态,
//     纯文本路径足够);
//   - rootUri 强烈建议给 (gopls 会在这个目录里扫包索引).
//
// 因此把这里只暴露 ProcessID + RootURI, 调用方需要更精细形状时可绕过
// Initialize, 直接 SendRequest("initialize", customParams) + 手发
// "initialized" notification.
type LSPInitializeParams struct {
	ProcessID int    `json:"processId"`
	RootURI   string `json:"rootUri"`
}

// defaultRequestTimeout 是 SendRequest 的默认超时
// gopls 第一个 initialize 可能要等几百 ms 把工作区扫起来; 10s 留出充分
// 余量. 后续具体 RPC 真要更长/更短的预算时, 再加 SendRequestContext 变体.
const defaultRequestTimeout = 10 * time.Second

// LaunchLSPClient 拉起 serverCmd args... 并建立 LSP 长连接
// pipe 的方向:
//   - cmd.Stdin   <- 我们写 (请求/通知)
//   - cmd.Stdout  -> 我们读 (响应/服务器通知)
//   - cmd.Stderr  -> 我们 drain (日志, 防止满管道导致服务器卡住)
//
// 读循环作为 goroutine 在返回前就跑起来; 任何启动错误都会清理子进程后回报.
func LaunchLSPClient(serverCmd string, args ...string) (*LSPClient, error) {
	if serverCmd == "" {
		return nil, errors.New("lspclient: empty server command")
	}
	if _, err := exec.LookPath(serverCmd); err != nil {
		return nil, fmt.Errorf("lspclient: server %q not on PATH: %w", serverCmd, err)
	}

	cmd := exec.Command(serverCmd, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lspclient: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("lspclient: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("lspclient: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("lspclient: start %s: %w", serverCmd, err)
	}

	c := &LSPClient{
		cmd:           cmd,
		stdin:         stdin,
		stdout:        bufio.NewReader(stdout),
		stderr:        stderr,
		pending:       make(map[int]chan *LSPMessage),
		notifications: make(chan *LSPMessage, 64),
		done:          make(chan struct{}),
	}

	// stderr drain: 防止管道写满, 也方便排障. 不阻塞读循环.
	go drainStderr(stderr)

	// 主读循环
	go c.readLoop()

	return c, nil
}

// readLoop 单独占用一个 goroutine 持续解码服务器发来的消息
// 每条消息走 routeMessage 决定去向:
//   - 有 ID + (Result 或 Error)  -> 对应 pending chan
//   - 无 ID + Method != ""       -> notifications chan
//   - 其它                       -> drop + 日志
//
// EOF / 读错误 时退出循环: fail 掉所有 pending, close notifications (本循环是
// 它唯一的发送方), 最后关闭 done, 让 Close 知道读端已停.
func (c *LSPClient) readLoop() {
	defer close(c.done)
	for {
		m, err := ReadLSPMessage(c.stdout)
		if err != nil {
			// 读侧失败一般是子进程退出 / pipe 关闭
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				fmt.Fprintf(os.Stderr, "lspclient: read loop: %v\n", err)
			}
			// 把所有 pending 全部 fail 掉, 让 SendRequest 的等待方早退
			c.failAllPending()
			// readLoop 是 notifications 的唯一发送方 (routeMessage 只在本循环
			// 里被调用); 读侧终结后不会再有投递, 这里 close 掉, 让上层的
			// `for range Notifications()` 随会话结束而退出, 不泄漏 drain goroutine.
			close(c.notifications)
			return
		}

		// Hold mu across routeMessage: it reads c.pending, which SendRequest
		// writes concurrently. The earlier "snapshot" (pending := c.pending)
		// was a map-reference alias, not a copy, so the lookup raced the
		// write. A real copy would instead drop responses for requests added
		// after the copy. routeMessage only does a map lookup + a non-blocking
		// channel send (buffered chans, select-default), so the critical
		// section stays tiny.
		c.mu.Lock()
		routeMessage(m, c.pending, c.notifications)
		c.mu.Unlock()
	}
}

// routeMessage 是单条 LSP 消息的"投递决策", 纯函数, 可单测
//   - 响应 (有 numeric ID 且 Result/Error 至少一个非空): 找 pending[id], 推到那个 channel
//   - 通知 (无 ID, 有 Method): 非阻塞推到 notif (满了就丢, 避免拖死读循环)
//   - 其它 (服务器发的 request -- LSP 允许, 例如 workspace/configuration):
//     当前实现不回, 静默丢弃; 未来需要时再加 server->client request 路由.
//
// 注意:
//   - 我们只识别数字 ID (NewRequest 总是写数字, 自洽).
//   - pending map 里的 channel 是 buffered=1, 不会阻塞读循环.
//   - readLoop 在 c.mu 锁内调用本函数 (pending 与 SendRequest 的写并发,
//     必须串行化); 这里只做 map 查找 + 非阻塞 channel 推送, 临界区极短.
//     单测里直接传一个独占的 map (无并发写), 同样安全.
func routeMessage(m *LSPMessage, pending map[int]chan *LSPMessage, notif chan<- *LSPMessage) {
	if m == nil {
		return
	}
	if m.ID != nil {
		// 可能是响应 (有 Result/Error) 或服务器发的 request (有 Method)
		hasResp := len(m.Result) > 0 || m.Error != nil
		if hasResp {
			var id int
			if err := json.Unmarshal(*m.ID, &id); err != nil {
				// 字符串 ID 不在我们发出的请求集合里 -- 我们只用数字 ID,
				// 收到字符串 ID 的响应说明协议错位, 丢弃即可
				return
			}
			if ch, ok := pending[id]; ok {
				// channel 是 buffered, 这次 send 不会阻塞;
				// 即便意外阻塞, 也用 select 兜底
				select {
				case ch <- m:
				default:
				}
			}
			return
		}
		// 服务器发的 request, 当前实现不处理
		return
	}
	// 没有 ID -> notification (或者畸形 -- LSP 规范要求 method 非空)
	if m.Method == "" {
		return
	}
	// 非阻塞推, 满了就丢弃 -- 不让上层未消费的通知反压住读循环
	select {
	case notif <- m:
	default:
	}
}

// failAllPending 在读循环退出时把所有等待方解绑
// 给每个 pending channel 推一条带 Error 的桩消息, 让 SendRequest 立即返回错误
// 不直接 close pending channel: 那样会跟正常返回的 buffered 1 路径混淆.
func (c *LSPClient) failAllPending() {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[int]chan *LSPMessage)
	c.mu.Unlock()
	for _, ch := range pending {
		stub := &LSPMessage{
			JSONRPC: "2.0",
			Error: &LSPError{
				Code:    -32000,
				Message: "lsp: server connection closed",
			},
		}
		select {
		case ch <- stub:
		default:
		}
	}
}

// drainStderr 把子进程的 stderr 持续读走丢弃
// 不消费会导致管道写满, 服务器卡死. 调试时可改成 io.Copy(os.Stderr, r).
func drainStderr(r io.ReadCloser) {
	br := bufio.NewReader(r)
	for {
		_, err := br.ReadBytes('\n')
		if err != nil {
			return
		}
	}
}

// Initialize 走 LSP 规范的 initialize -> initialized 握手
// 流程:
//  1. SendRequest("initialize", params)  阻塞等响应
//  2. 拿到 Result 之后立刻发 "initialized" notification (规范要求)
//
// 返回的是 initialize 响应里的原始 Result (包含 server capabilities 等),
// 调用方按需 json.Unmarshal 到自己关心的结构上.
func (c *LSPClient) Initialize(params LSPInitializeParams) (json.RawMessage, error) {
	resp, err := c.SendRequest("initialize", params)
	if err != nil {
		return nil, fmt.Errorf("lspclient: initialize: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("lspclient: initialize error: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}
	// 立刻发 initialized -- 规范要求 server 在收到这个 notification 后才接受
	// 后续的请求 (gopls 实测在收到之前会拒绝 textDocument/* 类调用).
	if err := c.SendNotification("initialized", struct{}{}); err != nil {
		return resp.Result, fmt.Errorf("lspclient: send initialized: %w", err)
	}
	return resp.Result, nil
}

// SendRequest 发一条带 ID 的 LSP 请求, 等服务器把同 ID 响应送回来
// 串行/并发都安全. 默认 10s 超时, 超时后:
//   - 把 pending[id] 清掉, 防止读循环之后误送到一个无人监听的 chan
//   - 返回 context-deadline 风格错误
//
// 不取消子进程: 单条请求超时不应该直接撕掉整个会话, 上层若决定终止, 自行 Close.
func (c *LSPClient) SendRequest(method string, params interface{}) (*LSPMessage, error) {
	if method == "" {
		return nil, errors.New("lspclient: empty method")
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("lspclient: closed")
	}
	c.nextID++
	id := c.nextID
	// buffered=1 保证读循环投递不阻塞, 即便我们这边因为超时已经放弃监听
	ch := make(chan *LSPMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	req, err := NewRequest(id, method, params)
	if err != nil {
		c.removePending(id)
		return nil, fmt.Errorf("lspclient: build request %s: %w", method, err)
	}
	if err := c.writeFrame(req); err != nil {
		c.removePending(id)
		return nil, fmt.Errorf("lspclient: write request %s: %w", method, err)
	}

	// 等响应 / 超时 / 读循环挂掉
	select {
	case m := <-ch:
		c.removePending(id)
		if m.Error != nil {
			// 把 server-side error 一并返回, 但保留 message 供上层判别
			return m, fmt.Errorf("lspclient: %s server error: code=%d msg=%s", method, m.Error.Code, m.Error.Message)
		}
		return m, nil
	case <-time.After(defaultRequestTimeout):
		c.removePending(id)
		return nil, fmt.Errorf("lspclient: %s timed out after %s", method, defaultRequestTimeout)
	case <-c.done:
		c.removePending(id)
		return nil, fmt.Errorf("lspclient: %s aborted: server connection closed", method)
	}
}

// removePending 在请求结束 (成功/超时/错误) 后删掉 pending 条目
// 单独抽出来避免在多个 return 点重复写锁逻辑.
func (c *LSPClient) removePending(id int) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// SendNotification 发一条无 ID 的 LSP 通知, 立刻返回
// 服务器不会回; 失败仅意味着写 pipe 失败 (子进程死了等).
func (c *LSPClient) SendNotification(method string, params interface{}) error {
	if method == "" {
		return errors.New("lspclient: empty method")
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("lspclient: closed")
	}
	c.mu.Unlock()

	note, err := NewNotification(method, params)
	if err != nil {
		return fmt.Errorf("lspclient: build notification %s: %w", method, err)
	}
	if err := c.writeFrame(note); err != nil {
		return fmt.Errorf("lspclient: write notification %s: %w", method, err)
	}
	return nil
}

// DidOpen 发 textDocument/didOpen 通知, 把一个文件注册给服务器
// gopls 在对一个文件做 hover/definition/completion 之前都需要先看到 didOpen --
// 它不读文件系统, 内容以 client 这边的视图为准. languageID 一般是 "go".
func (c *LSPClient) DidOpen(uri, languageID, text string, version int) error {
	type textDocument struct {
		URI        string `json:"uri"`
		LanguageID string `json:"languageId"`
		Version    int    `json:"version"`
		Text       string `json:"text"`
	}
	type params struct {
		TextDocument textDocument `json:"textDocument"`
	}
	return c.SendNotification("textDocument/didOpen", params{
		TextDocument: textDocument{
			URI:        uri,
			LanguageID: languageID,
			Version:    version,
			Text:       text,
		},
	})
}

// DidClose 发 textDocument/didClose, 告诉服务器某文档已关闭. 不发的话 gopls
// 会一直持有该文档 (过期 version + 诊断), 每关一个 tab 泄漏一份幽灵文档.
func (c *LSPClient) DidClose(uri string) error {
	type textDocument struct {
		URI string `json:"uri"`
	}
	type params struct {
		TextDocument textDocument `json:"textDocument"`
	}
	return c.SendNotification("textDocument/didClose", params{
		TextDocument: textDocument{URI: uri},
	})
}

// -----------------------------------------------------------------------------
// 典型 IDE 操作的类型化便利封装
// -----------------------------------------------------------------------------
//
// 这一段把 IDE 真正吃饭的几个 LSP RPC 包成 *typed* 方法:
//   - Completion / Hover / Definition / References  (request)
//   - DidChange                                     (notification)
//
// 设计取舍:
//   - 类型刻意取最小子集. LSP 规范的 CompletionItem/Hover/Location 字段巨多,
//     绝大部分 IDE 拉一次 hover/补全就把数据展给用户, 不再深加工; gopls 也
//     把所有重要的东西塞进 Label/Detail/Contents.value 这些字符串字段里.
//     真要扩字段, 在 LSPCompletionItem / LSPHover / LSPLocation 上加 JSON tag
//     即可, 不会改方法签名.
//   - 几个 RPC 的响应里存在 *形状多态*, 因为 LSP 规范允许 server 在两种合法
//     形状中任选:
//       * completion: CompletionList{IsIncomplete, Items} 或 raw []CompletionItem
//       * definition: Location 或 []Location
//       * hover.contents: string 或 MarkupContent{Kind, Value} 或 []MarkedString
//     这里的做法是先尝试 "数组/对象" 中的一种, 失败再退到另一种, 别让上层
//     去关心 server 选择了哪种.
//   - 所有方法共用 SendRequest 的 10s 默认超时. 如果上层要更细的预算, 后续
//     可以加 *Context 变体, 不破坏当前 API.

// LSPPosition 是 LSP 中通用的"行/列"坐标 (零基)
type LSPPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// LSPRange 是 LSP 的 [start, end) 文本区间
type LSPRange struct {
	Start LSPPosition `json:"start"`
	End   LSPPosition `json:"end"`
}

// LSPLocation 是带 URI 的源代码定位, definition/references 的基本返回单元
type LSPLocation struct {
	URI   string   `json:"uri"`
	Range LSPRange `json:"range"`
}

// LSPCompletionItem 是补全列表里的一条
// 字段是 LSP CompletionItem 的最小子集; gopls 在补全里把签名 / 注释往
// Detail/Documentation 里塞, Label 是用户看到的标识. InsertText 缺省时
// 通常等于 Label.
type LSPCompletionItem struct {
	Label      string `json:"label"`
	Detail     string `json:"detail,omitempty"`
	Kind       int    `json:"kind,omitempty"`
	InsertText string `json:"insertText,omitempty"`
}

// LSPHover 是简化后的 hover 结果
// 把 LSP 那一坨 contents 多态形态压平成一个字符串: UI 层只用得到这个.
type LSPHover struct {
	Contents string
}

// textDocumentPositionParams 拼一份所有 position-based RPC 共用的 params
// completion / hover / definition / references 都接收 {textDocument, position},
// 抽出来一处, 各方法少四行重复代码.
func textDocumentPositionParams(uri string, line, character int) map[string]interface{} {
	return map[string]interface{}{
		"textDocument": map[string]string{"uri": uri},
		"position":     LSPPosition{Line: line, Character: character},
	}
}

// Completion 请求 textDocument/completion 并返回补全项列表
// gopls 在两种合法响应形状之间任意切换:
//   - CompletionList: {"isIncomplete": bool, "items": []CompletionItem}
//   - 直接的 []CompletionItem
//
// 这里都接住: 先按 CompletionList 解, items 非空就用; 否则当裸数组解.
// 受默认 10s SendRequest 超时约束.
func (c *LSPClient) Completion(uri string, line, character int) ([]LSPCompletionItem, error) {
	resp, err := c.SendRequest("textDocument/completion", textDocumentPositionParams(uri, line, character))
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return nil, nil
	}
	// 优先按 CompletionList 解 -- gopls 默认就走这个分支
	var list struct {
		IsIncomplete bool                `json:"isIncomplete"`
		Items        []LSPCompletionItem `json:"items"`
	}
	if err := json.Unmarshal(resp.Result, &list); err == nil && list.Items != nil {
		return list.Items, nil
	}
	// 退化到 raw []CompletionItem -- 规范允许, 一些 server 这么发
	var items []LSPCompletionItem
	if err := json.Unmarshal(resp.Result, &items); err != nil {
		return nil, fmt.Errorf("lspclient: decode completion result: %w", err)
	}
	return items, nil
}

// Hover 请求 textDocument/hover 并把多态的 contents 压平成一个字符串
// 规范里 contents 有三种合法形态:
//   - 字符串                               -> 直接用
//   - MarkupContent{kind, value}           -> 取 value (gopls 默认走这个)
//   - 数组 (string 或 MarkedString)        -> 用 "\n" join
//
// 服务器返回 null (光标位置没有可悬停信息) 时, 返回 (nil, nil), 不当错误.
// 受默认 10s SendRequest 超时约束.
func (c *LSPClient) Hover(uri string, line, character int) (*LSPHover, error) {
	resp, err := c.SendRequest("textDocument/hover", textDocumentPositionParams(uri, line, character))
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return nil, nil
	}
	var outer struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(resp.Result, &outer); err != nil {
		return nil, fmt.Errorf("lspclient: decode hover result: %w", err)
	}
	if len(outer.Contents) == 0 || string(outer.Contents) == "null" {
		return &LSPHover{}, nil
	}
	contents, err := stringifyHoverContents(outer.Contents)
	if err != nil {
		return nil, fmt.Errorf("lspclient: decode hover contents: %w", err)
	}
	return &LSPHover{Contents: contents}, nil
}

// stringifyHoverContents 把 hover.contents 的三种规范形态都压成一段字符串
// 解码顺序基于 JSON 第一个非空白字节:
//
//	"  -> string 形态
//	{  -> MarkupContent / MarkedString 对象形态
//	[  -> 数组形态 (string 或 MarkedString 混排)
//
// 任何一种都不动 caller, 失败时 raw 原样返回上层做诊断.
func stringifyHoverContents(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", nil
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", err
		}
		return s, nil
	case '{':
		// MarkupContent {kind, value} 或 MarkedString {language, value}
		var obj struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(raw, &obj); err != nil {
			return "", err
		}
		return obj.Value, nil
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return "", err
		}
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			s, err := stringifyHoverContents(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, "\n"), nil
	default:
		return "", fmt.Errorf("unexpected hover contents shape: %s", string(trimmed))
	}
}

// Definition 请求 textDocument/definition 并归一为 []LSPLocation
// 规范允许 server 在两种形态间任选:
//   - 单个 Location 对象
//   - []Location (gopls 在跨实例 / 嵌入 / 接口实现处会用这个)
//
// null 表示没找到定义, 返回 (nil, nil), 上层照空切片处理即可.
// 受默认 10s SendRequest 超时约束.
func (c *LSPClient) Definition(uri string, line, character int) ([]LSPLocation, error) {
	resp, err := c.SendRequest("textDocument/definition", textDocumentPositionParams(uri, line, character))
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return nil, nil
	}
	trimmed := bytes.TrimSpace(resp.Result)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var locs []LSPLocation
		if err := json.Unmarshal(resp.Result, &locs); err != nil {
			return nil, fmt.Errorf("lspclient: decode definition result: %w", err)
		}
		return locs, nil
	}
	var loc LSPLocation
	if err := json.Unmarshal(resp.Result, &loc); err != nil {
		return nil, fmt.Errorf("lspclient: decode definition result: %w", err)
	}
	return []LSPLocation{loc}, nil
}

// References 请求 textDocument/references 并返回所有出现处
// 规范固定只返回 []Location 一种形态 (没有像 definition 那样的多态).
// includeDecl 控制是否把定义点也算一次引用, 通常上层是 true (跟 IDE 一致).
// 受默认 10s SendRequest 超时约束.
func (c *LSPClient) References(uri string, line, character int, includeDecl bool) ([]LSPLocation, error) {
	params := textDocumentPositionParams(uri, line, character)
	params["context"] = map[string]bool{"includeDeclaration": includeDecl}
	resp, err := c.SendRequest("textDocument/references", params)
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return nil, nil
	}
	var locs []LSPLocation
	if err := json.Unmarshal(resp.Result, &locs); err != nil {
		return nil, fmt.Errorf("lspclient: decode references result: %w", err)
	}
	return locs, nil
}

// LSPSymbol 是文件大纲里的一个符号 (函数/类型/变量等), 扁平化后的最小子集
// 对应 IDE 的 outline / breadcrumb. 字段语义见 DocumentSymbol:
//   - Detail   声明摘要 (函数签名等), legacy SymbolInformation 没有, 留空
//   - Kind     LSP SymbolKind 数字枚举 (Function=12, Struct=23, ...)
//   - Line     0 基行号, 取 range.start.line
//   - Children 仅在 hierarchical (DocumentSymbol) 形态下填充; legacy 形态为空
type LSPSymbol struct {
	Name     string
	Detail   string
	Kind     int
	Line     int
	Children []LSPSymbol
}

// LSPSignature 是签名提示里的一条函数签名, SignatureInformation 的最小子集
//   - Label         整条签名文本 (例如 "Println(a ...any) (n int, err error)")
//   - Documentation 压平后的文档串 (复用 hover 的 contents 压平逻辑)
//   - Parameters    每个形参的 label 文本
type LSPSignature struct {
	Label         string
	Documentation string
	Parameters    []string
}

// LSPSignatureHelp 是 textDocument/signatureHelp 压平后的结果
// ActiveSignature / ActiveParameter 指示 UI 该高亮哪条签名 / 哪个形参 (0 基).
type LSPSignatureHelp struct {
	Signatures      []LSPSignature
	ActiveSignature int
	ActiveParameter int
}

// DocumentSymbol 请求 textDocument/documentSymbol 并归一成扁平/层级化的 LSPSymbol
// params 只要 {textDocument:{uri}}, 没有 position. 响应形状由 server 能力决定,
// 规范允许两种, gopls 走前者:
//   - hierarchical []DocumentSymbol:
//     {name, detail, kind, range, selectionRange, children []DocumentSymbol}
//     -- 嵌套, 有 range/selectionRange, *没有* 顶层 location
//   - legacy []SymbolInformation (扁平):
//     {name, kind, location:{uri, range}, containerName}
//     -- 没有 children, 位置藏在 location.range 里
//
// 区分手段: 探测数组里第一个元素有没有 "location" 字段. 有 -> SymbolInformation,
// 否则当 DocumentSymbol (它用 range/selectionRange, 没有顶层 location). 两种都
// 拿不准时偏向 DocumentSymbol -- 它是现代 server 的默认, 也是 gopls 的形态.
// DocumentSymbol 保留层级 (Children 递归填充); SymbolInformation 返回扁平切片
// (Children 为空). null/空数组 -> 空切片, 不当错误.
// 受默认 10s SendRequest 超时约束.
func (c *LSPClient) DocumentSymbol(uri string) ([]LSPSymbol, error) {
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": uri},
	}
	resp, err := c.SendRequest("textDocument/documentSymbol", params)
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return nil, nil
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(resp.Result, &raw); err != nil {
		return nil, fmt.Errorf("lspclient: decode documentSymbol result: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	// 探测形状: SymbolInformation 一定带 location, DocumentSymbol 一定不带.
	if rawSymbolHasLocation(raw[0]) {
		return decodeSymbolInformation(raw)
	}
	return decodeDocumentSymbols(raw)
}

// rawSymbolHasLocation 探测一个 symbol 元素是不是 SymbolInformation (legacy 扁平)
// 只看 "location" 字段在不在 -- 它是 SymbolInformation 独有, DocumentSymbol 没有.
func rawSymbolHasLocation(raw json.RawMessage) bool {
	var probe struct {
		Location *json.RawMessage `json:"location"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return probe.Location != nil
}

// decodeDocumentSymbols 解 hierarchical []DocumentSymbol, 递归保留 Children
// Line 取 range.start.line.
func decodeDocumentSymbols(raw []json.RawMessage) ([]LSPSymbol, error) {
	type docSymbol struct {
		Name     string            `json:"name"`
		Detail   string            `json:"detail"`
		Kind     int               `json:"kind"`
		Range    LSPRange          `json:"range"`
		Children []json.RawMessage `json:"children"`
	}
	out := make([]LSPSymbol, 0, len(raw))
	for _, item := range raw {
		var ds docSymbol
		if err := json.Unmarshal(item, &ds); err != nil {
			return nil, fmt.Errorf("lspclient: decode documentSymbol entry: %w", err)
		}
		sym := LSPSymbol{
			Name:   ds.Name,
			Detail: ds.Detail,
			Kind:   ds.Kind,
			Line:   ds.Range.Start.Line,
		}
		if len(ds.Children) > 0 {
			children, err := decodeDocumentSymbols(ds.Children)
			if err != nil {
				return nil, err
			}
			sym.Children = children
		}
		out = append(out, sym)
	}
	return out, nil
}

// decodeSymbolInformation 解 legacy []SymbolInformation 为扁平切片 (Children 空)
// Line 取 location.range.start.line; 没有 detail 字段, 留空.
func decodeSymbolInformation(raw []json.RawMessage) ([]LSPSymbol, error) {
	type symbolInfo struct {
		Name     string      `json:"name"`
		Kind     int         `json:"kind"`
		Location LSPLocation `json:"location"`
	}
	out := make([]LSPSymbol, 0, len(raw))
	for _, item := range raw {
		var si symbolInfo
		if err := json.Unmarshal(item, &si); err != nil {
			return nil, fmt.Errorf("lspclient: decode symbolInformation entry: %w", err)
		}
		out = append(out, LSPSymbol{
			Name: si.Name,
			Kind: si.Kind,
			Line: si.Location.Range.Start.Line,
		})
	}
	return out, nil
}

// LSPWorkspaceSymbol 是 workspace/symbol 项目级搜索里的一条命中 (IDE 的 Cmd+T
// "Go to Symbol in Workspace"). 跟文件级的 LSPSymbol 不同, 工作区符号一定带
// URI (符号落在哪个文件) 和 ContainerName (所属包/类型), 所以单开一个类型而不是
// 复用扁平的 LSPSymbol:
//   - Kind            LSP SymbolKind 数字枚举 (Function=12, Struct=23, ...)
//   - Line/Character  0 基坐标, 取 location.range.start
type LSPWorkspaceSymbol struct {
	Name          string
	Kind          int
	ContainerName string
	URI           string
	Line          int // 0 基, 取 location.range.start.line
	Character     int
}

// WorkspaceSymbol 请求 workspace/symbol 做项目级符号搜索, 是 DocumentSymbol 的工作区对偶
// params 只要 {query}: 空串表示"全部符号" (gopls 会返回一个有上界的集合), 非空则做模糊匹配.
// 响应是一个数组, 每个元素在两种合法形态间任选, server 混发都合法:
//   - legacy SymbolInformation: {name, kind, containerName, location:{uri, range}}
//     -- location 是完整 Location, 带 range
//   - 现代 WorkspaceSymbol:      {name, kind, containerName, location:{uri}}
//     -- location 可能只带 uri, range 省略 (真要坐标时 server 靠 workspaceSymbol/resolve 补)
//
// 两种形态塞进同一个解码结构即可: location 用 LSPLocation 收, range 缺省时 json 保持零值,
// 于是 Line/Character 自然落到 0 -- 正是"缺 range 就默认 0"的期望, 无需特判.
// null/空数组 -> 空切片, 不当错误; server 端错误经 SendRequest 包好后原样透出.
// 受默认 10s SendRequest 超时约束.
func (c *LSPClient) WorkspaceSymbol(query string) ([]LSPWorkspaceSymbol, error) {
	params := map[string]interface{}{"query": query}
	resp, err := c.SendRequest("workspace/symbol", params)
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return []LSPWorkspaceSymbol{}, nil
	}
	var raw []struct {
		Name          string      `json:"name"`
		Kind          int         `json:"kind"`
		ContainerName string      `json:"containerName"`
		Location      LSPLocation `json:"location"`
	}
	if err := json.Unmarshal(resp.Result, &raw); err != nil {
		return nil, fmt.Errorf("lspclient: decode workspace/symbol result: %w", err)
	}
	out := make([]LSPWorkspaceSymbol, 0, len(raw))
	for _, s := range raw {
		out = append(out, LSPWorkspaceSymbol{
			Name:          s.Name,
			Kind:          s.Kind,
			ContainerName: s.ContainerName,
			URI:           s.Location.URI,
			Line:          s.Location.Range.Start.Line,
			Character:     s.Location.Range.Start.Character,
		})
	}
	return out, nil
}

// SignatureHelp 请求 textDocument/signatureHelp 并压平成 LSPSignatureHelp
// params 是跟 hover/completion 同形的 TextDocumentPositionParams. 响应:
//
//	SignatureHelp{signatures []SignatureInformation, activeSignature, activeParameter}
//
// 其中每条 SignatureInformation 的 documentation 跟 hover.contents 同样是
//
//	string | MarkupContent | []MarkedString 多态, 直接复用 stringifyHoverContents.
//
// 形参 documentation 当前不暴露 (UI 只展 label), 真要时在 LSPSignature 上扩字段即可.
// server 返回 null (光标不在调用实参里) 时返回 (nil, nil), 不当错误.
// 受默认 10s SendRequest 超时约束.
func (c *LSPClient) SignatureHelp(uri string, line, character int) (*LSPSignatureHelp, error) {
	resp, err := c.SendRequest("textDocument/signatureHelp", textDocumentPositionParams(uri, line, character))
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return nil, nil
	}
	var sh struct {
		Signatures []struct {
			Label         string          `json:"label"`
			Documentation json.RawMessage `json:"documentation"`
			Parameters    []struct {
				Label string `json:"label"`
			} `json:"parameters"`
		} `json:"signatures"`
		ActiveSignature int `json:"activeSignature"`
		ActiveParameter int `json:"activeParameter"`
	}
	if err := json.Unmarshal(resp.Result, &sh); err != nil {
		return nil, fmt.Errorf("lspclient: decode signatureHelp result: %w", err)
	}
	out := &LSPSignatureHelp{
		ActiveSignature: sh.ActiveSignature,
		ActiveParameter: sh.ActiveParameter,
	}
	out.Signatures = make([]LSPSignature, 0, len(sh.Signatures))
	for _, s := range sh.Signatures {
		sig := LSPSignature{Label: s.Label}
		if len(s.Documentation) > 0 && string(s.Documentation) != "null" {
			doc, err := stringifyHoverContents(s.Documentation)
			if err != nil {
				return nil, fmt.Errorf("lspclient: decode signature documentation: %w", err)
			}
			sig.Documentation = doc
		}
		sig.Parameters = make([]string, 0, len(s.Parameters))
		for _, p := range s.Parameters {
			sig.Parameters = append(sig.Parameters, p.Label)
		}
		out.Signatures = append(out.Signatures, sig)
	}
	return out, nil
}

// LSPTextEdit 是一处文本编辑: 在 Range 区间上用 NewText 替换
// formatting/rename 都用它当返回单元 -- LSP 的 TextEdit 就是 {range, newText}.
type LSPTextEdit struct {
	Range   LSPRange `json:"range"`
	NewText string   `json:"newText"`
}

// LSPWorkspaceEdit 是 rename 跨文件改动的结果, 按 uri 分组的 TextEdit
// LSP 的 WorkspaceEdit 有两种合法形态 (changes map 与 documentChanges 数组),
// 这里统一压平到 Changes (uri -> edits), 让上层不必关心 server 选了哪种.
type LSPWorkspaceEdit struct {
	Changes map[string][]LSPTextEdit // uri -> edits
}

// LSPCommand 是 LSP 的 Command: 一个可执行命令的引用 (title + command + 参数)
// code action 既可能是裸 Command (顶层就是这个形状), 也可能是 CodeAction 里
// 内嵌的 command 字段. Arguments 保留成 RawMessage 原样透出 -- 不同 command
// 的参数形状各异, 留给上层在真正 workspace/executeCommand 时再解释.
type LSPCommand struct {
	Title     string          `json:"title"`
	Command   string          `json:"command"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// LSPCodeAction 是 code action 菜单里的一项 (灯泡里的 quick-fix / refactor)
// Title 必有; Kind 在 CodeAction 形态下有 (例如 "quickfix" / "refactor.extract"),
// 裸 Command 形态没有 kind 留空. Edit / Command 携带"如何应用这一项":
//   - Edit 非 nil 时是一份内联 WorkspaceEdit, 直接喂给 ApplyTextEdits 即可生效.
//   - Command 非 nil 时是一条待执行命令 (裸 Command 形态, 或 CodeAction 自带 command).
//
// 两者都可能缺省 (留 nil), 也可能同时存在. 真正执行 command 走 workspace/executeCommand,
// 那是后续 commit 的事, 这一版只负责把数据透出来.
type LSPCodeAction struct {
	Title   string
	Kind    string            // 裸 Command 形态为空
	Edit    *LSPWorkspaceEdit // 无内联编辑时为 nil
	Command *LSPCommand       // 无命令时为 nil
}

// Formatting 请求 textDocument/formatting 并返回把整篇文档格式化所需的编辑
// gopls 对 Go 文件等价于跑一遍 gofmt: 它要 options{tabSize, insertSpaces},
// Go 用 tab 缩进, 所以默认 {tabSize:4, insertSpaces:false} (insertSpaces=false
// 时 tabSize 仅作展示宽度提示, gopls 实际产出真 tab). 响应是 []TextEdit,
// null (无需改动 / server 不支持) 归一成空切片, 不当错误.
// 受默认 10s SendRequest 超时约束.
func (c *LSPClient) Formatting(uri string) ([]LSPTextEdit, error) {
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": uri},
		"options": map[string]interface{}{
			"tabSize":      4,
			"insertSpaces": false,
		},
	}
	resp, err := c.SendRequest("textDocument/formatting", params)
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return []LSPTextEdit{}, nil
	}
	var edits []LSPTextEdit
	if err := json.Unmarshal(resp.Result, &edits); err != nil {
		return nil, fmt.Errorf("lspclient: decode formatting result: %w", err)
	}
	return edits, nil
}

// Rename 请求 textDocument/rename 并把跨工作区的改动归一成 LSPWorkspaceEdit
// 响应里的 WorkspaceEdit 有两种合法形态, server 任选:
//   - changes:         {uri: []TextEdit}            -- 简单 map 形态, 优先吃这个
//   - documentChanges: [{textDocument:{uri}, edits:[]TextEdit}]  -- 带版本号的形态
//
// 处理顺序: 先看 changes, 非空就用; changes 缺省时再折叠 documentChanges
// 到同一个 Changes map (丢掉版本号, 上层只关心 uri->edits). 两者都给时以
// changes 为准 -- 简单形态信息无损, 不必再读版本化形态.
// server 返回 null (符号不可改 / 没有出现处) 时返回 (nil, nil), 不当错误.
// 受默认 10s SendRequest 超时约束.
func (c *LSPClient) Rename(uri string, line, character int, newName string) (*LSPWorkspaceEdit, error) {
	params := textDocumentPositionParams(uri, line, character)
	params["newName"] = newName
	resp, err := c.SendRequest("textDocument/rename", params)
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return nil, nil
	}
	return decodeWorkspaceEdit(resp.Result, "rename")
}

// decodeWorkspaceEdit 把一份 WorkspaceEdit 的原始 JSON 归一成 LSPWorkspaceEdit
// WorkspaceEdit 有两种合法形态, server 任选 (rename 响应与 code action 的 edit 字段同此):
//   - changes:         {uri: []TextEdit}            -- 简单 map 形态, 优先吃这个
//   - documentChanges: [{textDocument:{uri}, edits:[]TextEdit}]  -- 带版本号的形态
//
// 处理顺序: 先看 changes, 非空就用; changes 缺省时再折叠 documentChanges 到同一个
// Changes map (丢掉版本号, 上层只关心 uri->edits), 同一个 uri 出现多次就把 edits 串接.
// 两者都给时以 changes 为准 -- 简单形态信息无损, 不必再读版本化形态. what 仅用于
// 出错时拼报文 (调用方说明这份 edit 来自哪个请求).
func decodeWorkspaceEdit(raw json.RawMessage, what string) (*LSPWorkspaceEdit, error) {
	var we struct {
		Changes         map[string][]LSPTextEdit `json:"changes"`
		DocumentChanges []struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Edits []LSPTextEdit `json:"edits"`
		} `json:"documentChanges"`
	}
	if err := json.Unmarshal(raw, &we); err != nil {
		return nil, fmt.Errorf("lspclient: decode %s result: %w", what, err)
	}
	out := &LSPWorkspaceEdit{Changes: map[string][]LSPTextEdit{}}
	if len(we.Changes) > 0 {
		out.Changes = we.Changes
		return out, nil
	}
	// changes 缺省: 折叠 documentChanges, 同一个 uri 出现多次就把 edits 串接.
	for _, dc := range we.DocumentChanges {
		out.Changes[dc.TextDocument.URI] = append(out.Changes[dc.TextDocument.URI], dc.Edits...)
	}
	return out, nil
}

// CodeAction 请求 textDocument/codeAction 并把灯泡菜单项的标题列出来
// params 需要 range + context.diagnostics; 我们只想列"这个区间有哪些动作",
// 不带具体诊断, context.diagnostics 给空数组即可 (gopls 仍会给出 source/refactor 类项).
// 响应是一个数组, 每个元素是两种形态之一, server 混着发都合法:
//   - 裸 Command:  {title, command, arguments}        -- 只有 title, 无 kind
//   - CodeAction:  {title, kind, edit?, command?}     -- 有 kind
//
// 两者都带 title, 所以宽松解码: title 必取, kind / edit / command 有则取无则留空.
//   - edit 是一份内联 WorkspaceEdit, 复用 rename 同款双形态折叠 (decodeWorkspaceEdit),
//     压平到 Edit.Changes 供 ApplyTextEdits 应用.
//   - 裸 Command 形态没有 edit, 顶层的 command/title 折进 Command 字段.
//
// 缺省字段一律留 nil/空, 不在"形态合法但稀疏"的项上报错. 真正执行 command
// (workspace/executeCommand) 留作后续 commit, 这一版只把数据透出来.
// null (区间内无可用动作) 归一成空切片, 不当错误.
// 受默认 10s SendRequest 超时约束.
func (c *LSPClient) CodeAction(uri string, startLine, startChar, endLine, endChar int) ([]LSPCodeAction, error) {
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": uri},
		"range": LSPRange{
			Start: LSPPosition{Line: startLine, Character: startChar},
			End:   LSPPosition{Line: endLine, Character: endChar},
		},
		"context": map[string]interface{}{
			"diagnostics": []interface{}{},
		},
	}
	resp, err := c.SendRequest("textDocument/codeAction", params)
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return []LSPCodeAction{}, nil
	}
	// 宽松解码: Command 和 CodeAction 都带 title; kind 只有 CodeAction 有.
	// edit/command 先收成 RawMessage, 缺省或 null 时留 nil, 不强行构造空对象.
	var raw []struct {
		Title     string          `json:"title"`
		Kind      string          `json:"kind"`
		Edit      json.RawMessage `json:"edit"`
		Command   json.RawMessage `json:"command"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(resp.Result, &raw); err != nil {
		return nil, fmt.Errorf("lspclient: decode codeAction result: %w", err)
	}
	out := make([]LSPCodeAction, 0, len(raw))
	for _, a := range raw {
		act := LSPCodeAction{Title: a.Title, Kind: a.Kind}
		// edit: 一份内联 WorkspaceEdit, 复用 rename 的双形态折叠.
		if len(a.Edit) > 0 && string(a.Edit) != "null" {
			edit, err := decodeWorkspaceEdit(a.Edit, "codeAction edit")
			if err != nil {
				return nil, err
			}
			act.Edit = edit
		}
		// command 有两种来源: CodeAction 内嵌的 command 对象, 或裸 Command 形态
		// (command 是字符串, title/arguments 在顶层). 两种都折进 *LSPCommand;
		// 缺省时 decodeCommandField 返回 nil.
		act.Command = decodeCommandField(a.Command, a.Title, a.Arguments)
		out = append(out, act)
	}
	return out, nil
}

// decodeCommandField 把 code action 项里的 command 归一成 *LSPCommand
// LSP 把 command 编码成两种形态, 取决于这一项本身是 Command 还是 CodeAction:
//   - 对象形态:  "command": {title, command, arguments}   -- CodeAction 内嵌的 command
//   - 字符串形态: "command": "id", 顶层另有 title / arguments -- 裸 Command 项
//
// 对象形态直接解码; 字符串形态用顶层的 title + arguments 补齐. 缺省 (无 command
// 或 null) 返回 nil. 形态合法但稀疏不报错: 解不出对象就退回字符串形态尽力补齐.
func decodeCommandField(rawCmd json.RawMessage, topTitle string, topArgs json.RawMessage) *LSPCommand {
	if len(rawCmd) == 0 || string(rawCmd) == "null" {
		return nil
	}
	// 对象形态: {title, command, arguments}
	var obj LSPCommand
	if err := json.Unmarshal(rawCmd, &obj); err == nil && obj.Command != "" {
		return &obj
	}
	// 字符串形态: command 是一个 id, title/arguments 在顶层 (裸 Command 项).
	var id string
	if err := json.Unmarshal(rawCmd, &id); err != nil || id == "" {
		return nil
	}
	return &LSPCommand{Title: topTitle, Command: id, Arguments: topArgs}
}

// ExecuteCommand 请求 workspace/executeCommand, 真正执行一条 command-form 的动作
// CodeAction 返回的项分两类: 一类自带内联 edit (直接 ApplyTextEdits 就生效),
// 另一类只给一个 command + arguments (gopls 的 "organize imports" /
// "extract function" 等 refactor), 必须回抛给服务器执行 -- 那就是这个 RPC.
// 上层从 LSPCodeAction.Command 里取出 Command / Arguments 原样喂进来.
//
// params 形状是 {command, arguments}. arguments 是一组已经序列化好的原始 JSON
// (每条 command 的参数形态各异, 不在这层解释); nil 时补成空数组 [] 而不是 null --
// gopls 对 arguments 缺省/为 null 会报错, 空数组是最安全的取值.
//
// 返回服务器的原始 Result: 大多数 gopls 命令的副作用是反过来发一个
// workspace/applyEdit 请求, result 本身回 null, 这种情况归一成 (nil, nil);
// 个别命令会回一个 JSON 结果, 原样透出给上层解释. server 端错误经 SendRequest
// 包好后原样透出. 受默认 10s SendRequest 超时约束.
func (c *LSPClient) ExecuteCommand(command string, arguments []json.RawMessage) (json.RawMessage, error) {
	if arguments == nil {
		// nil 切片会被 encoding/json 编成 null; gopls 期望 arguments 是数组,
		// 补成空切片让它序列化成 [].
		arguments = []json.RawMessage{}
	}
	params := map[string]interface{}{
		"command":   command,
		"arguments": arguments,
	}
	resp, err := c.SendRequest("workspace/executeCommand", params)
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return nil, nil
	}
	return resp.Result, nil
}

// LSPDocumentHighlight 是 textDocument/documentHighlight 的一条命中: 光标下符号在
// 当前文件里的一处出现. Kind 标出这处是读还是写 (1=Text 通用, 2=Read, 3=Write),
// 现代编辑器据此把"读"和"写"用不同底色区分; server 可省略 kind, omitempty 收零值.
type LSPDocumentHighlight struct {
	Range LSPRange `json:"range"`
	Kind  int      `json:"kind,omitempty"` // 1=Text, 2=Read, 3=Write (可选)
}

// DocumentHighlight 请求 textDocument/documentHighlight 并返回光标下符号在*当前文件*
// 内的所有出现处 (编辑器里"选中一个标识符, 同文件内所有同名引用泛起淡色底"的效果).
// 跟 References 的区别: 只在本文件里找, 不跨文件, 也不带 context. params 是共用的
// TextDocumentPositionParams. 响应固定是 []DocumentHighlight (没有 definition 那种
// 多态), null (光标不在符号上) 归一成空切片, 不当错误.
// 受默认 10s SendRequest 超时约束.
func (c *LSPClient) DocumentHighlight(uri string, line, character int) ([]LSPDocumentHighlight, error) {
	resp, err := c.SendRequest("textDocument/documentHighlight", textDocumentPositionParams(uri, line, character))
	if err != nil {
		return nil, err
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return []LSPDocumentHighlight{}, nil
	}
	var highlights []LSPDocumentHighlight
	if err := json.Unmarshal(resp.Result, &highlights); err != nil {
		return nil, fmt.Errorf("lspclient: decode documentHighlight result: %w", err)
	}
	return highlights, nil
}

// DidChange 发 textDocument/didChange 通知, 把整个文件最新内容推给 server
// LSP 支持 incremental sync (按 range 发 diff), 这里走最简单的 *full document
// sync*: 一次性把整篇 fullText 重新塞过去. 对一个文件大小一般 < 1MB 的 Go
// 项目, 完全够用, 也避免维护一份精确的 diff 状态机. version 是单调递增的
// 文档版本号, 跟前一次 didOpen/didChange 的 version 配套递增, 让 server 能
// 判别响应里的位置是基于哪个版本算出来的.
// 通知不会有响应; 失败仅意味着写 pipe 失败.
func (c *LSPClient) DidChange(uri string, version int, fullText string) error {
	type textDocument struct {
		URI     string `json:"uri"`
		Version int    `json:"version"`
	}
	type contentChange struct {
		Text string `json:"text"`
	}
	type params struct {
		TextDocument   textDocument    `json:"textDocument"`
		ContentChanges []contentChange `json:"contentChanges"`
	}
	return c.SendNotification("textDocument/didChange", params{
		TextDocument:   textDocument{URI: uri, Version: version},
		ContentChanges: []contentChange{{Text: fullText}},
	})
}

// Notifications 暴露服务器主动推送的通知通道
// 典型消费者: publishDiagnostics, window/logMessage, window/showMessage.
// 通道是 buffered=64; 上层不消费时会丢消息 (见 routeMessage), 这是有意的:
// LSP 通知本质上 best-effort, 不应该让一个迟到的消费者把读循环堵死.
// 读循环退出时 (server 退出 / Close) 该通道会被 close, 因此上层可以放心用
// `for m := range c.Notifications()` 消费 -- 循环随会话结束而终止, 不会把
// drain goroutine 泄漏在每次 client 重启之后.
func (c *LSPClient) Notifications() <-chan *LSPMessage { return c.notifications }

// Close 优雅关闭: shutdown 请求 -> exit 通知 -> 关 stdin -> 等子进程 / 兜底 Kill
// 任何一步出错都不阻断后续: 最终目标是把子进程清掉, 不漏 fd / 进程.
// 跟 dlv.go 里 Close 的写法保持一致: 锁内只翻 closed 标志, 实际清理放在锁外.
func (c *LSPClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	// shutdown 是 best-effort. 设短超时, 防止已经死掉的 gopls 拖住 Close.
	c.shutdownBestEffort()

	if c.stdin != nil {
		_ = c.stdin.Close()
	}

	// 给读循环一点时间自然退出 (stdin 关后 gopls 通常会自己退)
	select {
	case <-c.done:
	case <-time.After(500 * time.Millisecond):
	}

	if c.cmd != nil && c.cmd.Process != nil {
		// 即便已经退出, Kill 也是安全的 (返回 process already finished)
		_ = c.cmd.Process.Kill()
		_, _ = c.cmd.Process.Wait()
	}
	return nil
}

// shutdownBestEffort 在 Close 期间尝试一次 "shutdown" 请求 + "exit" 通知
// 任何错误都吞掉. 这里不复用 SendRequest 是因为 closed 已经为 true 让它早退;
// 用一对独立的 write 走最短路径, 给 server 一个体面退场的机会.
func (c *LSPClient) shutdownBestEffort() {
	if c.stdin == nil {
		return
	}
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()
	if req, err := NewRequest(id, "shutdown", nil); err == nil {
		_ = c.writeFrame(req)
	}
	// shutdown 之后规范要求紧跟 exit notification (无 params)
	if note, err := NewNotification("exit", nil); err == nil {
		_ = c.writeFrame(note)
	}
}

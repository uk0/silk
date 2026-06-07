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
// EOF / 读错误 时退出循环, 关闭 done, 让 Close 知道读端已停.
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
			return
		}

		c.mu.Lock()
		pending := c.pending
		c.mu.Unlock()
		routeMessage(m, pending, c.notifications)
	}
}

// routeMessage 是单条 LSP 消息的"投递决策", 纯函数, 可单测
//   - 响应 (有 numeric ID 且 Result/Error 至少一个非空): 找 pending[id], 推到那个 channel
//   - 通知 (无 ID, 有 Method): 非阻塞推到 notif (满了就丢, 避免拖死读循环)
//   - 其它 (服务器发的 request -- LSP 允许, 例如 workspace/configuration):
//     当前实现不回, 静默丢弃; 未来需要时再加 server->client request 路由.
// 注意:
//   - 我们只识别数字 ID (NewRequest 总是写数字, 自洽).
//   - pending map 里的 channel 是 buffered=1, 不会阻塞读循环.
//   - 由于 readLoop 已经把 pending 的 *snapshot* 传进来, 这里不再加锁,
//     调用方负责把锁的活儿留在 readLoop 那一层.
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
//   1. SendRequest("initialize", params)  阻塞等响应
//   2. 拿到 Result 之后立刻发 "initialized" notification (规范要求)
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
//   "  -> string 形态
//   {  -> MarkupContent / MarkedString 对象形态
//   [  -> 数组形态 (string 或 MarkedString 混排)
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

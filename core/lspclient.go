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

	// handlerMu 单独护住 "宿主注册的回调 + 握手拿到的服务器状态".
	// 刻意不复用 mu: mu 在 readLoop 路由消息期间被持有, 而回 server->client
	// 请求时要读这几个字段; 分成两把锁, 即便将来把回包挪回路由锁内也不会自锁.
	handlerMu   sync.Mutex
	configFn    LSPConfigurationFunc
	applyEditFn LSPApplyEditFunc
	folders     []LSPWorkspaceFolder
	serverCaps  json.RawMessage // initialize 响应里的 capabilities 对象 (原始 JSON)

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

// LSPInitializeParams 是 "initialize" 请求体
// 早期版本只带 ProcessID + RootURI, 把 capabilities 整段省掉. 省掉的代价不是
// "少几个可选字段", 而是服务器会把所有 client 能力当 false 处理, 于是一整批
// 功能被静默降级:
//   - 不宣称 workspace.applyEdit / configuration -> gopls 不会发
//     workspace/applyEdit, command 形态的重构 (organize imports / extract)
//     执行完什么都不会发生;
//   - 不宣称 codeAction.codeActionLiteralSupport -> gopls 只回裸 Command,
//     LSPCodeAction.Edit 永远是 nil;
//   - 不宣称 semanticTokens / inlayHint / callHierarchy / typeHierarchy ->
//     对应的 provider 根本不出现在 server capabilities 里, 请求直接 MethodNotFound.
//
// 因此 Capabilities 缺省时 Initialize 会填 DefaultClientCapabilities():
// 一份"我们真的实现了"的能力集 (见 defaultClientCapabilitiesJSON). 调用方要
// 自定义时把自己的 JSON 塞进 Capabilities 即可, 不会被覆盖.
// WorkspaceFolders 缺省且 RootURI 非空时, 自动补一个以 RootURI 为根的 folder --
// 多模块工作区可以显式给多个.
//
// InitializationOptions 是服务器私有的启动设置. 有些能力只有在这里打开之后服务器
// 才会把它登记进 capabilities: 实测 gopls v0.22 要拿到 semanticTokensProvider 得给
// {"semanticTokens":true}, 要拿到非空的 inlay hint 得给 {"hints":{...}}.
type LSPInitializeParams struct {
	ProcessID             int                  `json:"processId"`
	RootURI               string               `json:"rootUri"`
	Capabilities          json.RawMessage      `json:"capabilities,omitempty"`
	WorkspaceFolders      []LSPWorkspaceFolder `json:"workspaceFolders,omitempty"`
	InitializationOptions json.RawMessage      `json:"initializationOptions,omitempty"`
}

// LSPWorkspaceFolder 是 initialize 的 workspaceFolders 项 (也是
// workspace/workspaceFolders 请求的响应单元)
type LSPWorkspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// defaultClientCapabilitiesJSON 是我们对外宣称的 client capabilities
// 原则: 只宣称 *这个包真的实现了* 的能力 —— 宣称多了会让服务器改用我们
// 处理不了的形态, 比这不宣称更糟. 三个刻意留白的点:
//   - codeAction.resolveSupport / completionItem.resolveSupport 都不宣称:
//     一旦宣称, gopls 会把 edit 延迟到 codeAction/resolve, 而我们没有实现
//     resolve, LSPCodeAction.Edit 会变回 nil.
//   - insertReplaceSupport 不宣称: 保持 completionItem.textEdit 是简单的
//     TextEdit 形态 (LSPCompletionItem.TextEdit 也能吃 InsertReplaceEdit, 但
//     没必要主动招惹).
//   - window.workDoneProgress 不宣称: 那会让服务器持续推 $/progress 通知,
//     把 Notifications() 通道 (buffered 64) 挤满, 挤掉真正有用的诊断.
//
// didChangeWatchedFiles.dynamicRegistration 同样保持 false: 我们没有文件系统
// 监听器, 宣称了就要负责发 workspace/didChangeWatchedFiles.
const defaultClientCapabilitiesJSON = `{
  "general": {"positionEncodings": ["utf-16"]},
  "workspace": {
    "applyEdit": true,
    "configuration": true,
    "workspaceFolders": true,
    "workspaceEdit": {
      "documentChanges": true,
      "resourceOperations": ["create", "rename", "delete"],
      "failureHandling": "textOnlyTransactional",
      "normalizesLineEndings": true
    },
    "didChangeConfiguration": {"dynamicRegistration": false},
    "didChangeWatchedFiles": {"dynamicRegistration": false},
    "executeCommand": {"dynamicRegistration": false},
    "symbol": {"dynamicRegistration": false},
    "semanticTokens": {"refreshSupport": true},
    "inlayHint": {"refreshSupport": true},
    "codeLens": {"refreshSupport": true},
    "diagnostics": {"refreshSupport": true}
  },
  "textDocument": {
    "synchronization": {"dynamicRegistration": false, "willSave": false, "willSaveWaitUntil": false, "didSave": true},
    "completion": {
      "dynamicRegistration": false,
      "contextSupport": true,
      "completionItem": {
        "snippetSupport": true,
        "commitCharactersSupport": false,
        "documentationFormat": ["markdown", "plaintext"],
        "deprecatedSupport": true,
        "preselectSupport": true,
        "insertReplaceSupport": false,
        "labelDetailsSupport": true
      }
    },
    "hover": {"contentFormat": ["markdown", "plaintext"]},
    "signatureHelp": {"signatureInformation": {"documentationFormat": ["markdown", "plaintext"]}},
    "declaration": {"linkSupport": false},
    "definition": {"linkSupport": false},
    "typeDefinition": {"linkSupport": false},
    "implementation": {"linkSupport": false},
    "references": {"dynamicRegistration": false},
    "documentHighlight": {"dynamicRegistration": false},
    "documentSymbol": {"hierarchicalDocumentSymbolSupport": true},
    "codeAction": {
      "codeActionLiteralSupport": {
        "codeActionKind": {
          "valueSet": ["", "quickfix", "refactor", "refactor.extract", "refactor.inline",
                       "refactor.rewrite", "source", "source.organizeImports", "source.fixAll"]
        }
      },
      "isPreferredSupport": true,
      "honorsChangeAnnotations": false
    },
    "codeLens": {"dynamicRegistration": false},
    "formatting": {"dynamicRegistration": false},
    "rangeFormatting": {"dynamicRegistration": false},
    "rename": {"prepareSupport": true, "prepareSupportDefaultBehavior": 1},
    "publishDiagnostics": {"relatedInformation": true, "versionSupport": true, "tagSupport": {"valueSet": [1, 2]}},
    "callHierarchy": {"dynamicRegistration": false},
    "typeHierarchy": {"dynamicRegistration": false},
    "inlayHint": {"dynamicRegistration": false},
    "documentLink": {"tooltipSupport": true},
    "foldingRange": {"lineFoldingOnly": true},
    "selectionRange": {"dynamicRegistration": false},
    "semanticTokens": {
      "dynamicRegistration": false,
      "requests": {"range": false, "full": {"delta": true}},
      "formats": ["relative"],
      "overlappingTokenSupport": false,
      "multilineTokenSupport": true,
      "augmentsSyntaxTokens": false,
      "tokenTypes": ["namespace", "type", "class", "enum", "interface", "struct", "typeParameter",
                     "parameter", "variable", "property", "enumMember", "event", "function", "method",
                     "macro", "keyword", "modifier", "comment", "string", "number", "regexp",
                     "operator", "decorator", "label"],
      "tokenModifiers": ["declaration", "definition", "readonly", "static", "deprecated", "abstract",
                         "async", "modification", "documentation", "defaultLibrary"]
    }
  },
  "window": {"showMessage": {}, "showDocument": {"support": false}}
}`

// DefaultClientCapabilities 返回 Initialize 在 Capabilities 缺省时用的能力集
// 返回的是一份拷贝: 调用方可以在它上面二次加工 (比如塞进自己的 initialize
// params) 而不影响后续调用.
func DefaultClientCapabilities() json.RawMessage {
	return append(json.RawMessage(nil), defaultClientCapabilitiesJSON...)
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

		// server->client 请求 (workspace/configuration, workspace/applyEdit,
		// client/registerCapability ...) 既不查 pending 也不进 notifications,
		// 所以不需要 c.mu; 单独走锁外的一趟路由, 免得回包那次 stdin 写把 c.mu
		// 一直攥在手里, 拖住并发的 SendRequest/SendNotification.
		if isServerRequest(m) {
			routeMessage(m, nil, nil, c)
			continue
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

// isServerRequest 判定一条消息是不是 *服务器发给我们* 的请求
// 特征: 有 ID (要求回复) + 有 Method, 且没有 Result/Error (那是响应的特征).
func isServerRequest(m *LSPMessage) bool {
	return m != nil && m.ID != nil && m.Method != "" && len(m.Result) == 0 && m.Error == nil
}

// routeMessage 是单条 LSP 消息的"投递决策", 纯函数, 可单测
//   - 响应 (有 numeric ID 且 Result/Error 至少一个非空): 找 pending[id], 推到那个 channel
//   - 通知 (无 ID, 有 Method): 非阻塞推到 notif (满了就丢, 避免拖死读循环)
//   - 服务器发的 request (有 ID + Method, 无 Result/Error -- LSP 允许, 例如
//     workspace/configuration / workspace/applyEdit): 交给可选的 responder 回包.
//     没给 responder 时保持旧行为静默丢弃 (纯路由单测就是这么调的).
//
// 注意:
//   - 我们只识别数字 ID (NewRequest 总是写数字, 自洽).
//   - pending map 里的 channel 是 buffered=1, 不会阻塞读循环.
//   - readLoop 在 c.mu 锁内调用本函数 (pending 与 SendRequest 的写并发,
//     必须串行化); 这里只做 map 查找 + 非阻塞 channel 推送, 临界区极短.
//     单测里直接传一个独占的 map (无并发写), 同样安全.
//   - responder 分支例外: 那条路径要写 stdin, readLoop 在锁 *外* 单独调
//     (见 isServerRequest), 所以传 pending/notif 都可以为 nil.
func routeMessage(m *LSPMessage, pending map[int]chan *LSPMessage, notif chan<- *LSPMessage, responder ...serverRequestResponder) {
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
		// 服务器发的 request: 有 responder 就回包, 没有就保持旧行为丢弃.
		if m.Method != "" && len(responder) > 0 && responder[0] != nil {
			responder[0].respondToServerRequest(m)
		}
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

// -----------------------------------------------------------------------------
// server -> client 请求: 必须回包, 否则服务器会一直等
// -----------------------------------------------------------------------------
//
// LSP 是双向的: 服务器也会给客户端发 *请求*, 而 JSON-RPC 的请求必须有响应.
// 旧实现把这类消息静默丢掉, 后果不是"少一个可选特性", 而是两条主线功能死掉:
//   - workspace/configuration: gopls 在初始化后问客户端要设置, 拿不到就一直
//     等那个 id 的响应, 期间一部分分析工作挂起.
//   - workspace/applyEdit:     command 形态的重构 (source.organizeImports /
//     extract function) 的真实产出就是这个请求. 不回, 编辑永远落不了地 ——
//     用户看到的现象是 "点了整理导入, 什么都没发生".
//
// 处理策略:
//   - workspace/configuration -> 每个 item 回一份配置; 没注册 handler 就回 {}.
//   - workspace/applyEdit     -> 交给宿主注册的 handler 决定, 回 {applied}.
//   - workspace/workspaceFolders -> 回 Initialize 时给过的 folder 列表.
//   - 一批"知会型"请求 (registerCapability / 各种 refresh / workDoneProgress
//     create ...) -> 回 null 表示收到, 服务器就能往下走.
//   - 其余未知请求 -> 按 JSON-RPC 回 MethodNotFound, 同样不让服务器悬着.

// serverRequestResponder 是 routeMessage 把 server->client 请求交出去的出口
// 由 *LSPClient 实现; 抽成接口是为了让 routeMessage 保持"纯路由", 单测里可以
// 换成任意别的实现.
type serverRequestResponder interface {
	respondToServerRequest(m *LSPMessage)
}

// LSPConfigurationFunc 为 workspace/configuration 的一个 item 提供配置
// scopeURI 是请求作用域 (通常是某个 workspace folder, 可能为空), section 是
// 配置命名空间 (gopls 问的是 "gopls"). 返回 nil 表示"这一项没有配置", 客户端
// 会回一个空对象 {} —— 服务器据此走它自己的默认值.
type LSPConfigurationFunc func(scopeURI, section string) json.RawMessage

// LSPApplyEditFunc 处理服务器发来的 workspace/applyEdit
// 宿主拿到已经解好的 LSPWorkspaceEdit (含 documentChanges 的版本号与资源操作),
// 把它落到编辑器/磁盘上, 然后返回是否真的应用成功 —— 这个布尔值会作为
// {"applied": bool} 回给服务器, 服务器据此决定 executeCommand 是成功还是失败.
type LSPApplyEditFunc func(label string, edit *LSPWorkspaceEdit) bool

// SetConfigurationHandler 注册 workspace/configuration 的配置提供者
// 不注册时客户端仍然会回包 (每项 {}), 只是不带任何自定义设置. 可以传 nil 撤销.
func (c *LSPClient) SetConfigurationHandler(fn LSPConfigurationFunc) {
	c.handlerMu.Lock()
	c.configFn = fn
	c.handlerMu.Unlock()
}

// SetApplyEditHandler 注册 workspace/applyEdit 的落地器
// 不注册时客户端回 {"applied": false, "failureReason": ...}: 协议上仍然完整
// (服务器不会悬着), 但编辑不会生效. 可以传 nil 撤销.
func (c *LSPClient) SetApplyEditHandler(fn LSPApplyEditFunc) {
	c.handlerMu.Lock()
	c.applyEditFn = fn
	c.handlerMu.Unlock()
}

// ServerCapabilities 返回 initialize 响应里的 capabilities 对象 (原始 JSON)
// 没握手 / 服务器没给时返回 nil. 用途举例: 从 semanticTokensProvider.legend
// 里读 token 类型表, 好把 SemanticTokensFull 的数字下标翻成名字.
func (c *LSPClient) ServerCapabilities() json.RawMessage {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	if len(c.serverCaps) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), c.serverCaps...)
}

// respondToServerRequest 算出响应并写回去, 是 serverRequestResponder 的实现
// 取回调/folder 用 handlerMu (跟 readLoop 的 mu 不是同一把), 真正的回包内容由
// 纯函数 buildServerRequestReply 决定, 方便单测.
func (c *LSPClient) respondToServerRequest(m *LSPMessage) {
	c.handlerMu.Lock()
	ctx := serverRequestContext{
		Configuration: c.configFn,
		ApplyEdit:     c.applyEditFn,
		Folders:       c.folders,
	}
	c.handlerMu.Unlock()
	reply := buildServerRequestReply(m, ctx)
	if reply == nil {
		return
	}
	_ = c.writeFrame(reply)
}

// serverRequestContext 是回一条 server->client 请求所需要的全部宿主状态
// 单独成型是为了让 buildServerRequestReply 保持纯函数: 单测直接构造它.
type serverRequestContext struct {
	Configuration LSPConfigurationFunc
	ApplyEdit     LSPApplyEditFunc
	Folders       []LSPWorkspaceFolder
}

// lspMethodNotFound 是 JSON-RPC 的 MethodNotFound 错误码
// 双向都用得到: 我们回给服务器的未知请求, 以及服务器回给我们的"没这个能力".
const lspMethodNotFound = -32601

// serverRequestAck 是"收到即确认"的 server->client 请求集合
// 这些请求的响应内容对我们没有意义 (客户端没有对应 UI 动作), 但必须回, 否则
// 服务器会一直等. 回 null 是合法响应: 对 showMessageRequest 表示"用户没选",
// 对 register/unregisterCapability 与各种 refresh 表示"已收到".
var serverRequestAck = map[string]bool{
	"client/registerCapability":        true,
	"client/unregisterCapability":      true,
	"window/showMessageRequest":        true,
	"window/workDoneProgress/create":   true,
	"workspace/semanticTokens/refresh": true,
	"workspace/inlayHint/refresh":      true,
	"workspace/codeLens/refresh":       true,
	"workspace/diagnostic/refresh":     true,
}

// buildServerRequestReply 根据一条 server->client 请求算出要回的响应, 纯函数
// 返回 nil 表示"这条消息不是需要回复的服务器请求" (调用方不写任何字节).
func buildServerRequestReply(m *LSPMessage, ctx serverRequestContext) *LSPMessage {
	if m == nil || m.ID == nil || m.Method == "" {
		return nil
	}
	switch m.Method {
	case "workspace/configuration":
		return serverReplyResult(m, configurationResult(m.Params, ctx.Configuration))
	case "workspace/applyEdit":
		return serverReplyResult(m, applyEditResult(m.Params, ctx.ApplyEdit))
	case "workspace/workspaceFolders":
		return serverReplyResult(m, workspaceFoldersResult(ctx.Folders))
	}
	if serverRequestAck[m.Method] {
		return serverReplyResult(m, json.RawMessage(`null`))
	}
	return &LSPMessage{
		JSONRPC: "2.0",
		ID:      m.ID,
		Error: &LSPError{
			Code:    lspMethodNotFound,
			Message: "lspclient: unhandled server request " + m.Method,
		},
	}
}

// serverReplyResult 把一份 result 包成"回给服务器的响应" (ID 原样回传)
// ID 直接复用服务器给的那份原始字节, 数字/字符串两种形态都无损.
func serverReplyResult(req *LSPMessage, result json.RawMessage) *LSPMessage {
	return &LSPMessage{JSONRPC: "2.0", ID: req.ID, Result: result}
}

// configurationResult 拼 workspace/configuration 的响应
// 规范要求响应是一个数组, 长度跟 params.items 一一对应. 缺一项服务器就会按
// 顺序错位取值, 所以这里严格按 items 数量产出; 没有 handler 或 handler 回 nil
// 的项一律填 {} (等价于"我没有这项设置", 服务器用自己的默认值).
func configurationResult(params json.RawMessage, fn LSPConfigurationFunc) json.RawMessage {
	var p struct {
		Items []struct {
			ScopeURI string `json:"scopeUri"`
			Section  string `json:"section"`
		} `json:"items"`
	}
	if len(params) > 0 {
		// 畸形 params 不报错: 当成"没有 items", 回空数组照样让服务器往下走.
		_ = json.Unmarshal(params, &p)
	}
	out := make([]json.RawMessage, 0, len(p.Items))
	for _, item := range p.Items {
		var v json.RawMessage
		if fn != nil {
			v = fn(item.ScopeURI, item.Section)
		}
		if len(v) == 0 {
			v = json.RawMessage(`{}`)
		}
		out = append(out, v)
	}
	b, err := json.Marshal(out)
	if err != nil {
		// handler 给了非法 JSON: 宁可回空数组也不要漏掉响应.
		return json.RawMessage(`[]`)
	}
	return b
}

// applyEditResult 拼 workspace/applyEdit 的响应 (ApplyWorkspaceEditResult)
// 形状是 {applied, failureReason?}. 没注册 handler 时明确回 applied:false 并
// 附原因 —— 服务器据此把 executeCommand 标记成失败, 比谎报成功要好.
func applyEditResult(params json.RawMessage, fn LSPApplyEditFunc) json.RawMessage {
	var p struct {
		Label string          `json:"label"`
		Edit  json.RawMessage `json:"edit"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}
	if fn == nil {
		return json.RawMessage(`{"applied":false,"failureReason":"no applyEdit handler registered"}`)
	}
	edit := &LSPWorkspaceEdit{Changes: map[string][]LSPTextEdit{}}
	if len(p.Edit) > 0 && string(p.Edit) != "null" {
		decoded, err := decodeWorkspaceEdit(p.Edit, "applyEdit")
		if err != nil {
			return json.RawMessage(`{"applied":false,"failureReason":"malformed workspace edit"}`)
		}
		edit = decoded
	}
	if fn(p.Label, edit) {
		return json.RawMessage(`{"applied":true}`)
	}
	return json.RawMessage(`{"applied":false}`)
}

// workspaceFoldersResult 拼 workspace/workspaceFolders 的响应
// 没有 folder 时回 null (规范里的"没有工作区"), 而不是空数组.
func workspaceFoldersResult(folders []LSPWorkspaceFolder) json.RawMessage {
	if len(folders) == 0 {
		return json.RawMessage(`null`)
	}
	b, err := json.Marshal(folders)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return b
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
//  1. 补默认 capabilities / workspaceFolders (调用方给了就不动)
//  2. SendRequest("initialize", params)  阻塞等响应
//  3. 记下服务器 capabilities (供 ServerCapabilities / 能力预判用)
//  4. 发 "initialized" notification (规范要求)
//
// 返回的是 initialize 响应里的原始 Result (包含 server capabilities 等),
// 调用方按需 json.Unmarshal 到自己关心的结构上.
func (c *LSPClient) Initialize(params LSPInitializeParams) (json.RawMessage, error) {
	if len(params.Capabilities) == 0 {
		params.Capabilities = DefaultClientCapabilities()
	}
	if len(params.WorkspaceFolders) == 0 && params.RootURI != "" {
		params.WorkspaceFolders = []LSPWorkspaceFolder{{
			URI:  params.RootURI,
			Name: workspaceFolderName(params.RootURI),
		}}
	}
	// folder 列表先存下来: 服务器可能在 initialized 之后立刻反问
	// workspace/workspaceFolders, 那时得答得上来.
	c.handlerMu.Lock()
	c.folders = params.WorkspaceFolders
	c.handlerMu.Unlock()

	resp, err := c.SendRequest("initialize", params)
	if err != nil {
		return nil, fmt.Errorf("lspclient: initialize: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("lspclient: initialize error: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}
	c.storeServerCapabilities(resp.Result)
	// 立刻发 initialized -- 规范要求 server 在收到这个 notification 后才接受
	// 后续的请求 (gopls 实测在收到之前会拒绝 textDocument/* 类调用).
	if err := c.SendNotification("initialized", struct{}{}); err != nil {
		return resp.Result, fmt.Errorf("lspclient: send initialized: %w", err)
	}
	return resp.Result, nil
}

// workspaceFolderName 从 rootUri 里取最后一段当 folder 名
// "file:///home/me/proj" -> "proj". 取不到时退回整个 URI: 名字只用于服务器
// 侧的日志/展示, 不参与任何判定, 不值得为它报错.
func workspaceFolderName(rootURI string) string {
	trimmed := strings.TrimRight(rootURI, "/")
	if i := strings.LastIndexByte(trimmed, '/'); i >= 0 && i+1 < len(trimmed) {
		return trimmed[i+1:]
	}
	if trimmed == "" {
		return rootURI
	}
	return trimmed
}

// storeServerCapabilities 从 initialize 响应里抠出 capabilities 对象存下来
// 存的是原始 JSON: 服务器的能力描述形态各异 (bool / options 对象 /
// registrationOptions), 提前解成固定结构只会丢信息.
func (c *LSPClient) storeServerCapabilities(initResult json.RawMessage) {
	if len(initResult) == 0 {
		return
	}
	var r struct {
		Capabilities json.RawMessage `json:"capabilities"`
	}
	if err := json.Unmarshal(initResult, &r); err != nil {
		return
	}
	c.handlerMu.Lock()
	c.serverCaps = r.Capabilities
	c.handlerMu.Unlock()
}

// serverSupports 预判服务器有没有某个 provider 能力
// 没握手 (serverCaps 为空) 时一律返回 true: 不知道就别拦, 让请求真跑一次.
func (c *LSPClient) serverSupports(provider string) bool {
	c.handlerMu.Lock()
	caps := c.serverCaps
	c.handlerMu.Unlock()
	return capabilitySupported(caps, provider)
}

// capabilitySupported 在一份 server capabilities 上查某个 provider 键, 纯函数
// 规范要求服务器把自己支持的每个能力都登记在 capabilities 里, 形态可以是:
//
//	"implementationProvider": true            布尔
//	"inlayHintProvider": {}                   空 options 对象 (仍表示支持)
//	"semanticTokensProvider": {"legend": ...}  带 options
//
// 因此判定规则是"键存在且值不是 false/null". caps 为空 (没握手/服务器没给)
// 时返回 true —— 未知不等于不支持, 交给真实请求去判定.
func capabilitySupported(caps json.RawMessage, provider string) bool {
	if len(caps) == 0 {
		return true
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(caps, &m); err != nil || len(m) == 0 {
		return true
	}
	v, ok := m[provider]
	if !ok {
		return false
	}
	s := string(bytes.TrimSpace(v))
	return s != "false" && s != "null" && s != ""
}

// unsupportedMethod 判定一次失败的 SendRequest 是不是"服务器没实现这个 method"
// JSON-RPC 对未实现的方法回 -32601; 那不是错误状态, 是能力缺失, 上层应该当
// "空结果"处理而不是把错误弹给用户.
func unsupportedMethod(resp *LSPMessage) bool {
	return resp != nil && resp.Error != nil && resp.Error.Code == lspMethodNotFound
}

// requestOptional 发一条"服务器可能压根没实现"的请求, 把能力缺失跟真错误分开
// 返回 (result, ok, err):
//   - ok=false, err=nil  服务器没这个能力 (capabilities 里没登记, 或回了
//     MethodNotFound), 或结果就是 null -> 上层按空结果处理, 不当错误
//   - ok=false, err!=nil 真错误 (超时 / 连接断 / 服务器内部错)
//   - ok=true            result 是非 null 的原始结果
//
// provider 为空串时跳过 capabilities 预判, 直接发请求.
func (c *LSPClient) requestOptional(method, provider string, params interface{}) (json.RawMessage, bool, error) {
	if provider != "" && !c.serverSupports(provider) {
		return nil, false, nil
	}
	resp, err := c.SendRequest(method, params)
	if err != nil {
		if unsupportedMethod(resp) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if len(resp.Result) == 0 || string(bytes.TrimSpace(resp.Result)) == "null" {
		return nil, false, nil
	}
	return resp.Result, true, nil
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
// gopls 在补全里把签名 / 注释往 Detail/Documentation 里塞, Label 是用户看到的
// 标识. 光有 Label/InsertText 是不够的, 少了下面这些字段, 补全在编辑器里会明显
// 不对劲:
//   - TextEdit            服务器给的精确替换区间. 只按 InsertText 在光标处插入,
//     遇到"已经打了半个前缀"或需要往前吃掉一段 (比如把 foo.Bar 补成 (*foo).Bar)
//     就会出现重复文本. 有 TextEdit 时必须用它, 而不是自己猜区间.
//   - AdditionalTextEdits 补全的连带编辑, 典型就是 gopls 的 "unimported
//     completion": 选中 fmt.Println 时顺手把 import "fmt" 加进文件头. 丢掉
//     它意味着补全出来的代码编译不过.
//   - InsertTextFormat    2 = Snippet, 文本里带 $1/${1:name} 占位符; 当纯文本
//     插进去用户会看到一串 $1.
//   - SortText/FilterText 服务器给的排序/过滤键. 拿 Label 排序会把 gopls 精心
//     排好的相关度打乱 (它靠 sortText 前缀控制次序).
//   - Documentation       悬浮文档, 跟 hover 一样是 string|MarkupContent 多态,
//     这里压平成字符串.
//
// 两个多态字段 (documentation / textEdit) 用不了结构体 tag 直接解, 所以本类型
// 自带 UnmarshalJSON.
type LSPCompletionItem struct {
	Label      string `json:"label"`
	Detail     string `json:"detail,omitempty"`
	Kind       int    `json:"kind,omitempty"`
	InsertText string `json:"insertText,omitempty"`

	InsertTextFormat    int           `json:"insertTextFormat,omitempty"` // 1=PlainText, 2=Snippet
	SortText            string        `json:"sortText,omitempty"`
	FilterText          string        `json:"filterText,omitempty"`
	Documentation       string        `json:"documentation,omitempty"` // 压平后的文档串
	TextEdit            *LSPTextEdit  `json:"textEdit,omitempty"`
	AdditionalTextEdits []LSPTextEdit `json:"additionalTextEdits,omitempty"`
}

// UnmarshalJSON 解一条 CompletionItem, 顺带把两个多态字段压平
// 规范里这两个字段可以是两种形状, 直接用 tag 解会整条报错:
//   - documentation: string | MarkupContent{kind, value}
//   - textEdit:      TextEdit{range, newText} | InsertReplaceEdit{insert, replace, newText}
//
// 做法是先解进影子结构 (多态字段收成 RawMessage), 再逐个压平. 压不动的多态值
// 按"缺省"处理而不是报错: 一个字段的形状怪异不应该让整份补全列表作废.
func (it *LSPCompletionItem) UnmarshalJSON(data []byte) error {
	var raw struct {
		Label               string          `json:"label"`
		Detail              string          `json:"detail"`
		Kind                int             `json:"kind"`
		InsertText          string          `json:"insertText"`
		InsertTextFormat    int             `json:"insertTextFormat"`
		SortText            string          `json:"sortText"`
		FilterText          string          `json:"filterText"`
		Documentation       json.RawMessage `json:"documentation"`
		TextEdit            json.RawMessage `json:"textEdit"`
		AdditionalTextEdits []LSPTextEdit   `json:"additionalTextEdits"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*it = LSPCompletionItem{
		Label:               raw.Label,
		Detail:              raw.Detail,
		Kind:                raw.Kind,
		InsertText:          raw.InsertText,
		InsertTextFormat:    raw.InsertTextFormat,
		SortText:            raw.SortText,
		FilterText:          raw.FilterText,
		AdditionalTextEdits: raw.AdditionalTextEdits,
		TextEdit:            decodeCompletionTextEdit(raw.TextEdit),
	}
	if len(raw.Documentation) > 0 && string(raw.Documentation) != "null" {
		// 复用 hover 的压平逻辑 (string / {value} / 数组三种形态都吃).
		if doc, err := stringifyHoverContents(raw.Documentation); err == nil {
			it.Documentation = doc
		}
	}
	return nil
}

// decodeCompletionTextEdit 把 completionItem.textEdit 归一成 *LSPTextEdit
// TextEdit 给 range; InsertReplaceEdit 给 insert + replace 两个区间 (前者是
// "只插入"的范围, 后者是"连带覆盖已有标识符"的范围). 我们取 replace: 那是
// 编辑器里按 Tab 接受补全时的默认行为. 缺 range 又缺 insert/replace 的对象不是
// 合法编辑, 返回 nil 让上层退回 InsertText 路径.
func decodeCompletionTextEdit(raw json.RawMessage) *LSPTextEdit {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var te struct {
		Range   *LSPRange `json:"range"`
		Insert  *LSPRange `json:"insert"`
		Replace *LSPRange `json:"replace"`
		NewText string    `json:"newText"`
	}
	if err := json.Unmarshal(raw, &te); err != nil {
		return nil
	}
	out := &LSPTextEdit{NewText: te.NewText}
	switch {
	case te.Range != nil:
		out.Range = *te.Range
	case te.Replace != nil:
		out.Range = *te.Replace
	case te.Insert != nil:
		out.Range = *te.Insert
	default:
		return nil
	}
	return out
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

// LSPWorkspaceEdit 是 rename / code action / applyEdit 跨文件改动的结果
// LSP 的 WorkspaceEdit 有两种合法形态 (changes map 与 documentChanges 数组):
//   - Changes         两种形态都会填, 压平成 uri -> edits, 老调用方照旧可用.
//   - DocumentChanges 仅 documentChanges 形态有. 它比 Changes 多两样 *不能丢*
//     的信息: 每个文档的版本号 (拿它跟编辑器里的 version 对一下, 就知道这份
//     edit 是不是已经过期), 以及 create/rename/delete 三种资源操作 —— 后者
//     压根不是文本编辑, 压平到 Changes 里会彻底消失, 于是 "把文件重命名" 这类
//     重构表现成"什么都没发生".
//
// 顺序也只有 DocumentChanges 保得住: 资源操作跟文本编辑是有先后的 (先建文件
// 再往里写), map 遍历没有顺序.
type LSPWorkspaceEdit struct {
	Changes         map[string][]LSPTextEdit // uri -> edits (两种形态都填)
	DocumentChanges []LSPDocumentChange      // 保序的 documentChanges; changes-only 形态下为空
}

// LSPDocumentChange 是 documentChanges 数组里的一项
// Kind 决定其余字段怎么读:
//
//	"edit"   对 URI 的文本编辑, Edits 有效; Version/Versioned 是服务器算这份
//	         edit 时看到的文档版本 (可选, 服务器给 null 时 Versioned=false)
//	"create" 新建 URI; IgnoreIfExists/Overwrite 是可选选项
//	"rename" 把 URI 改名成 NewURI; IgnoreIfExists/Overwrite 同上
//	"delete" 删除 URI; Recursive 表示目录递归删, IgnoreIfNotExists 容忍不存在
type LSPDocumentChange struct {
	Kind      string        // "edit" | "create" | "rename" | "delete"
	URI       string        // edit/create/delete 的目标; rename 时是 oldUri
	NewURI    string        // 仅 rename: newUri
	Version   int           // 仅 edit: textDocument.version
	Versioned bool          // version 是否真的给了 (区分"版本 0"与"没给版本")
	Edits     []LSPTextEdit // 仅 edit

	Overwrite         bool // create/rename 选项
	IgnoreIfExists    bool // create/rename 选项
	Recursive         bool // delete 选项
	IgnoreIfNotExists bool // delete 选项
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
// WorkspaceEdit 有两种合法形态, server 任选 (rename 响应 / code action 的 edit /
// workspace/applyEdit 的 params.edit 都是这个结构):
//   - changes:         {uri: []TextEdit}                          -- 简单 map 形态
//   - documentChanges: [TextDocumentEdit | Create | Rename | Delete] -- 版本化 + 资源操作形态
//
// 两条都会解:
//   - Changes:         有 changes 就直接用 (信息无损); 否则由 documentChanges 里
//     的文本编辑折叠而来, 同一个 uri 出现多次就把 edits 串接.
//   - DocumentChanges: 只要 documentChanges 存在就保序解出来, 连版本号和
//     create/rename/delete 一起带上 —— 资源操作在 Changes 里没有容身之处.
//
// what 仅用于出错时拼报文 (调用方说明这份 edit 来自哪个请求).
func decodeWorkspaceEdit(raw json.RawMessage, what string) (*LSPWorkspaceEdit, error) {
	var we struct {
		Changes         map[string][]LSPTextEdit `json:"changes"`
		DocumentChanges []json.RawMessage        `json:"documentChanges"`
	}
	if err := json.Unmarshal(raw, &we); err != nil {
		return nil, fmt.Errorf("lspclient: decode %s result: %w", what, err)
	}
	out := &LSPWorkspaceEdit{Changes: map[string][]LSPTextEdit{}}
	if len(we.DocumentChanges) > 0 {
		changes, err := decodeDocumentChanges(we.DocumentChanges, what)
		if err != nil {
			return nil, err
		}
		out.DocumentChanges = changes
	}
	if len(we.Changes) > 0 {
		out.Changes = we.Changes
		return out, nil
	}
	// changes 缺省: 折叠 documentChanges 里的文本编辑 (资源操作只留在
	// DocumentChanges 里, 它们没有 uri->edits 的表示).
	for _, dc := range out.DocumentChanges {
		if dc.Kind != "edit" {
			continue
		}
		out.Changes[dc.URI] = append(out.Changes[dc.URI], dc.Edits...)
	}
	return out, nil
}

// decodeDocumentChanges 解 documentChanges 数组, 保序
// 元素形态靠 "kind" 字段区分: create/rename/delete 三种资源操作一定带 kind,
// TextDocumentEdit 一定不带 (它是 {textDocument, edits}), 于是没有 kind 的按
// "edit" 处理.
func decodeDocumentChanges(raw []json.RawMessage, what string) ([]LSPDocumentChange, error) {
	out := make([]LSPDocumentChange, 0, len(raw))
	for _, item := range raw {
		var dc struct {
			Kind         string `json:"kind"`
			TextDocument struct {
				URI     string `json:"uri"`
				Version *int   `json:"version"`
			} `json:"textDocument"`
			Edits   []LSPTextEdit `json:"edits"`
			URI     string        `json:"uri"`    // create/delete
			OldURI  string        `json:"oldUri"` // rename
			NewURI  string        `json:"newUri"` // rename
			Options struct {
				Overwrite         bool `json:"overwrite"`
				IgnoreIfExists    bool `json:"ignoreIfExists"`
				Recursive         bool `json:"recursive"`
				IgnoreIfNotExists bool `json:"ignoreIfNotExists"`
			} `json:"options"`
		}
		if err := json.Unmarshal(item, &dc); err != nil {
			return nil, fmt.Errorf("lspclient: decode %s documentChanges entry: %w", what, err)
		}
		change := LSPDocumentChange{
			Kind:              dc.Kind,
			NewURI:            dc.NewURI,
			Overwrite:         dc.Options.Overwrite,
			IgnoreIfExists:    dc.Options.IgnoreIfExists,
			Recursive:         dc.Options.Recursive,
			IgnoreIfNotExists: dc.Options.IgnoreIfNotExists,
		}
		switch dc.Kind {
		case "create", "delete":
			change.URI = dc.URI
		case "rename":
			change.URI = dc.OldURI
		default:
			// 没有 kind -> TextDocumentEdit. version 是 integer|null, 只有真给了
			// 数字才算"版本已知".
			change.Kind = "edit"
			change.URI = dc.TextDocument.URI
			change.Edits = dc.Edits
			if dc.TextDocument.Version != nil {
				change.Version = *dc.TextDocument.Version
				change.Versioned = true
			}
		}
		out = append(out, change)
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

// -----------------------------------------------------------------------------
// 进阶协议: prepareRename / implementation / call & type hierarchy /
//           inlayHint / semanticTokens / codeLens
// -----------------------------------------------------------------------------
//
// 共同约定 (全部走 requestOptional):
//   - 服务器没登记对应 provider, 或回 MethodNotFound, 或结果是 null ->
//     返回空结果 + nil error. 这些 method 是"能有就有"的增强, 服务器不支持时
//     UI 该做的是不显示这块, 而不是弹错误.
//   - 真错误 (超时 / 连接断 / 服务器内部错) 照常返回 error.
//   - 全部受 SendRequest 的 10s 默认超时约束.

// LSPPrepareRename 是 textDocument/prepareRename 的结果
// 语义是"这个位置能不能改名, 以及要改的那段文本在哪":
//   - Range           待改名标识符的区间 (UI 拿它做预选 + 高亮)
//   - Placeholder     服务器建议的初始文本; 缺省时 UI 自己从 Range 取
//   - DefaultBehavior 服务器说"按客户端默认规则判定" (没给 range 的那种形态)
type LSPPrepareRename struct {
	Range           LSPRange
	Placeholder     string
	DefaultBehavior bool
}

// PrepareRename 请求 textDocument/prepareRename, 在真正改名之前问一句"能改吗"
// 这是 IDE 里 F2 的第一步: 先拿到待改区间和建议名字, 弹输入框, 用户确认后才发
// textDocument/rename. 没有它的话, 光标落在不可改名的位置 (关键字/字面量/标准
// 库符号) 时只能等 rename 报错, 体验上是"输入了新名字, 然后报错".
//
// 响应有三种合法形态, 这里都吃:
//
//	{"start":...,"end":...}                  裸 Range
//	{"range":{...},"placeholder":"Foo"}      Range + 建议名 (gopls 走这个)
//	{"defaultBehavior":true}                 让客户端按默认规则处理
//
// null (不可改名) -> (nil, nil).
func (c *LSPClient) PrepareRename(uri string, line, character int) (*LSPPrepareRename, error) {
	raw, ok, err := c.requestOptional("textDocument/prepareRename", "renameProvider",
		textDocumentPositionParams(uri, line, character))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	// 一个结构体同时吃三种形态: 裸 Range 的 start/end 落在顶层, 包装形态落在
	// range 里, defaultBehavior 形态两者都没有.
	var pr struct {
		Range           *LSPRange    `json:"range"`
		Placeholder     string       `json:"placeholder"`
		DefaultBehavior bool         `json:"defaultBehavior"`
		Start           *LSPPosition `json:"start"`
		End             *LSPPosition `json:"end"`
	}
	if err := json.Unmarshal(raw, &pr); err != nil {
		return nil, fmt.Errorf("lspclient: decode prepareRename result: %w", err)
	}
	out := &LSPPrepareRename{Placeholder: pr.Placeholder, DefaultBehavior: pr.DefaultBehavior}
	switch {
	case pr.Range != nil:
		out.Range = *pr.Range
	case pr.Start != nil && pr.End != nil:
		out.Range = LSPRange{Start: *pr.Start, End: *pr.End}
	}
	return out, nil
}

// Implementation 请求 textDocument/implementation: 接口方法 -> 所有实现
// 跟 Definition 是一对: Definition 从用法跳到声明, Implementation 从接口
// (或接口方法) 跳到实现它的具体类型 (或方法). Go 项目里这是读代码的主力操作.
// 响应跟 definition 同样多态 (Location | []Location | []LocationLink), 一并归一.
func (c *LSPClient) Implementation(uri string, line, character int) ([]LSPLocation, error) {
	raw, ok, err := c.requestOptional("textDocument/implementation", "implementationProvider",
		textDocumentPositionParams(uri, line, character))
	if err != nil {
		return nil, err
	}
	if !ok {
		return []LSPLocation{}, nil
	}
	return decodeLocationsResult(raw, "implementation")
}

// decodeLocationsResult 把 Location | []Location | []LocationLink 归一成 []LSPLocation
func decodeLocationsResult(raw json.RawMessage, what string) ([]LSPLocation, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return []LSPLocation{}, nil
	}
	if trimmed[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("lspclient: decode %s result: %w", what, err)
		}
		out := make([]LSPLocation, 0, len(items))
		for _, item := range items {
			loc, err := decodeLocationOrLink(item, what)
			if err != nil {
				return nil, err
			}
			out = append(out, loc)
		}
		return out, nil
	}
	loc, err := decodeLocationOrLink(raw, what)
	if err != nil {
		return nil, err
	}
	return []LSPLocation{loc}, nil
}

// decodeLocationOrLink 解一个 Location 或 LocationLink
// LocationLink 把目标位置放在 targetUri/targetRange, 另有一个 targetSelectionRange
// 指向"符号名本身"; 跳转时用后者体验更好 (光标落在名字上而不是整个声明块开头).
func decodeLocationOrLink(raw json.RawMessage, what string) (LSPLocation, error) {
	var v struct {
		URI                  string    `json:"uri"`
		Range                LSPRange  `json:"range"`
		TargetURI            string    `json:"targetUri"`
		TargetRange          LSPRange  `json:"targetRange"`
		TargetSelectionRange *LSPRange `json:"targetSelectionRange"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return LSPLocation{}, fmt.Errorf("lspclient: decode %s location: %w", what, err)
	}
	if v.URI != "" {
		return LSPLocation{URI: v.URI, Range: v.Range}, nil
	}
	r := v.TargetRange
	if v.TargetSelectionRange != nil {
		r = *v.TargetSelectionRange
	}
	return LSPLocation{URI: v.TargetURI, Range: r}, nil
}

// LSPCallHierarchyItem 是调用树上的一个节点 (一个函数/方法)
// JSON tag 跟线协议一一对应, 因为它既要被解码, 也要被 *原样回传*:
// callHierarchy/incomingCalls 与 outgoingCalls 的 params 就是 {item: <这个对象>},
// 其中 Data 是服务器自己的私货 (gopls 拿它缓存位置信息), 必须一字不改地送回去,
// 所以留成 RawMessage.
type LSPCallHierarchyItem struct {
	Name           string          `json:"name"`
	Kind           int             `json:"kind"` // LSP SymbolKind (Function=12, Method=6, ...)
	Detail         string          `json:"detail,omitempty"`
	URI            string          `json:"uri"`
	Range          LSPRange        `json:"range"`
	SelectionRange LSPRange        `json:"selectionRange"`
	Data           json.RawMessage `json:"data,omitempty"`
}

// LSPCallHierarchyCall 是调用树上的一条边
// Item 是"另一端": incomingCalls 时它是调用方 (from), outgoingCalls 时它是被调方
// (to). Ranges 是调用点的位置 (规范里的 fromRanges), 用来在源码里逐个跳过去.
type LSPCallHierarchyCall struct {
	Item   LSPCallHierarchyItem
	Ranges []LSPRange
}

// CallHierarchyPrepare 请求 textDocument/prepareCallHierarchy
// 调用层次是两步协议: 先用光标位置 prepare 出根节点 (可能多个, 比如同名方法),
// 再拿根节点去问 incoming/outgoing. 这一步不给结果就没法展开调用树.
func (c *LSPClient) CallHierarchyPrepare(uri string, line, character int) ([]LSPCallHierarchyItem, error) {
	raw, ok, err := c.requestOptional("textDocument/prepareCallHierarchy", "callHierarchyProvider",
		textDocumentPositionParams(uri, line, character))
	if err != nil {
		return nil, err
	}
	if !ok {
		return []LSPCallHierarchyItem{}, nil
	}
	var items []LSPCallHierarchyItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("lspclient: decode prepareCallHierarchy result: %w", err)
	}
	return items, nil
}

// IncomingCalls 请求 callHierarchy/incomingCalls: 谁调用了这个函数
// item 必须是 CallHierarchyPrepare (或上一层 IncomingCalls) 返回的节点原物 --
// 它的 Data 字段要原样回传给服务器.
func (c *LSPClient) IncomingCalls(item LSPCallHierarchyItem) ([]LSPCallHierarchyCall, error) {
	raw, ok, err := c.requestOptional("callHierarchy/incomingCalls", "callHierarchyProvider",
		map[string]interface{}{"item": item})
	if err != nil {
		return nil, err
	}
	if !ok {
		return []LSPCallHierarchyCall{}, nil
	}
	return decodeCallHierarchyCalls(raw, true, "incomingCalls")
}

// OutgoingCalls 请求 callHierarchy/outgoingCalls: 这个函数调用了谁
func (c *LSPClient) OutgoingCalls(item LSPCallHierarchyItem) ([]LSPCallHierarchyCall, error) {
	raw, ok, err := c.requestOptional("callHierarchy/outgoingCalls", "callHierarchyProvider",
		map[string]interface{}{"item": item})
	if err != nil {
		return nil, err
	}
	if !ok {
		return []LSPCallHierarchyCall{}, nil
	}
	return decodeCallHierarchyCalls(raw, false, "outgoingCalls")
}

// decodeCallHierarchyCalls 解 incoming/outgoing 的调用边数组
// incoming 用 "from" 装另一端, outgoing 用 "to"; 除此之外两者形状一致, 所以只用
// 一个 bool 切换取哪个字段. 两个字段都缺的畸形元素跳过, 不让一条坏数据毁掉整棵树.
func decodeCallHierarchyCalls(raw json.RawMessage, incoming bool, what string) ([]LSPCallHierarchyCall, error) {
	var arr []struct {
		From       *LSPCallHierarchyItem `json:"from"`
		To         *LSPCallHierarchyItem `json:"to"`
		FromRanges []LSPRange            `json:"fromRanges"`
	}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("lspclient: decode %s result: %w", what, err)
	}
	out := make([]LSPCallHierarchyCall, 0, len(arr))
	for _, e := range arr {
		item := e.From
		if !incoming {
			item = e.To
		}
		if item == nil {
			continue
		}
		out = append(out, LSPCallHierarchyCall{Item: *item, Ranges: e.FromRanges})
	}
	return out, nil
}

// LSPTypeHierarchyItem 是类型层次上的一个节点 (一个类型)
// 跟 LSPCallHierarchyItem 同形 (含必须原样回传的 Data), 但语义不同: Go 里
// supertypes 是"这个类型实现的接口", subtypes 是"实现了这个接口的类型".
type LSPTypeHierarchyItem struct {
	Name           string          `json:"name"`
	Kind           int             `json:"kind"` // LSP SymbolKind (Struct=23, Interface=11, ...)
	Detail         string          `json:"detail,omitempty"`
	URI            string          `json:"uri"`
	Range          LSPRange        `json:"range"`
	SelectionRange LSPRange        `json:"selectionRange"`
	Data           json.RawMessage `json:"data,omitempty"`
}

// TypeHierarchyPrepare 请求 textDocument/prepareTypeHierarchy
// 跟调用层次一样是两步协议: 先 prepare 出根节点, 再 Supertypes / Subtypes 展开.
func (c *LSPClient) TypeHierarchyPrepare(uri string, line, character int) ([]LSPTypeHierarchyItem, error) {
	raw, ok, err := c.requestOptional("textDocument/prepareTypeHierarchy", "typeHierarchyProvider",
		textDocumentPositionParams(uri, line, character))
	if err != nil {
		return nil, err
	}
	if !ok {
		return []LSPTypeHierarchyItem{}, nil
	}
	return decodeTypeHierarchyItems(raw, "prepareTypeHierarchy")
}

// Supertypes 请求 typeHierarchy/supertypes: 这个类型的"上层" (Go: 它实现的接口)
func (c *LSPClient) Supertypes(item LSPTypeHierarchyItem) ([]LSPTypeHierarchyItem, error) {
	raw, ok, err := c.requestOptional("typeHierarchy/supertypes", "typeHierarchyProvider",
		map[string]interface{}{"item": item})
	if err != nil {
		return nil, err
	}
	if !ok {
		return []LSPTypeHierarchyItem{}, nil
	}
	return decodeTypeHierarchyItems(raw, "supertypes")
}

// Subtypes 请求 typeHierarchy/subtypes: 这个类型的"下层" (Go: 实现它的类型)
func (c *LSPClient) Subtypes(item LSPTypeHierarchyItem) ([]LSPTypeHierarchyItem, error) {
	raw, ok, err := c.requestOptional("typeHierarchy/subtypes", "typeHierarchyProvider",
		map[string]interface{}{"item": item})
	if err != nil {
		return nil, err
	}
	if !ok {
		return []LSPTypeHierarchyItem{}, nil
	}
	return decodeTypeHierarchyItems(raw, "subtypes")
}

// decodeTypeHierarchyItems 解 []TypeHierarchyItem
func decodeTypeHierarchyItems(raw json.RawMessage, what string) ([]LSPTypeHierarchyItem, error) {
	var items []LSPTypeHierarchyItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("lspclient: decode %s result: %w", what, err)
	}
	return items, nil
}

// LSPInlayHint 是一条内嵌提示 (编辑器里灰色的虚拟文本)
// Go 里两个典型用途: 显示 := 推导出来的类型, 以及在调用实参前显示形参名.
//   - Label        压平后的提示文本 (规范允许 string 或 []InlayHintLabelPart)
//   - Kind         1=Type, 2=Parameter (可选, 服务器可省略)
//   - PaddingLeft/PaddingRight 渲染时要不要留一个空格
//   - Tooltip      压平后的悬浮说明
//   - TextEdits    "接受这条提示"时要应用的编辑 (例如把推导类型真的写进代码)
type LSPInlayHint struct {
	Position     LSPPosition
	Label        string
	Kind         int
	PaddingLeft  bool
	PaddingRight bool
	Tooltip      string
	TextEdits    []LSPTextEdit
}

// InlayHint 请求 textDocument/inlayHint 拿一个区间内的内嵌提示
// params 是 {textDocument, range}: 只问可视区间, 别整篇文件都要 (大文件上
// 服务器要算很久). 响应是 []InlayHint, 其中 label/tooltip 都是多态字段, 压平.
//
// gopls 的每一类提示都是默认关闭的: provider 在, 但不给设置就回空数组. 要看到
// 提示得在 InitializationOptions / 配置里打开对应开关, 例如
// {"hints":{"parameterNames":true,"assignVariableTypes":true}}.
func (c *LSPClient) InlayHint(uri string, startLine, startChar, endLine, endChar int) ([]LSPInlayHint, error) {
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": uri},
		"range": LSPRange{
			Start: LSPPosition{Line: startLine, Character: startChar},
			End:   LSPPosition{Line: endLine, Character: endChar},
		},
	}
	raw, ok, err := c.requestOptional("textDocument/inlayHint", "inlayHintProvider", params)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []LSPInlayHint{}, nil
	}
	var arr []struct {
		Position     LSPPosition     `json:"position"`
		Label        json.RawMessage `json:"label"`
		Kind         int             `json:"kind"`
		Tooltip      json.RawMessage `json:"tooltip"`
		PaddingLeft  bool            `json:"paddingLeft"`
		PaddingRight bool            `json:"paddingRight"`
		TextEdits    []LSPTextEdit   `json:"textEdits"`
	}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("lspclient: decode inlayHint result: %w", err)
	}
	out := make([]LSPInlayHint, 0, len(arr))
	for _, h := range arr {
		hint := LSPInlayHint{
			Position:     h.Position,
			Label:        flattenInlayHintLabel(h.Label),
			Kind:         h.Kind,
			PaddingLeft:  h.PaddingLeft,
			PaddingRight: h.PaddingRight,
			TextEdits:    h.TextEdits,
		}
		if len(h.Tooltip) > 0 && string(h.Tooltip) != "null" {
			if tip, err := stringifyHoverContents(h.Tooltip); err == nil {
				hint.Tooltip = tip
			}
		}
		out = append(out, hint)
	}
	return out, nil
}

// flattenInlayHintLabel 把 inlayHint.label 压成一段文本
// 两种合法形态: 字符串, 或 []InlayHintLabelPart ({value, tooltip?, location?}).
// 分段形态直接首尾相接 —— 它们本来就是同一段提示被切开的 (切开只为让每段能挂
// 自己的 tooltip/跳转), 中间不该插分隔符. 形状不对时回空串, 不报错.
func flattenInlayHintLabel(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return ""
		}
		return s
	case '[':
		var parts []struct {
			Value string `json:"value"`
		}
		if err := json.Unmarshal(raw, &parts); err != nil {
			return ""
		}
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Value)
		}
		return b.String()
	}
	return ""
}

// LSPSemanticTokens 是 textDocument/semanticTokens/full 的结果
// Data 是规范定义的扁平 uint32 数组, 每 5 个一组描述一个 token:
//
//	deltaLine, deltaStartChar, length, tokenType, tokenModifiers
//
// 前两个是相对上一个 token 的增量 (所以不能单独看某一组). 用
// DecodeSemanticTokenData 展开成绝对坐标. ResultID 用来后续要增量 (Delta).
type LSPSemanticTokens struct {
	ResultID string
	Data     []uint32
}

// LSPSemanticTokensEdit 是一次增量: 把 Data 里 [Start, Start+DeleteCount) 换成 Data
type LSPSemanticTokensEdit struct {
	Start       int
	DeleteCount int
	Data        []uint32
}

// LSPSemanticTokensDelta 是 semanticTokens/full/delta 的结果
// 服务器有两种合法回法, Full 标出是哪种:
//   - Full=false: Edits 有效, 拿它去改上一次的 Data (省流量的正常路径)
//   - Full=true:  服务器放弃增量, 直接回了整份 tokens, Data 有效
type LSPSemanticTokensDelta struct {
	ResultID string
	Full     bool
	Data     []uint32
	Edits    []LSPSemanticTokensEdit
}

// LSPSemanticToken 是展开后的一个语义 token (绝对坐标)
// TokenType / Modifiers 是下标/位掩码, 对应服务器 capabilities 里
// semanticTokensProvider.legend 的 tokenTypes / tokenModifiers 两张表.
type LSPSemanticToken struct {
	Line      int
	Character int
	Length    int
	TokenType int
	Modifiers int
}

// DecodeSemanticTokenData 把规范的相对编码展开成绝对坐标的 token 列表
// 编码规则: 每 5 个 uint32 一组; deltaLine 是相对上一个 token 的行增量,
// deltaStartChar 在同一行时是相对上一个 token 起点的列增量, 换行时就是绝对列.
// 长度不足 5 的尾巴丢掉 (畸形数据, 不 panic).
func DecodeSemanticTokenData(data []uint32) []LSPSemanticToken {
	out := make([]LSPSemanticToken, 0, len(data)/5)
	line, char := 0, 0
	for i := 0; i+4 < len(data); i += 5 {
		deltaLine := int(data[i])
		deltaChar := int(data[i+1])
		if deltaLine > 0 {
			line += deltaLine
			char = deltaChar
		} else {
			char += deltaChar
		}
		out = append(out, LSPSemanticToken{
			Line:      line,
			Character: char,
			Length:    int(data[i+2]),
			TokenType: int(data[i+3]),
			Modifiers: int(data[i+4]),
		})
	}
	return out
}

// SemanticTokensFull 请求 textDocument/semanticTokens/full 拿整篇文件的语义着色
// 语义着色是"编辑器着色跟编译器一致"的唯一途径: 正则/词法着色分不出一个标识符
// 是类型、变量还是函数, 更分不出它是不是标准库的.
//
// 注意 gopls 默认不开: 实测 v0.22 只在设置里 semanticTokens 为 true 时才登记
// semanticTokensProvider, 所以宿主要在 LSPInitializeParams.InitializationOptions
// 里给 {"semanticTokens":true} (或在 SetConfigurationHandler 里回同样的设置).
// 没开时服务器 capabilities 里就没这一项, 本方法按能力缺失返回 (nil, nil).
func (c *LSPClient) SemanticTokensFull(uri string) (*LSPSemanticTokens, error) {
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": uri},
	}
	raw, ok, err := c.requestOptional("textDocument/semanticTokens/full", "semanticTokensProvider", params)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	var st struct {
		ResultID string   `json:"resultId"`
		Data     []uint32 `json:"data"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("lspclient: decode semanticTokens/full result: %w", err)
	}
	return &LSPSemanticTokens{ResultID: st.ResultID, Data: st.Data}, nil
}

// SemanticTokensFullDelta 请求 textDocument/semanticTokens/full/delta 拿增量着色
// previousResultID 是上一次 SemanticTokensFull / 本方法返回的 ResultID. 打字的时候
// 每次都拉整份 token 数组很浪费 (大文件几万个 uint32), 增量只回改动的那一段.
// 服务器可以随时决定"这次算不出增量", 直接回整份 —— 那时 Full=true, Data 有效.
// null / 服务器不支持 -> (nil, nil).
func (c *LSPClient) SemanticTokensFullDelta(uri, previousResultID string) (*LSPSemanticTokensDelta, error) {
	params := map[string]interface{}{
		"textDocument":     map[string]string{"uri": uri},
		"previousResultId": previousResultID,
	}
	raw, ok, err := c.requestOptional("textDocument/semanticTokens/full/delta", "semanticTokensProvider", params)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	var sd struct {
		ResultID string   `json:"resultId"`
		Data     []uint32 `json:"data"`
		Edits    []struct {
			Start       int      `json:"start"`
			DeleteCount int      `json:"deleteCount"`
			Data        []uint32 `json:"data"`
		} `json:"edits"`
	}
	if err := json.Unmarshal(raw, &sd); err != nil {
		return nil, fmt.Errorf("lspclient: decode semanticTokens/full/delta result: %w", err)
	}
	out := &LSPSemanticTokensDelta{ResultID: sd.ResultID}
	if sd.Edits == nil {
		// 没有 edits 字段 -> 服务器回的是完整 SemanticTokens 形态.
		out.Full = true
		out.Data = sd.Data
		return out, nil
	}
	out.Edits = make([]LSPSemanticTokensEdit, 0, len(sd.Edits))
	for _, e := range sd.Edits {
		out.Edits = append(out.Edits, LSPSemanticTokensEdit{
			Start:       e.Start,
			DeleteCount: e.DeleteCount,
			Data:        e.Data,
		})
	}
	return out, nil
}

// LSPCodeLens 是一条 code lens: 挂在某一行上的可点击动作
// Command 缺省 (nil) 表示这条 lens 是"未解析"的 —— 规范允许服务器先回位置,
// 等客户端发 codeLens/resolve 再补 command. 我们不宣称 resolveSupport, gopls
// 会直接把 command 给全; Data 是服务器私货, 留着以备将来接 resolve.
type LSPCodeLens struct {
	Range   LSPRange
	Command *LSPCommand
	Data    json.RawMessage
}

// CodeLens 请求 textDocument/codeLens 拿文件里的可点动作
// gopls 的 code lens 是"跑测试/跑基准/更新依赖/生成代码"这类入口: 它们挂在
// func TestXxx 那一行上, 点一下就走 workspace/executeCommand. 拿到 Command 后
// 直接喂 ExecuteCommand 即可.
// null / 服务器不支持 -> 空切片.
func (c *LSPClient) CodeLens(uri string) ([]LSPCodeLens, error) {
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": uri},
	}
	raw, ok, err := c.requestOptional("textDocument/codeLens", "codeLensProvider", params)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []LSPCodeLens{}, nil
	}
	var arr []struct {
		Range   LSPRange        `json:"range"`
		Command json.RawMessage `json:"command"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("lspclient: decode codeLens result: %w", err)
	}
	out := make([]LSPCodeLens, 0, len(arr))
	for _, l := range arr {
		out = append(out, LSPCodeLens{
			Range:   l.Range,
			Command: decodeCommandField(l.Command, "", nil),
			Data:    l.Data,
		})
	}
	return out, nil
}

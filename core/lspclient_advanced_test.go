package core

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// 这一组测试覆盖 lspclient.go 里"协议深水区"的部分, 全程不需要真的 gopls:
//   - server -> client 请求的回包 (workspace/configuration / applyEdit / folders)
//   - initialize 里 client capabilities + workspaceFolders 的实际写出形状
//   - CompletionItem 的 textEdit / additionalTextEdits / 多态 documentation
//   - WorkspaceEdit 的 documentChanges: 版本号 + create/rename/delete 资源操作
//   - prepareRename / implementation / call & type hierarchy / inlayHint /
//     semanticTokens / codeLens 的解码与"服务器没这能力"时的优雅降级
//
// 复用 lspclient_test.go 里的 in-process 设施 (newPipedClient / runFakeServer /
// fakeReply / memWriteCloser), 不引入新的假服务器实现.

// advReadFrame 把客户端写出的一段字节流解回一条 LSPMessage
// 客户端回给服务器的响应也是标准 LSP 帧, 直接复用 ReadLSPMessage 解.
func advReadFrame(t *testing.T, raw string) *LSPMessage {
	t.Helper()
	m, err := ReadLSPMessage(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("decode written frame: %v (raw=%q)", err, raw)
	}
	return m
}

// advOfflineClient 造一个"不跑读循环"的客户端: stdin 指向内存 buffer
// 用于只关心写出字节 / 只关心能力预判的测试.
func advOfflineClient() (*LSPClient, *memWriteCloser) {
	buf := &memWriteCloser{Buffer: &bytes.Buffer{}}
	return &LSPClient{stdin: buf, pending: map[int]chan *LSPMessage{}}, buf
}

// applyEditParams 是一份贴近 gopls 真实产出的 workspace/applyEdit 载荷:
// 一条带版本号的文本编辑 + create/rename/delete 三种资源操作 + 一条 version 为
// null 的文本编辑. (形状取自 gopls v0.22 执行 gopls.extract_to_new_file 时实际
// 发来的请求, 补上 rename/delete 两种操作.)
const applyEditParams = `{"label":"extract to new file","edit":{"documentChanges":[
{"textDocument":{"uri":"file:///a.go","version":7},"edits":[
  {"range":{"start":{"line":8,"character":0},"end":{"line":11,"character":0}},"newText":""}
]},
{"kind":"create","uri":"file:///compute.go","options":{"ignoreIfExists":true}},
{"kind":"rename","oldUri":"file:///compute.go","newUri":"file:///renamed.go","options":{"overwrite":true}},
{"kind":"delete","uri":"file:///old.go","options":{"recursive":true,"ignoreIfNotExists":true}},
{"textDocument":{"uri":"file:///a.go","version":null},"edits":[
  {"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"newText":"package main\n"}
]}
]}}`

// -----------------------------------------------------------------------------
// routeMessage: server -> client 请求必须产出一条回包
// -----------------------------------------------------------------------------

// 旧实现把服务器发来的请求直接丢弃, 于是 command 形态的重构 (organize imports /
// extract) 执行完什么都不会发生 -- 服务器等 applyEdit 的响应等不到.
// 这里喂 routeMessage 一条 workspace/applyEdit 请求, 断言:
//  1. 注册的 handler 被调到, 且拿到的是解好的 WorkspaceEdit (含资源操作);
//  2. 客户端真的往 stdin 写了一条 id 对得上的响应, applied=true.
func TestLSPAdvRouteMessage_ApplyEditRepliesApplied(t *testing.T) {
	c, buf := advOfflineClient()
	var gotLabel string
	var gotEdit *LSPWorkspaceEdit
	c.SetApplyEditHandler(func(label string, edit *LSPWorkspaceEdit) bool {
		gotLabel, gotEdit = label, edit
		return true
	})

	id := json.RawMessage(`42`)
	req := &LSPMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "workspace/applyEdit",
		Params:  json.RawMessage(applyEditParams),
	}
	pending := map[int]chan *LSPMessage{}
	notif := make(chan *LSPMessage, 1)
	routeMessage(req, pending, notif, c)

	if gotLabel != "extract to new file" {
		t.Errorf("handler label = %q", gotLabel)
	}
	if gotEdit == nil {
		t.Fatal("handler got nil edit")
	}
	if len(gotEdit.DocumentChanges) != 5 {
		t.Fatalf("documentChanges len = %d, want 5: %+v", len(gotEdit.DocumentChanges), gotEdit.DocumentChanges)
	}
	if kinds := advKinds(gotEdit.DocumentChanges); kinds != "edit,create,rename,delete,edit" {
		t.Errorf("documentChanges kinds = %q", kinds)
	}
	// 服务器请求不该串到 pending / notifications 里
	if len(notif) != 0 {
		t.Errorf("notif got %d msgs, want 0", len(notif))
	}

	reply := advReadFrame(t, buf.String())
	if reply.ID == nil || string(*reply.ID) != "42" {
		t.Fatalf("reply id = %v, want 42", reply.ID)
	}
	if reply.Error != nil {
		t.Fatalf("reply carried error: %+v", reply.Error)
	}
	var res struct {
		Applied bool `json:"applied"`
	}
	if err := json.Unmarshal(reply.Result, &res); err != nil {
		t.Fatalf("decode reply result: %v (%s)", err, string(reply.Result))
	}
	if !res.Applied {
		t.Errorf("applied = false, want true (%s)", string(reply.Result))
	}
}

// 有 responder 时, 普通响应仍然照常投递到 pending -- 新增的分支不能吃掉响应.
func TestLSPAdvRouteMessage_ResponseStillRoutesWithResponder(t *testing.T) {
	c, buf := advOfflineClient()
	ch := make(chan *LSPMessage, 1)
	pending := map[int]chan *LSPMessage{9: ch}
	id := json.RawMessage(`9`)
	resp := &LSPMessage{JSONRPC: "2.0", ID: &id, Result: json.RawMessage(`{"ok":true}`)}
	routeMessage(resp, pending, make(chan *LSPMessage, 1), c)
	select {
	case got := <-ch:
		if got != resp {
			t.Error("pending got a different message")
		}
	default:
		t.Fatal("response was not delivered to pending")
	}
	if buf.Len() != 0 {
		t.Errorf("responder wrote %d bytes for a plain response", buf.Len())
	}
}

// isServerRequest 是 readLoop 用来决定"走不走回包路径"的判据
func TestLSPAdvIsServerRequest(t *testing.T) {
	id := json.RawMessage(`1`)
	cases := []struct {
		name string
		m    *LSPMessage
		want bool
	}{
		{"nil", nil, false},
		{"server request", &LSPMessage{ID: &id, Method: "workspace/configuration"}, true},
		{"response with result", &LSPMessage{ID: &id, Result: json.RawMessage(`null`)}, false},
		{"response with error", &LSPMessage{ID: &id, Error: &LSPError{Code: -1}}, false},
		{"notification", &LSPMessage{Method: "$/progress"}, false},
		{"id only", &LSPMessage{ID: &id}, false},
	}
	for _, tc := range cases {
		if got := isServerRequest(tc.m); got != tc.want {
			t.Errorf("%s: isServerRequest = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// -----------------------------------------------------------------------------
// buildServerRequestReply: 每种 server -> client 请求的回包内容
// -----------------------------------------------------------------------------

func TestLSPAdvBuildServerRequestReply_Configuration(t *testing.T) {
	id := json.RawMessage(`3`)
	req := &LSPMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "workspace/configuration",
		Params:  json.RawMessage(`{"items":[{"scopeUri":"file:///w","section":"gopls"},{"section":"other"}]}`),
	}

	// 没注册 handler: 每项回 {}, 数量必须跟 items 对齐 (少一项服务器会错位取值).
	reply := buildServerRequestReply(req, serverRequestContext{})
	if reply == nil {
		t.Fatal("nil reply for workspace/configuration")
	}
	if got := string(reply.Result); got != `[{},{}]` {
		t.Errorf("default configuration result = %s, want [{},{}]", got)
	}

	// 注册 handler: 逐项回宿主给的值, scopeUri/section 要透传进去.
	var seen []string
	reply = buildServerRequestReply(req, serverRequestContext{
		Configuration: func(scopeURI, section string) json.RawMessage {
			seen = append(seen, scopeURI+"|"+section)
			if section == "gopls" {
				return json.RawMessage(`{"semanticTokens":true}`)
			}
			return nil // nil -> 补成 {}
		},
	})
	if len(seen) != 2 || seen[0] != "file:///w|gopls" || seen[1] != "|other" {
		t.Errorf("handler saw %v", seen)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(reply.Result, &items); err != nil {
		t.Fatalf("decode configuration result: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("configuration result len = %d, want 2: %s", len(items), string(reply.Result))
	}
	if string(items[0]) != `{"semanticTokens":true}` || string(items[1]) != `{}` {
		t.Errorf("configuration items = %s", string(reply.Result))
	}
}

func TestLSPAdvBuildServerRequestReply_ConfigurationNoItems(t *testing.T) {
	id := json.RawMessage(`3`)
	for _, params := range []string{``, `{}`, `{"items":[]}`, `not json`} {
		req := &LSPMessage{ID: &id, Method: "workspace/configuration", Params: json.RawMessage(params)}
		reply := buildServerRequestReply(req, serverRequestContext{})
		if reply == nil || string(reply.Result) != `[]` {
			t.Errorf("params %q -> result %v, want []", params, reply)
		}
	}
}

func TestLSPAdvBuildServerRequestReply_ApplyEditWithoutHandler(t *testing.T) {
	id := json.RawMessage(`8`)
	req := &LSPMessage{ID: &id, Method: "workspace/applyEdit", Params: json.RawMessage(applyEditParams)}
	reply := buildServerRequestReply(req, serverRequestContext{})
	if reply == nil {
		t.Fatal("nil reply")
	}
	var res struct {
		Applied       bool   `json:"applied"`
		FailureReason string `json:"failureReason"`
	}
	if err := json.Unmarshal(reply.Result, &res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	// 关键点是"仍然回了包": 服务器不会悬着; 但要如实报告没有应用.
	if res.Applied {
		t.Error("applied = true without a handler")
	}
	if res.FailureReason == "" {
		t.Error("failureReason empty; server cannot tell why the edit was refused")
	}
}

func TestLSPAdvBuildServerRequestReply_ApplyEditHandlerRefuses(t *testing.T) {
	id := json.RawMessage(`8`)
	req := &LSPMessage{ID: &id, Method: "workspace/applyEdit", Params: json.RawMessage(applyEditParams)}
	called := false
	reply := buildServerRequestReply(req, serverRequestContext{
		ApplyEdit: func(string, *LSPWorkspaceEdit) bool { called = true; return false },
	})
	if !called {
		t.Fatal("handler not called")
	}
	if got := string(reply.Result); got != `{"applied":false}` {
		t.Errorf("result = %s, want {\"applied\":false}", got)
	}
}

func TestLSPAdvBuildServerRequestReply_WorkspaceFolders(t *testing.T) {
	id := json.RawMessage(`11`)
	req := &LSPMessage{ID: &id, Method: "workspace/workspaceFolders"}

	// 没有 folder: 回 null (规范里的"没有工作区"), 不是空数组.
	if got := string(buildServerRequestReply(req, serverRequestContext{}).Result); got != "null" {
		t.Errorf("empty folders result = %s, want null", got)
	}

	reply := buildServerRequestReply(req, serverRequestContext{
		Folders: []LSPWorkspaceFolder{{URI: "file:///w", Name: "w"}},
	})
	var folders []LSPWorkspaceFolder
	if err := json.Unmarshal(reply.Result, &folders); err != nil {
		t.Fatalf("decode folders: %v", err)
	}
	if len(folders) != 1 || folders[0].URI != "file:///w" || folders[0].Name != "w" {
		t.Errorf("folders = %+v", folders)
	}
}

func TestLSPAdvBuildServerRequestReply_AckAndUnknown(t *testing.T) {
	id := json.RawMessage(`12`)
	// 知会型请求: 回 null 就够, 但必须回 -- 尤其 client/registerCapability,
	// 不回 gopls 会一直等.
	for _, method := range []string{
		"client/registerCapability", "client/unregisterCapability",
		"window/workDoneProgress/create", "window/showMessageRequest",
		"workspace/semanticTokens/refresh", "workspace/inlayHint/refresh",
		"workspace/codeLens/refresh", "workspace/diagnostic/refresh",
	} {
		reply := buildServerRequestReply(&LSPMessage{ID: &id, Method: method}, serverRequestContext{})
		if reply == nil {
			t.Fatalf("%s: nil reply", method)
		}
		if reply.Error != nil || string(reply.Result) != "null" {
			t.Errorf("%s: reply = %+v, want null result", method, reply)
		}
	}

	// 未知请求: 回 MethodNotFound. 同样是"回了", 服务器不会悬着.
	reply := buildServerRequestReply(&LSPMessage{ID: &id, Method: "some/futureRequest"}, serverRequestContext{})
	if reply == nil || reply.Error == nil {
		t.Fatalf("unknown request reply = %+v, want an error reply", reply)
	}
	if reply.Error.Code != lspMethodNotFound {
		t.Errorf("error code = %d, want %d", reply.Error.Code, lspMethodNotFound)
	}

	// 通知 (没有 ID) 不需要回包.
	if got := buildServerRequestReply(&LSPMessage{Method: "$/progress"}, serverRequestContext{}); got != nil {
		t.Errorf("notification produced a reply: %+v", got)
	}
	if got := buildServerRequestReply(nil, serverRequestContext{}); got != nil {
		t.Errorf("nil message produced a reply: %+v", got)
	}
}

// -----------------------------------------------------------------------------
// initialize: client capabilities + workspaceFolders 真的写进了 params
// -----------------------------------------------------------------------------

func TestLSPAdvInitialize_AdvertisesCapabilitiesAndFolders(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var captured json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(method string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		if method != "initialize" {
			t.Errorf("method = %q", method)
		}
		captured = append(captured[:0:0], params...)
		return &fakeReply{Result: json.RawMessage(`{"capabilities":{"renameProvider":{"prepareProvider":true},"callHierarchyProvider":true},"serverInfo":{"name":"fake"}}`)}
	})

	if _, err := c.Initialize(LSPInitializeParams{ProcessID: 4321, RootURI: "file:///home/me/proj"}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	var got struct {
		ProcessID    int    `json:"processId"`
		RootURI      string `json:"rootUri"`
		Capabilities struct {
			Workspace struct {
				ApplyEdit     bool `json:"applyEdit"`
				Configuration bool `json:"configuration"`
				WorkspaceEdit struct {
					DocumentChanges    bool     `json:"documentChanges"`
					ResourceOperations []string `json:"resourceOperations"`
				} `json:"workspaceEdit"`
			} `json:"workspace"`
			TextDocument struct {
				CodeAction struct {
					Literal *struct {
						Kind struct {
							ValueSet []string `json:"valueSet"`
						} `json:"codeActionKind"`
					} `json:"codeActionLiteralSupport"`
				} `json:"codeAction"`
				Rename struct {
					PrepareSupport bool `json:"prepareSupport"`
				} `json:"rename"`
				SemanticTokens struct {
					TokenTypes []string `json:"tokenTypes"`
					Requests   struct {
						Full struct {
							Delta bool `json:"delta"`
						} `json:"full"`
					} `json:"requests"`
				} `json:"semanticTokens"`
				CallHierarchy *json.RawMessage `json:"callHierarchy"`
				TypeHierarchy *json.RawMessage `json:"typeHierarchy"`
				InlayHint     *json.RawMessage `json:"inlayHint"`
				Completion    struct {
					Item struct {
						SnippetSupport bool `json:"snippetSupport"`
					} `json:"completionItem"`
				} `json:"completion"`
			} `json:"textDocument"`
		} `json:"capabilities"`
		WorkspaceFolders []LSPWorkspaceFolder `json:"workspaceFolders"`
	}
	if err := json.Unmarshal(captured, &got); err != nil {
		t.Fatalf("decode initialize params: %v (%s)", err, string(captured))
	}
	if got.ProcessID != 4321 || got.RootURI != "file:///home/me/proj" {
		t.Errorf("processId/rootUri = %d/%q", got.ProcessID, got.RootURI)
	}
	ws := got.Capabilities.Workspace
	if !ws.ApplyEdit || !ws.Configuration {
		t.Errorf("workspace.applyEdit=%v configuration=%v, both must be advertised", ws.ApplyEdit, ws.Configuration)
	}
	if !ws.WorkspaceEdit.DocumentChanges || len(ws.WorkspaceEdit.ResourceOperations) != 3 {
		t.Errorf("workspaceEdit = %+v", ws.WorkspaceEdit)
	}
	td := got.Capabilities.TextDocument
	// codeActionLiteralSupport 是 organize-imports 能拿到内联 edit 的前提
	if td.CodeAction.Literal == nil || len(td.CodeAction.Literal.Kind.ValueSet) == 0 {
		t.Error("codeAction.codeActionLiteralSupport missing; server would only return bare Commands")
	}
	if !td.Rename.PrepareSupport {
		t.Error("rename.prepareSupport not advertised")
	}
	if len(td.SemanticTokens.TokenTypes) == 0 || !td.SemanticTokens.Requests.Full.Delta {
		t.Errorf("semanticTokens capability = %+v", td.SemanticTokens)
	}
	if td.CallHierarchy == nil || td.TypeHierarchy == nil || td.InlayHint == nil {
		t.Error("callHierarchy/typeHierarchy/inlayHint capabilities missing")
	}
	if !td.Completion.Item.SnippetSupport {
		t.Error("completionItem.snippetSupport not advertised")
	}
	// workspaceFolders 由 rootUri 推出来, 名字取最后一段
	if len(got.WorkspaceFolders) != 1 || got.WorkspaceFolders[0].URI != "file:///home/me/proj" ||
		got.WorkspaceFolders[0].Name != "proj" {
		t.Errorf("workspaceFolders = %+v", got.WorkspaceFolders)
	}

	// initialize 响应里的 capabilities 被记下来了
	caps := c.ServerCapabilities()
	if len(caps) == 0 || !bytes.Contains(caps, []byte("callHierarchyProvider")) {
		t.Errorf("ServerCapabilities() = %s", string(caps))
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// 调用方自己给了 capabilities 时不能被默认值覆盖.
func TestLSPAdvInitialize_KeepsCallerCapabilities(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	var captured json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		captured = append(captured[:0:0], params...)
		return &fakeReply{Result: json.RawMessage(`{"capabilities":{}}`)}
	})

	if _, err := c.Initialize(LSPInitializeParams{
		RootURI:          "file:///w",
		Capabilities:     json.RawMessage(`{"custom":true}`),
		WorkspaceFolders: []LSPWorkspaceFolder{{URI: "file:///w/a", Name: "a"}, {URI: "file:///w/b", Name: "b"}},
	}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	var got struct {
		Capabilities     map[string]bool      `json:"capabilities"`
		WorkspaceFolders []LSPWorkspaceFolder `json:"workspaceFolders"`
	}
	if err := json.Unmarshal(captured, &got); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if len(got.Capabilities) != 1 || !got.Capabilities["custom"] {
		t.Errorf("capabilities = %+v, want the caller's own blob", got.Capabilities)
	}
	if len(got.WorkspaceFolders) != 2 {
		t.Errorf("workspaceFolders = %+v, want the caller's 2 folders", got.WorkspaceFolders)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

func TestLSPAdvDefaultClientCapabilities_ValidJSONAndCopied(t *testing.T) {
	caps := DefaultClientCapabilities()
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(caps, &probe); err != nil {
		t.Fatalf("default capabilities are not valid JSON: %v", err)
	}
	for _, k := range []string{"workspace", "textDocument", "window", "general"} {
		if _, ok := probe[k]; !ok {
			t.Errorf("default capabilities missing %q", k)
		}
	}
	// 返回的是拷贝: 改一份不影响下一次
	caps[0] = 'x'
	if DefaultClientCapabilities()[0] != '{' {
		t.Error("DefaultClientCapabilities returned a shared buffer")
	}
}

func TestLSPAdvWorkspaceFolderName(t *testing.T) {
	cases := map[string]string{
		"file:///home/me/proj":  "proj",
		"file:///home/me/proj/": "proj",
		"file:///":              "file:",
		"":                      "",
	}
	for in, want := range cases {
		if got := workspaceFolderName(in); got != want {
			t.Errorf("workspaceFolderName(%q) = %q, want %q", in, got, want)
		}
	}
}

// -----------------------------------------------------------------------------
// capabilities 预判: 键存在且不是 false/null 才算支持; 未握手时不预判
// -----------------------------------------------------------------------------

func TestLSPAdvCapabilitySupported(t *testing.T) {
	const caps = `{"implementationProvider":true,"inlayHintProvider":{},"semanticTokensProvider":{"legend":{}},
"codeLensProvider":false,"typeHierarchyProvider":null}`
	cases := []struct {
		caps     string
		provider string
		want     bool
	}{
		{caps, "implementationProvider", true},
		{caps, "inlayHintProvider", true},
		{caps, "semanticTokensProvider", true},
		{caps, "codeLensProvider", false},
		{caps, "typeHierarchyProvider", false},
		{caps, "callHierarchyProvider", false}, // 键都没有 -> 不支持
		{``, "anything", true},                 // 没握手 -> 不预判
		{`{}`, "anything", true},               // 服务器给了空 capabilities -> 不预判
		{`not json`, "anything", true},         // 解不动 -> 不预判
	}
	for _, tc := range cases {
		if got := capabilitySupported(json.RawMessage(tc.caps), tc.provider); got != tc.want {
			t.Errorf("capabilitySupported(%.20q, %q) = %v, want %v", tc.caps, tc.provider, got, tc.want)
		}
	}
}

func TestLSPAdvUnsupportedMethod(t *testing.T) {
	if unsupportedMethod(nil) {
		t.Error("nil resp reported as unsupported")
	}
	if unsupportedMethod(&LSPMessage{}) {
		t.Error("resp without error reported as unsupported")
	}
	if unsupportedMethod(&LSPMessage{Error: &LSPError{Code: -32603, Message: "internal"}}) {
		t.Error("internal error reported as unsupported")
	}
	if !unsupportedMethod(&LSPMessage{Error: &LSPError{Code: lspMethodNotFound}}) {
		t.Error("MethodNotFound not reported as unsupported")
	}
}

// 能力预判为 false 时一个字节都不发: 省一次往返, 也不会在 UI 上弹错.
func TestLSPAdvSkipsRequestWhenCapabilityAbsent(t *testing.T) {
	c, buf := advOfflineClient()
	c.serverCaps = json.RawMessage(`{"renameProvider":true}`) // 只有 rename, 其余全缺

	if got, err := c.Implementation("file:///a.go", 1, 1); err != nil || len(got) != 0 {
		t.Errorf("Implementation = %v, %v; want empty, nil", got, err)
	}
	if got, err := c.CallHierarchyPrepare("file:///a.go", 1, 1); err != nil || len(got) != 0 {
		t.Errorf("CallHierarchyPrepare = %v, %v", got, err)
	}
	if got, err := c.TypeHierarchyPrepare("file:///a.go", 1, 1); err != nil || len(got) != 0 {
		t.Errorf("TypeHierarchyPrepare = %v, %v", got, err)
	}
	if got, err := c.InlayHint("file:///a.go", 0, 0, 5, 0); err != nil || len(got) != 0 {
		t.Errorf("InlayHint = %v, %v", got, err)
	}
	if got, err := c.SemanticTokensFull("file:///a.go"); err != nil || got != nil {
		t.Errorf("SemanticTokensFull = %v, %v", got, err)
	}
	if got, err := c.SemanticTokensFullDelta("file:///a.go", "1"); err != nil || got != nil {
		t.Errorf("SemanticTokensFullDelta = %v, %v", got, err)
	}
	if got, err := c.CodeLens("file:///a.go"); err != nil || len(got) != 0 {
		t.Errorf("CodeLens = %v, %v", got, err)
	}
	if buf.Len() != 0 {
		t.Errorf("sent %d bytes for unsupported capabilities: %q", buf.Len(), buf.String())
	}
}

// -----------------------------------------------------------------------------
// CompletionItem: textEdit / additionalTextEdits / insertTextFormat /
// sortText / filterText / 多态 documentation
// -----------------------------------------------------------------------------

// 载荷取自 gopls v0.22 的 unimported completion: 选中 fmt.Fprintln 时既给
// snippet 形态的 textEdit, 又给一条把 import "fmt" 插到文件头的 additionalTextEdit.
const completionItemJSON = `{
"label":"Fprintln","detail":"func(w io.Writer, a ...any) (n int, err error)","kind":3,
"insertText":"Fprintln","insertTextFormat":2,"sortText":"00000","filterText":"Fprintln",
"documentation":{"kind":"markdown","value":"Fprintln formats using the default formats"},
"textEdit":{"range":{"start":{"line":3,"character":5},"end":{"line":3,"character":9}},"newText":"Fprintln(${1:})"},
"additionalTextEdits":[
 {"range":{"start":{"line":1,"character":0},"end":{"line":1,"character":0}},"newText":"import \"fmt\"\n"}
]}`

func TestLSPAdvCompletionItem_DecodesEditsAndMetadata(t *testing.T) {
	var it LSPCompletionItem
	if err := json.Unmarshal([]byte(completionItemJSON), &it); err != nil {
		t.Fatalf("unmarshal completion item: %v", err)
	}
	if it.Label != "Fprintln" || it.Kind != 3 || it.InsertText != "Fprintln" {
		t.Errorf("basic fields = %+v", it)
	}
	if it.InsertTextFormat != 2 {
		t.Errorf("insertTextFormat = %d, want 2 (snippet)", it.InsertTextFormat)
	}
	if it.SortText != "00000" || it.FilterText != "Fprintln" {
		t.Errorf("sortText/filterText = %q/%q", it.SortText, it.FilterText)
	}
	if it.Documentation != "Fprintln formats using the default formats" {
		t.Errorf("documentation = %q", it.Documentation)
	}
	if it.TextEdit == nil {
		t.Fatal("textEdit dropped")
	}
	if it.TextEdit.NewText != "Fprintln(${1:})" ||
		it.TextEdit.Range.Start.Character != 5 || it.TextEdit.Range.End.Character != 9 {
		t.Errorf("textEdit = %+v", *it.TextEdit)
	}
	if len(it.AdditionalTextEdits) != 1 || it.AdditionalTextEdits[0].NewText != "import \"fmt\"\n" ||
		it.AdditionalTextEdits[0].Range.Start.Line != 1 {
		t.Errorf("additionalTextEdits = %+v", it.AdditionalTextEdits)
	}
}

func TestLSPAdvCompletionItem_DocumentationAndTextEditShapes(t *testing.T) {
	// documentation 的字符串形态
	var it LSPCompletionItem
	if err := json.Unmarshal([]byte(`{"label":"A","documentation":"plain doc"}`), &it); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if it.Documentation != "plain doc" {
		t.Errorf("string documentation = %q", it.Documentation)
	}
	// documentation 形状怪异时不该毁掉整条 item
	it = LSPCompletionItem{}
	if err := json.Unmarshal([]byte(`{"label":"B","documentation":123}`), &it); err != nil {
		t.Fatalf("numeric documentation broke the item: %v", err)
	}
	if it.Label != "B" || it.Documentation != "" {
		t.Errorf("item = %+v", it)
	}
	// InsertReplaceEdit 形态: 没有 range, 取 replace (接受补全时覆盖已有标识符)
	it = LSPCompletionItem{}
	if err := json.Unmarshal([]byte(`{"label":"C","textEdit":{
"insert":{"start":{"line":1,"character":4},"end":{"line":1,"character":4}},
"replace":{"start":{"line":1,"character":2},"end":{"line":1,"character":6}},
"newText":"Cat"}}`), &it); err != nil {
		t.Fatalf("unmarshal InsertReplaceEdit: %v", err)
	}
	if it.TextEdit == nil || it.TextEdit.Range.Start.Character != 2 || it.TextEdit.Range.End.Character != 6 {
		t.Errorf("InsertReplaceEdit -> %+v, want the replace range", it.TextEdit)
	}
	// 缺 range/insert/replace 的 textEdit 不是合法编辑 -> nil, 上层退回 InsertText
	it = LSPCompletionItem{}
	if err := json.Unmarshal([]byte(`{"label":"D","textEdit":{"newText":"x"}}`), &it); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if it.TextEdit != nil {
		t.Errorf("rangeless textEdit = %+v, want nil", it.TextEdit)
	}
	// textEdit 缺省 / null
	it = LSPCompletionItem{}
	if err := json.Unmarshal([]byte(`{"label":"E","textEdit":null,"additionalTextEdits":null}`), &it); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if it.TextEdit != nil || it.AdditionalTextEdits != nil {
		t.Errorf("null edits = %+v / %+v", it.TextEdit, it.AdditionalTextEdits)
	}
}

// 端到端: 新字段经 Completion() 的两种响应形态都能出来
func TestLSPAdvCompletion_CarriesNewFieldsThroughRPC(t *testing.T) {
	for _, shape := range []struct {
		name   string
		result string
	}{
		{"CompletionList", `{"isIncomplete":true,"items":[` + completionItemJSON + `]}`},
		{"RawArray", `[` + completionItemJSON + `]`},
	} {
		t.Run(shape.name, func(t *testing.T) {
			c, srvIn, srvOut := newPipedClient(t)
			defer func() { _ = c.Close() }()
			done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
				return &fakeReply{Result: json.RawMessage(shape.result)}
			})
			items, err := c.Completion("file:///a.go", 3, 9)
			if err != nil {
				t.Fatalf("Completion: %v", err)
			}
			if len(items) != 1 {
				t.Fatalf("items = %+v", items)
			}
			if items[0].TextEdit == nil || len(items[0].AdditionalTextEdits) != 1 ||
				items[0].InsertTextFormat != 2 || items[0].Documentation == "" {
				t.Errorf("item lost fields: %+v", items[0])
			}
			_ = srvIn.Close()
			_ = srvOut.Close()
			<-done
		})
	}
}

// -----------------------------------------------------------------------------
// WorkspaceEdit: documentChanges 的版本号 + create/rename/delete 资源操作
// -----------------------------------------------------------------------------

// advKinds 把 documentChanges 的 Kind 串起来, 方便断言顺序
func advKinds(changes []LSPDocumentChange) string {
	parts := make([]string, 0, len(changes))
	for _, c := range changes {
		parts = append(parts, c.Kind)
	}
	return strings.Join(parts, ",")
}

func TestLSPAdvDecodeWorkspaceEdit_DocumentChanges(t *testing.T) {
	var params struct {
		Edit json.RawMessage `json:"edit"`
	}
	if err := json.Unmarshal([]byte(applyEditParams), &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	we, err := decodeWorkspaceEdit(params.Edit, "test")
	if err != nil {
		t.Fatalf("decodeWorkspaceEdit: %v", err)
	}
	if got := advKinds(we.DocumentChanges); got != "edit,create,rename,delete,edit" {
		t.Fatalf("kinds = %q", got)
	}

	// 1) 带版本号的文本编辑
	e0 := we.DocumentChanges[0]
	if e0.URI != "file:///a.go" || !e0.Versioned || e0.Version != 7 || len(e0.Edits) != 1 {
		t.Errorf("documentChanges[0] = %+v", e0)
	}
	// 2) create + options
	cr := we.DocumentChanges[1]
	if cr.URI != "file:///compute.go" || !cr.IgnoreIfExists || cr.Overwrite {
		t.Errorf("create = %+v", cr)
	}
	// 3) rename: URI 是 oldUri, NewURI 是 newUri
	rn := we.DocumentChanges[2]
	if rn.URI != "file:///compute.go" || rn.NewURI != "file:///renamed.go" || !rn.Overwrite {
		t.Errorf("rename = %+v", rn)
	}
	// 4) delete + options
	dl := we.DocumentChanges[3]
	if dl.URI != "file:///old.go" || !dl.Recursive || !dl.IgnoreIfNotExists {
		t.Errorf("delete = %+v", dl)
	}
	// 5) version 为 null: 有 uri 但版本未知, 不能报成"版本 0"
	e4 := we.DocumentChanges[4]
	if e4.URI != "file:///a.go" || e4.Versioned || e4.Version != 0 {
		t.Errorf("documentChanges[4] = %+v", e4)
	}

	// Changes 仍然是压平后的 uri -> edits (老调用方照旧); 两条 a.go 的编辑合并,
	// 资源操作不会伪装成文本编辑塞进来.
	if len(we.Changes) != 1 {
		t.Fatalf("changes = %+v, want only a.go", we.Changes)
	}
	if got := we.Changes["file:///a.go"]; len(got) != 2 {
		t.Errorf("a.go edits = %+v, want 2 folded edits", got)
	}
}

func TestLSPAdvDecodeWorkspaceEdit_ChangesOnlyAndBoth(t *testing.T) {
	// 只有 changes: DocumentChanges 为空, Changes 照旧
	we, err := decodeWorkspaceEdit(json.RawMessage(`{"changes":{"file:///a.go":[
{"range":{"start":{"line":1,"character":0},"end":{"line":1,"character":3}},"newText":"Bar"}]}}`), "test")
	if err != nil {
		t.Fatalf("decodeWorkspaceEdit: %v", err)
	}
	if len(we.DocumentChanges) != 0 {
		t.Errorf("DocumentChanges = %+v, want empty for changes-only form", we.DocumentChanges)
	}
	if len(we.Changes["file:///a.go"]) != 1 {
		t.Errorf("Changes = %+v", we.Changes)
	}

	// 两种都给: Changes 以简单形态为准 (旧行为), 但版本化信息也要留下来
	we, err = decodeWorkspaceEdit(json.RawMessage(`{
"changes":{"file:///a.go":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}},"newText":"x"}]},
"documentChanges":[{"textDocument":{"uri":"file:///b.go","version":2},"edits":[
{"range":{"start":{"line":5,"character":0},"end":{"line":5,"character":1}},"newText":"y"}]}]}`), "test")
	if err != nil {
		t.Fatalf("decodeWorkspaceEdit: %v", err)
	}
	if len(we.Changes) != 1 || len(we.Changes["file:///a.go"]) != 1 {
		t.Errorf("Changes = %+v, want the changes map to win", we.Changes)
	}
	if len(we.DocumentChanges) != 1 || we.DocumentChanges[0].URI != "file:///b.go" ||
		!we.DocumentChanges[0].Versioned || we.DocumentChanges[0].Version != 2 {
		t.Errorf("DocumentChanges = %+v", we.DocumentChanges)
	}
}

func TestLSPAdvDecodeWorkspaceEdit_Malformed(t *testing.T) {
	if _, err := decodeWorkspaceEdit(json.RawMessage(`{"documentChanges":[3]}`), "rename"); err == nil {
		t.Error("expected an error for a non-object documentChanges entry")
	}
	// 空对象是合法的 (什么都不改)
	we, err := decodeWorkspaceEdit(json.RawMessage(`{}`), "rename")
	if err != nil || we == nil || len(we.Changes) != 0 || len(we.DocumentChanges) != 0 {
		t.Errorf("empty edit -> %+v, %v", we, err)
	}
}

// Rename 走的是同一个解码器: 资源操作要能透到调用方
func TestLSPAdvRename_KeepsResourceOperations(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		// 改包名时 gopls 会连带把文件改名: 一条 edit + 一条 rename
		return &fakeReply{Result: json.RawMessage(`{"documentChanges":[
{"textDocument":{"uri":"file:///old.go","version":4},"edits":[
 {"range":{"start":{"line":0,"character":8},"end":{"line":0,"character":11}},"newText":"new"}]},
{"kind":"rename","oldUri":"file:///old.go","newUri":"file:///new.go"}]}`)}
	})

	we, err := c.Rename("file:///old.go", 0, 8, "new")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if we == nil {
		t.Fatal("nil workspace edit")
	}
	if advKinds(we.DocumentChanges) != "edit,rename" {
		t.Fatalf("kinds = %q", advKinds(we.DocumentChanges))
	}
	if we.DocumentChanges[0].Version != 4 || !we.DocumentChanges[0].Versioned {
		t.Errorf("version lost: %+v", we.DocumentChanges[0])
	}
	if we.DocumentChanges[1].NewURI != "file:///new.go" {
		t.Errorf("rename target lost: %+v", we.DocumentChanges[1])
	}
	// 老字段照旧
	if len(we.Changes["file:///old.go"]) != 1 {
		t.Errorf("Changes = %+v", we.Changes)
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// -----------------------------------------------------------------------------
// 进阶方法的解码
// -----------------------------------------------------------------------------

// advRPC 用一个只回一条固定 result 的假服务器跑一次调用, 并把请求 params 抓出来
func advRPC(t *testing.T, result string, call func(c *LSPClient)) json.RawMessage {
	t.Helper()
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()
	var captured json.RawMessage
	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, params json.RawMessage) *fakeReply {
		captured = append(captured[:0:0], params...)
		return &fakeReply{Result: json.RawMessage(result)}
	})
	call(c)
	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
	return captured
}

func TestLSPAdvPrepareRename_Shapes(t *testing.T) {
	// gopls 的形态: {range, placeholder}
	advRPC(t, `{"range":{"start":{"line":8,"character":5},"end":{"line":8,"character":12}},"placeholder":"compute"}`,
		func(c *LSPClient) {
			pr, err := c.PrepareRename("file:///a.go", 8, 5)
			if err != nil {
				t.Fatalf("PrepareRename: %v", err)
			}
			if pr == nil {
				t.Fatal("nil result")
			}
			if pr.Placeholder != "compute" || pr.Range.Start.Character != 5 || pr.Range.End.Character != 12 {
				t.Errorf("prepareRename = %+v", pr)
			}
		})
	// 裸 Range 形态
	advRPC(t, `{"start":{"line":2,"character":1},"end":{"line":2,"character":4}}`, func(c *LSPClient) {
		pr, err := c.PrepareRename("file:///a.go", 2, 1)
		if err != nil || pr == nil {
			t.Fatalf("PrepareRename = %+v, %v", pr, err)
		}
		if pr.Range.Start.Line != 2 || pr.Range.End.Character != 4 || pr.Placeholder != "" {
			t.Errorf("bare range shape = %+v", pr)
		}
	})
	// defaultBehavior 形态
	advRPC(t, `{"defaultBehavior":true}`, func(c *LSPClient) {
		pr, err := c.PrepareRename("file:///a.go", 2, 1)
		if err != nil || pr == nil {
			t.Fatalf("PrepareRename = %+v, %v", pr, err)
		}
		if !pr.DefaultBehavior {
			t.Errorf("defaultBehavior shape = %+v", pr)
		}
	})
	// null: 这个位置改不了名
	advRPC(t, `null`, func(c *LSPClient) {
		pr, err := c.PrepareRename("file:///a.go", 0, 0)
		if err != nil || pr != nil {
			t.Errorf("null -> %+v, %v; want nil, nil", pr, err)
		}
	})
}

func TestLSPAdvImplementation_Shapes(t *testing.T) {
	// []Location
	advRPC(t, `[{"uri":"file:///impl.go","range":{"start":{"line":3,"character":6},"end":{"line":3,"character":9}}}]`,
		func(c *LSPClient) {
			locs, err := c.Implementation("file:///a.go", 1, 1)
			if err != nil {
				t.Fatalf("Implementation: %v", err)
			}
			if len(locs) != 1 || locs[0].URI != "file:///impl.go" || locs[0].Range.Start.Line != 3 {
				t.Errorf("locs = %+v", locs)
			}
		})
	// 单个 Location
	advRPC(t, `{"uri":"file:///impl.go","range":{"start":{"line":9,"character":0},"end":{"line":9,"character":2}}}`,
		func(c *LSPClient) {
			locs, err := c.Implementation("file:///a.go", 1, 1)
			if err != nil || len(locs) != 1 || locs[0].Range.Start.Line != 9 {
				t.Errorf("single location -> %+v, %v", locs, err)
			}
		})
	// LocationLink: 用 targetSelectionRange (光标落在符号名上)
	advRPC(t, `[{"targetUri":"file:///impl.go",
"targetRange":{"start":{"line":10,"character":0},"end":{"line":14,"character":1}},
"targetSelectionRange":{"start":{"line":10,"character":6},"end":{"line":10,"character":9}}}]`,
		func(c *LSPClient) {
			locs, err := c.Implementation("file:///a.go", 1, 1)
			if err != nil || len(locs) != 1 {
				t.Fatalf("link -> %+v, %v", locs, err)
			}
			if locs[0].URI != "file:///impl.go" || locs[0].Range.Start.Character != 6 {
				t.Errorf("locationLink = %+v", locs[0])
			}
		})
	// null -> 空切片
	advRPC(t, `null`, func(c *LSPClient) {
		locs, err := c.Implementation("file:///a.go", 1, 1)
		if err != nil || len(locs) != 0 {
			t.Errorf("null -> %+v, %v", locs, err)
		}
	})
}

const callHierarchyItemJSON = `{"name":"compute","kind":12,"detail":"probe",
"uri":"file:///a.go",
"range":{"start":{"line":8,"character":0},"end":{"line":10,"character":1}},
"selectionRange":{"start":{"line":8,"character":5},"end":{"line":8,"character":12}},
"data":{"pkg":"probe","gen":3}}`

func TestLSPAdvCallHierarchy_PrepareAndCalls(t *testing.T) {
	var root LSPCallHierarchyItem
	advRPC(t, `[`+callHierarchyItemJSON+`]`, func(c *LSPClient) {
		items, err := c.CallHierarchyPrepare("file:///a.go", 8, 5)
		if err != nil {
			t.Fatalf("CallHierarchyPrepare: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("items = %+v", items)
		}
		root = items[0]
		if root.Name != "compute" || root.Kind != 12 || root.Detail != "probe" ||
			root.URI != "file:///a.go" || root.SelectionRange.Start.Character != 5 {
			t.Errorf("root = %+v", root)
		}
		if len(root.Data) == 0 {
			t.Error("item data dropped; incoming/outgoingCalls need it verbatim")
		}
	})

	// incomingCalls: 另一端在 "from"
	params := advRPC(t, `[{"from":`+callHierarchyItemJSON+`,"fromRanges":[
{"start":{"line":4,"character":6},"end":{"line":4,"character":13}},
{"start":{"line":5,"character":2},"end":{"line":5,"character":9}}]}]`, func(c *LSPClient) {
		calls, err := c.IncomingCalls(root)
		if err != nil {
			t.Fatalf("IncomingCalls: %v", err)
		}
		if len(calls) != 1 {
			t.Fatalf("calls = %+v", calls)
		}
		if calls[0].Item.Name != "compute" || len(calls[0].Ranges) != 2 ||
			calls[0].Ranges[1].Start.Line != 5 {
			t.Errorf("call = %+v", calls[0])
		}
	})
	// item (含服务器私货 data) 必须原样回传
	var sent struct {
		Item struct {
			Name string          `json:"name"`
			URI  string          `json:"uri"`
			Data json.RawMessage `json:"data"`
		} `json:"item"`
	}
	if err := json.Unmarshal(params, &sent); err != nil {
		t.Fatalf("decode incomingCalls params: %v (%s)", err, string(params))
	}
	if sent.Item.Name != "compute" || sent.Item.URI != "file:///a.go" {
		t.Errorf("params.item = %+v", sent.Item)
	}
	if !bytes.Contains(sent.Item.Data, []byte(`"gen":3`)) {
		t.Errorf("params.item.data = %s, want the server's own data echoed back", string(sent.Item.Data))
	}

	// outgoingCalls: 另一端在 "to"; "from" 的元素要被忽略
	advRPC(t, `[{"to":`+callHierarchyItemJSON+`,"fromRanges":[{"start":{"line":9,"character":8},"end":{"line":9,"character":9}}]},
{"from":`+callHierarchyItemJSON+`,"fromRanges":[]}]`, func(c *LSPClient) {
		calls, err := c.OutgoingCalls(root)
		if err != nil {
			t.Fatalf("OutgoingCalls: %v", err)
		}
		if len(calls) != 1 || calls[0].Ranges[0].Start.Line != 9 {
			t.Errorf("outgoing calls = %+v", calls)
		}
	})

	// null -> 空切片
	advRPC(t, `null`, func(c *LSPClient) {
		if calls, err := c.IncomingCalls(root); err != nil || len(calls) != 0 {
			t.Errorf("null -> %+v, %v", calls, err)
		}
	})
}

func TestLSPAdvTypeHierarchy_PrepareSupertypesSubtypes(t *testing.T) {
	const itemJSON = `{"name":"Server","kind":23,"uri":"file:///s.go",
"range":{"start":{"line":2,"character":0},"end":{"line":6,"character":1}},
"selectionRange":{"start":{"line":2,"character":5},"end":{"line":2,"character":11}},
"data":{"id":"srv"}}`

	var root LSPTypeHierarchyItem
	advRPC(t, `[`+itemJSON+`]`, func(c *LSPClient) {
		items, err := c.TypeHierarchyPrepare("file:///s.go", 2, 5)
		if err != nil {
			t.Fatalf("TypeHierarchyPrepare: %v", err)
		}
		if len(items) != 1 || items[0].Name != "Server" || items[0].Kind != 23 {
			t.Fatalf("items = %+v", items)
		}
		root = items[0]
	})

	params := advRPC(t, `[{"name":"Handler","kind":11,"uri":"file:///h.go",
"range":{"start":{"line":0,"character":0},"end":{"line":2,"character":1}},
"selectionRange":{"start":{"line":0,"character":5},"end":{"line":0,"character":12}}}]`, func(c *LSPClient) {
		sup, err := c.Supertypes(root)
		if err != nil {
			t.Fatalf("Supertypes: %v", err)
		}
		if len(sup) != 1 || sup[0].Name != "Handler" || sup[0].Kind != 11 {
			t.Errorf("supertypes = %+v", sup)
		}
	})
	if !bytes.Contains(params, []byte(`"id":"srv"`)) {
		t.Errorf("supertypes params lost item.data: %s", string(params))
	}

	advRPC(t, `[`+itemJSON+`]`, func(c *LSPClient) {
		sub, err := c.Subtypes(root)
		if err != nil || len(sub) != 1 || sub[0].Name != "Server" {
			t.Errorf("subtypes = %+v, %v", sub, err)
		}
	})
	advRPC(t, `null`, func(c *LSPClient) {
		if sub, err := c.Subtypes(root); err != nil || len(sub) != 0 {
			t.Errorf("null -> %+v, %v", sub, err)
		}
	})
}

func TestLSPAdvInlayHint_LabelShapesAndParams(t *testing.T) {
	// 形状取自 gopls: 一条类型提示 (label 是字符串, paddingLeft), 一条形参名提示
	// (label 分段, paddingRight), 外加 tooltip 的 MarkupContent 形态与 textEdits.
	params := advRPC(t, `[
{"position":{"line":4,"character":2},"label":"int","kind":1,"paddingLeft":true,
 "textEdits":[{"range":{"start":{"line":4,"character":2},"end":{"line":4,"character":2}},"newText":" int"}]},
{"position":{"line":4,"character":14},"label":[{"value":"n"},{"value":": "}],"kind":2,"paddingRight":true,
 "tooltip":{"kind":"markdown","value":"parameter n"}},
{"position":{"line":5,"character":0},"label":42}
]`, func(c *LSPClient) {
		hints, err := c.InlayHint("file:///a.go", 0, 0, 12, 0)
		if err != nil {
			t.Fatalf("InlayHint: %v", err)
		}
		if len(hints) != 3 {
			t.Fatalf("hints = %+v", hints)
		}
		if hints[0].Label != "int" || hints[0].Kind != 1 || !hints[0].PaddingLeft ||
			len(hints[0].TextEdits) != 1 || hints[0].TextEdits[0].NewText != " int" {
			t.Errorf("hints[0] = %+v", hints[0])
		}
		// 分段 label 首尾相接, 中间不插分隔符
		if hints[1].Label != "n: " || hints[1].Kind != 2 || !hints[1].PaddingRight ||
			hints[1].Tooltip != "parameter n" {
			t.Errorf("hints[1] = %+v", hints[1])
		}
		// label 形状不认识时留空, 不报错
		if hints[2].Label != "" || hints[2].Position.Line != 5 {
			t.Errorf("hints[2] = %+v", hints[2])
		}
	})
	// params 是 {textDocument, range}
	var got struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Range LSPRange `json:"range"`
	}
	if err := json.Unmarshal(params, &got); err != nil {
		t.Fatalf("decode inlayHint params: %v", err)
	}
	if got.TextDocument.URI != "file:///a.go" || got.Range.End.Line != 12 {
		t.Errorf("params = %+v", got)
	}

	advRPC(t, `null`, func(c *LSPClient) {
		if hints, err := c.InlayHint("file:///a.go", 0, 0, 1, 0); err != nil || len(hints) != 0 {
			t.Errorf("null -> %+v, %v", hints, err)
		}
	})
}

func TestLSPAdvSemanticTokens_FullAndDelta(t *testing.T) {
	advRPC(t, `{"resultId":"7","data":[0,0,7,9,0, 0,8,4,0,0, 2,0,4,9,0]}`, func(c *LSPClient) {
		st, err := c.SemanticTokensFull("file:///a.go")
		if err != nil {
			t.Fatalf("SemanticTokensFull: %v", err)
		}
		if st == nil {
			t.Fatal("nil tokens")
		}
		if st.ResultID != "7" || len(st.Data) != 15 {
			t.Fatalf("tokens = %+v", st)
		}
		// 相对编码展开成绝对坐标
		toks := DecodeSemanticTokenData(st.Data)
		if len(toks) != 3 {
			t.Fatalf("decoded = %+v", toks)
		}
		if toks[0] != (LSPSemanticToken{Line: 0, Character: 0, Length: 7, TokenType: 9}) {
			t.Errorf("toks[0] = %+v", toks[0])
		}
		// 同一行: 列号累加
		if toks[1] != (LSPSemanticToken{Line: 0, Character: 8, Length: 4, TokenType: 0}) {
			t.Errorf("toks[1] = %+v", toks[1])
		}
		// 换行: 列号重置为 deltaStart
		if toks[2] != (LSPSemanticToken{Line: 2, Character: 0, Length: 4, TokenType: 9}) {
			t.Errorf("toks[2] = %+v", toks[2])
		}
	})

	// delta 形态: edits
	params := advRPC(t, `{"resultId":"8","edits":[{"start":5,"deleteCount":5,"data":[0,3,2,1,0]}]}`, func(c *LSPClient) {
		d, err := c.SemanticTokensFullDelta("file:///a.go", "7")
		if err != nil {
			t.Fatalf("SemanticTokensFullDelta: %v", err)
		}
		if d == nil {
			t.Fatal("nil delta")
		}
		if d.Full {
			t.Error("Full = true for an edits-shaped delta")
		}
		if d.ResultID != "8" || len(d.Edits) != 1 || d.Edits[0].Start != 5 ||
			d.Edits[0].DeleteCount != 5 || len(d.Edits[0].Data) != 5 {
			t.Errorf("delta = %+v", d)
		}
	})
	var sent struct {
		PreviousResultID string `json:"previousResultId"`
	}
	if err := json.Unmarshal(params, &sent); err != nil {
		t.Fatalf("decode delta params: %v", err)
	}
	if sent.PreviousResultID != "7" {
		t.Errorf("previousResultId = %q", sent.PreviousResultID)
	}

	// delta 形态: 服务器放弃增量, 直接回整份 tokens
	advRPC(t, `{"resultId":"9","data":[0,0,3,1,0]}`, func(c *LSPClient) {
		d, err := c.SemanticTokensFullDelta("file:///a.go", "8")
		if err != nil || d == nil {
			t.Fatalf("delta = %+v, %v", d, err)
		}
		if !d.Full || len(d.Data) != 5 || len(d.Edits) != 0 {
			t.Errorf("full-shaped delta = %+v", d)
		}
	})

	advRPC(t, `null`, func(c *LSPClient) {
		if st, err := c.SemanticTokensFull("file:///a.go"); err != nil || st != nil {
			t.Errorf("null -> %+v, %v", st, err)
		}
	})
}

func TestLSPAdvDecodeSemanticTokenData_Edges(t *testing.T) {
	if got := DecodeSemanticTokenData(nil); len(got) != 0 {
		t.Errorf("nil data -> %+v", got)
	}
	// 长度不足 5 的尾巴丢掉, 不 panic
	if got := DecodeSemanticTokenData([]uint32{0, 0, 3, 1, 0, 1, 2}); len(got) != 1 {
		t.Errorf("ragged data -> %+v", got)
	}
}

func TestLSPAdvCodeLens_CommandAndData(t *testing.T) {
	advRPC(t, `[
{"range":{"start":{"line":4,"character":0},"end":{"line":4,"character":18}},
 "command":{"title":"run test","command":"gopls.run_tests","arguments":[{"URI":"file:///a_test.go"}]}},
{"range":{"start":{"line":9,"character":0},"end":{"line":9,"character":5}},"data":{"lazy":true}}
]`, func(c *LSPClient) {
		lenses, err := c.CodeLens("file:///a_test.go")
		if err != nil {
			t.Fatalf("CodeLens: %v", err)
		}
		if len(lenses) != 2 {
			t.Fatalf("lenses = %+v", lenses)
		}
		if lenses[0].Range.Start.Line != 4 || lenses[0].Command == nil {
			t.Fatalf("lenses[0] = %+v", lenses[0])
		}
		if lenses[0].Command.Command != "gopls.run_tests" || lenses[0].Command.Title != "run test" ||
			!bytes.Contains(lenses[0].Command.Arguments, []byte("a_test.go")) {
			t.Errorf("lens command = %+v", lenses[0].Command)
		}
		// 未解析的 lens: 没有 command, data 留着
		if lenses[1].Command != nil {
			t.Errorf("lenses[1].Command = %+v, want nil", lenses[1].Command)
		}
		if !bytes.Contains(lenses[1].Data, []byte("lazy")) {
			t.Errorf("lenses[1].Data = %s", string(lenses[1].Data))
		}
	})

	advRPC(t, `null`, func(c *LSPClient) {
		if lenses, err := c.CodeLens("file:///a.go"); err != nil || len(lenses) != 0 {
			t.Errorf("null -> %+v, %v", lenses, err)
		}
	})
}

// -----------------------------------------------------------------------------
// 优雅降级 vs 真错误
// -----------------------------------------------------------------------------

// 服务器回 MethodNotFound (没实现这个 method) 时, 上层拿到的是空结果而不是错误:
// 一个老服务器不该让 UI 弹一堆报错.
func TestLSPAdvMethodNotFound_DegradesToEmpty(t *testing.T) {
	calls := map[string]func(c *LSPClient) (int, error){
		"implementation": func(c *LSPClient) (int, error) {
			v, err := c.Implementation("file:///a.go", 1, 1)
			return len(v), err
		},
		"prepareCallHierarchy": func(c *LSPClient) (int, error) {
			v, err := c.CallHierarchyPrepare("file:///a.go", 1, 1)
			return len(v), err
		},
		"incomingCalls": func(c *LSPClient) (int, error) {
			v, err := c.IncomingCalls(LSPCallHierarchyItem{Name: "x"})
			return len(v), err
		},
		"outgoingCalls": func(c *LSPClient) (int, error) {
			v, err := c.OutgoingCalls(LSPCallHierarchyItem{Name: "x"})
			return len(v), err
		},
		"prepareTypeHierarchy": func(c *LSPClient) (int, error) {
			v, err := c.TypeHierarchyPrepare("file:///a.go", 1, 1)
			return len(v), err
		},
		"supertypes": func(c *LSPClient) (int, error) {
			v, err := c.Supertypes(LSPTypeHierarchyItem{Name: "x"})
			return len(v), err
		},
		"subtypes": func(c *LSPClient) (int, error) {
			v, err := c.Subtypes(LSPTypeHierarchyItem{Name: "x"})
			return len(v), err
		},
		"inlayHint": func(c *LSPClient) (int, error) {
			v, err := c.InlayHint("file:///a.go", 0, 0, 1, 0)
			return len(v), err
		},
		"codeLens": func(c *LSPClient) (int, error) {
			v, err := c.CodeLens("file:///a.go")
			return len(v), err
		},
		"prepareRename": func(c *LSPClient) (int, error) {
			v, err := c.PrepareRename("file:///a.go", 1, 1)
			if v != nil {
				return 1, err
			}
			return 0, err
		},
		"semanticTokens": func(c *LSPClient) (int, error) {
			v, err := c.SemanticTokensFull("file:///a.go")
			if v != nil {
				return 1, err
			}
			return 0, err
		},
		"semanticTokensDelta": func(c *LSPClient) (int, error) {
			v, err := c.SemanticTokensFullDelta("file:///a.go", "1")
			if v != nil {
				return 1, err
			}
			return 0, err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			c, srvIn, srvOut := newPipedClient(t)
			defer func() { _ = c.Close() }()
			done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
				return &fakeReply{Err: &LSPError{Code: lspMethodNotFound, Message: "method not supported"}}
			})
			n, err := call(c)
			if err != nil {
				t.Errorf("MethodNotFound surfaced as an error: %v", err)
			}
			if n != 0 {
				t.Errorf("result count = %d, want 0", n)
			}
			_ = srvIn.Close()
			_ = srvOut.Close()
			<-done
		})
	}
}

// 真错误照常透出: 服务器内部错不能被当成"没这个能力"吞掉.
func TestLSPAdvServerError_StillSurfaces(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()
	done := runFakeServer(t, srvIn, srvOut, func(_ string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		return &fakeReply{Err: &LSPError{Code: -32603, Message: "not a type name"}}
	})

	if _, err := c.TypeHierarchyPrepare("file:///a.go", 1, 1); err == nil {
		t.Error("TypeHierarchyPrepare swallowed a server error")
	} else if !strings.Contains(err.Error(), "not a type name") {
		t.Errorf("err = %v, want the server message", err)
	}
	if _, err := c.InlayHint("file:///a.go", 0, 0, 1, 0); err == nil {
		t.Error("InlayHint swallowed a server error")
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
	<-done
}

// 解码失败要报错, 不能悄悄返回空结果 (那会把"协议对不上"伪装成"没有结果").
func TestLSPAdvMalformedResults_ReportErrors(t *testing.T) {
	advRPC(t, `{"not":"an array"}`, func(c *LSPClient) {
		if _, err := c.CallHierarchyPrepare("file:///a.go", 1, 1); err == nil {
			t.Error("CallHierarchyPrepare accepted a non-array result")
		}
	})
	advRPC(t, `["nope"]`, func(c *LSPClient) {
		if _, err := c.InlayHint("file:///a.go", 0, 0, 1, 0); err == nil {
			t.Error("InlayHint accepted a string element")
		}
	})
	advRPC(t, `[1,2,3]`, func(c *LSPClient) {
		if _, err := c.Implementation("file:///a.go", 1, 1); err == nil {
			t.Error("Implementation accepted numeric locations")
		}
	})
}

// -----------------------------------------------------------------------------
// handler 的注册/撤销 + 读循环里真实的一趟回包
// -----------------------------------------------------------------------------

func TestLSPAdvSetHandlers_NilClears(t *testing.T) {
	c, buf := advOfflineClient()
	c.SetApplyEditHandler(func(string, *LSPWorkspaceEdit) bool { return true })
	c.SetApplyEditHandler(nil)
	c.SetConfigurationHandler(func(string, string) json.RawMessage { return json.RawMessage(`{"x":1}`) })
	c.SetConfigurationHandler(nil)

	id := json.RawMessage(`1`)
	routeMessage(&LSPMessage{JSONRPC: "2.0", ID: &id, Method: "workspace/applyEdit", Params: json.RawMessage(applyEditParams)},
		nil, nil, c)
	reply := advReadFrame(t, buf.String())
	if !bytes.Contains(reply.Result, []byte(`"applied":false`)) {
		t.Errorf("after clearing the handler, result = %s", string(reply.Result))
	}
}

// 端到端: 假服务器主动发一条 workspace/configuration 请求, 客户端的 readLoop
// 必须把响应写回到那根 pipe 上 (这是 gopls 初始化时真实会发生的一步).
func TestLSPAdvReadLoop_AnswersServerRequest(t *testing.T) {
	c, srvIn, srvOut := newPipedClient(t)
	defer func() { _ = c.Close() }()

	c.SetConfigurationHandler(func(scopeURI, section string) json.RawMessage {
		if section != "gopls" {
			t.Errorf("section = %q", section)
		}
		return json.RawMessage(`{"semanticTokens":true}`)
	})

	replies := make(chan *LSPMessage, 1)
	go func() {
		br := bufio.NewReader(srvIn)
		for {
			m, err := ReadLSPMessage(br)
			if err != nil {
				return
			}
			if m.ID != nil && m.Method == "" {
				select {
				case replies <- m:
				default:
				}
			}
		}
	}()

	id := json.RawMessage(`77`)
	if err := WriteLSPMessage(srvOut, &LSPMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "workspace/configuration",
		Params:  json.RawMessage(`{"items":[{"scopeUri":"file:///w","section":"gopls"}]}`),
	}); err != nil {
		t.Fatalf("write server request: %v", err)
	}

	select {
	case reply := <-replies:
		if reply.ID == nil || string(*reply.ID) != "77" {
			t.Errorf("reply id = %v", reply.ID)
		}
		if !bytes.Contains(reply.Result, []byte("semanticTokens")) {
			t.Errorf("reply result = %s", string(reply.Result))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("read loop never answered the server request")
	}

	_ = srvIn.Close()
	_ = srvOut.Close()
}

package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// LSP JSON-RPC 2.0 wire-protocol primitives.
// 这里只实现 Microsoft Language Server Protocol 的"分帧层":
// 每条消息以一组 HTTP 风格的 header 起头, 至少有 "Content-Length: N",
// 然后一个空的 "\r\n" 行作为分隔符, 之后跟随 N 字节的 JSON 负载.
// 例如:
//   Content-Length: 56\r\n
//   \r\n
//   {"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
//
// 本文件刻意 *不* 实现更高层的 LSP 概念 (initialize/handshake, didOpen/
// didChange notifications, completion request shapes 等). 那些请求/响应
// 形状属于 future 的 core/lspclient.go: 它会建立在这里的 ReadLSPMessage /
// WriteLSPMessage / NewRequest / NewNotification 之上, 来跟 gopls 通信.
// 把 framing 拆开是为了在不引入完整 LSP 类型依赖的前提下被复用 (例如做协
// 议抓包/replay/单元测试).

// LSPMessage 是一个解码后的 JSON-RPC 2.0 消息
// 同一个结构体覆盖 request / response / notification 三种用途:
//   - request:      JSONRPC + ID(string|number) + Method + Params
//   - response:     JSONRPC + ID(string|number) + (Result XOR Error)
//   - notification: JSONRPC + Method + Params, ID 为 nil
//
// ID 之所以是 *json.RawMessage 而不是 interface{}/string/int, 是为了:
//   - 准确区分 "ID 缺失" 和 "ID 为 0/空字符串"
//   - 透明 round-trip: server 给的 ID 是字符串还是数字, 原样还给上层
//
// Params/Result 同样保留为 RawMessage, 让具体 LSP method 的反序列化由更
// 高一层来做(通过 DecodeParams), 这里不耦合任何 method-specific 类型.
type LSPMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *LSPError        `json:"error,omitempty"`
}

// LSPError 是 JSON-RPC 2.0 中 response.error 的内嵌对象
// Code 取标准定义(-32700 ParseError, -32600 InvalidRequest, ...), 但本
// 文件不去校验具体取值, 仅做透明承载
type LSPError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// ReadLSPMessage 从 bufio.Reader 中读取一条完整的 LSP 消息
// 流程:
//  1. 按行读 header, 行结束符必须是 "\r\n"; 空行(只剩 "\r\n")标志 header 结束
//  2. header 中必须含有 "Content-Length: N", 大小写不敏感
//  3. 用 io.ReadFull 严格读 N 字节作为 body, 短读直接报错
//  4. 用 encoding/json 把 body 反序列化成 LSPMessage
//
// 任何畸形输入都通过 error 返回; 不会 panic
func ReadLSPMessage(r *bufio.Reader) (*LSPMessage, error) {
	if r == nil {
		return nil, errors.New("lsp: nil reader")
	}

	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			// 读 header 时遇 EOF 也算错: 一条合法消息至少要有 Content-Length 头
			return nil, fmt.Errorf("lsp: read header: %w", err)
		}
		// LSP 强制 "\r\n" 行结束符
		if !strings.HasSuffix(line, "\r\n") {
			return nil, fmt.Errorf("lsp: header line missing CRLF: %q", line)
		}
		line = strings.TrimSuffix(line, "\r\n")
		if line == "" {
			// header 结束
			break
		}
		// header 形如 "Name: Value"
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			return nil, fmt.Errorf("lsp: malformed header (no colon): %q", line)
		}
		name := strings.TrimSpace(line[:colon])
		value := strings.TrimSpace(line[colon+1:])
		if strings.EqualFold(name, "Content-Length") {
			n, perr := strconv.Atoi(value)
			if perr != nil {
				return nil, fmt.Errorf("lsp: invalid Content-Length %q: %w", value, perr)
			}
			if n < 0 {
				return nil, fmt.Errorf("lsp: negative Content-Length %d", n)
			}
			contentLength = n
		}
		// 其他 header (Content-Type, ...) 静默忽略: LSP 只强依赖 Content-Length
	}

	if contentLength < 0 {
		return nil, errors.New("lsp: missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		// io.ErrUnexpectedEOF 在短读时出现, 一并 wrap
		return nil, fmt.Errorf("lsp: read body (want %d bytes): %w", contentLength, err)
	}

	var m LSPMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("lsp: decode body: %w", err)
	}
	return &m, nil
}

// WriteLSPMessage 把一条 LSPMessage 序列化为 JSON 并按 LSP framing 写出
// 写出顺序:
//  1. 序列化 body 以便准确算 Content-Length
//  2. 写 "Content-Length: N\r\n\r\n"
//  3. 写 body
//
// JSONRPC 字段为空时默认填 "2.0", 让调用方可以省事地只填 Method/Params
func WriteLSPMessage(w io.Writer, m *LSPMessage) error {
	if w == nil {
		return errors.New("lsp: nil writer")
	}
	if m == nil {
		return errors.New("lsp: nil message")
	}
	// 不修改调用方传入的对象: 复制一份再写回 JSONRPC
	out := *m
	if out.JSONRPC == "" {
		out.JSONRPC = "2.0"
	}
	body, err := json.Marshal(&out)
	if err != nil {
		return fmt.Errorf("lsp: encode body: %w", err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("lsp: write header: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("lsp: write body: %w", err)
	}
	return nil
}

// NewRequest 构造一个 JSON-RPC 请求消息
// id 以数字形式编码; LSP 协议两种都接受, 数字是绝大多数客户端的默认选择
// params 可以是 nil/任意可 JSON-marshal 的值; 已经是 json.RawMessage 时
// 也会按 raw bytes 直接进对应字段
func NewRequest(id int, method string, params interface{}) (*LSPMessage, error) {
	idRaw, err := json.Marshal(id)
	if err != nil {
		return nil, fmt.Errorf("lsp: encode id: %w", err)
	}
	raw := json.RawMessage(idRaw)
	m := &LSPMessage{
		JSONRPC: "2.0",
		ID:      &raw,
		Method:  method,
	}
	if params != nil {
		p, err := marshalParams(params)
		if err != nil {
			return nil, err
		}
		m.Params = p
	}
	return m, nil
}

// NewNotification 构造一个 JSON-RPC 通知消息 (无 ID)
// LSP 中所有 "didChange/didOpen/..." 都是 notification, 服务器不会回复
func NewNotification(method string, params interface{}) (*LSPMessage, error) {
	m := &LSPMessage{
		JSONRPC: "2.0",
		Method:  method,
	}
	if params != nil {
		p, err := marshalParams(params)
		if err != nil {
			return nil, err
		}
		m.Params = p
	}
	return m, nil
}

// IsNotification 在 ID 缺失时返回 true
// 调用方据此决定要不要等响应
func IsNotification(m *LSPMessage) bool {
	return m != nil && m.ID == nil
}

// DecodeParams 把 m.Params 反序列化进 into 指向的对象
// 便利封装: 等价于 json.Unmarshal(m.Params, into), 但对常见空值更友好
// (m == nil 或 Params 为空时不返回错误, into 保持零值)
func DecodeParams(m *LSPMessage, into interface{}) error {
	if m == nil || len(m.Params) == 0 {
		return nil
	}
	if err := json.Unmarshal(m.Params, into); err != nil {
		return fmt.Errorf("lsp: decode params: %w", err)
	}
	return nil
}

// marshalParams 把任意 params 值转成 json.RawMessage
// 当 caller 已经传入 RawMessage 时跳过二次封装, 直接使用其字节
func marshalParams(params interface{}) (json.RawMessage, error) {
	if raw, ok := params.(json.RawMessage); ok {
		return raw, nil
	}
	b, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("lsp: encode params: %w", err)
	}
	return b, nil
}

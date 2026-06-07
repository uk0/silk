package core

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// 构造一段带 LSP framing 的字节串, 测试输入辅助
func frame(body string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

func TestLSPMessage_RoundTrip(t *testing.T) {
	// build -> write -> read 应当恢复出语义等价的消息
	type pingParams struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	want, err := NewRequest(7, "ping", pingParams{Name: "hi", N: 42})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	var buf bytes.Buffer
	if err := WriteLSPMessage(&buf, want); err != nil {
		t.Fatalf("WriteLSPMessage: %v", err)
	}

	// header 应当是单一的 Content-Length 行 + 空行 + body
	if !bytes.HasPrefix(buf.Bytes(), []byte("Content-Length: ")) {
		t.Errorf("written bytes missing Content-Length prefix: %q", buf.String())
	}

	got, err := ReadLSPMessage(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("ReadLSPMessage: %v", err)
	}
	if got.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", got.JSONRPC, "2.0")
	}
	if got.Method != "ping" {
		t.Errorf("Method = %q, want %q", got.Method, "ping")
	}
	if got.ID == nil {
		t.Fatalf("ID is nil, want present")
	}
	if string(*got.ID) != "7" {
		t.Errorf("ID raw = %q, want %q", string(*got.ID), "7")
	}
	var gotParams pingParams
	if err := json.Unmarshal(got.Params, &gotParams); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	wantParams := pingParams{Name: "hi", N: 42}
	if gotParams != wantParams {
		t.Errorf("params = %+v, want %+v", gotParams, wantParams)
	}
}

func TestReadLSPMessage_Request(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":"file:///tmp"}}`
	r := bufio.NewReader(strings.NewReader(frame(body)))
	m, err := ReadLSPMessage(r)
	if err != nil {
		t.Fatalf("ReadLSPMessage: %v", err)
	}
	if m.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", m.JSONRPC, "2.0")
	}
	if m.Method != "initialize" {
		t.Errorf("Method = %q, want %q", m.Method, "initialize")
	}
	if m.ID == nil || string(*m.ID) != "1" {
		t.Errorf("ID raw = %v, want %q", m.ID, "1")
	}
	if IsNotification(m) {
		t.Errorf("IsNotification = true, want false for a request")
	}
	var p struct {
		RootURI string `json:"rootUri"`
	}
	if err := DecodeParams(m, &p); err != nil {
		t.Fatalf("DecodeParams: %v", err)
	}
	if p.RootURI != "file:///tmp" {
		t.Errorf("rootUri = %q", p.RootURI)
	}
}

func TestReadLSPMessage_Response(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":2,"result":{"capabilities":{"hoverProvider":true}}}`
	m, err := ReadLSPMessage(bufio.NewReader(strings.NewReader(frame(body))))
	if err != nil {
		t.Fatalf("ReadLSPMessage: %v", err)
	}
	if m.ID == nil || string(*m.ID) != "2" {
		t.Errorf("ID raw = %v, want %q", m.ID, "2")
	}
	if m.Method != "" {
		t.Errorf("Method = %q, want empty for response", m.Method)
	}
	if len(m.Result) == 0 {
		t.Fatalf("Result is empty")
	}
	if m.Error != nil {
		t.Errorf("Error = %+v, want nil", m.Error)
	}
	var r struct {
		Capabilities struct {
			HoverProvider bool `json:"hoverProvider"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(m.Result, &r); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !r.Capabilities.HoverProvider {
		t.Errorf("hoverProvider = false, want true")
	}
}

func TestReadLSPMessage_ErrorResponse(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":"abc","error":{"code":-32601,"message":"Method not found"}}`
	m, err := ReadLSPMessage(bufio.NewReader(strings.NewReader(frame(body))))
	if err != nil {
		t.Fatalf("ReadLSPMessage: %v", err)
	}
	if m.ID == nil {
		t.Fatalf("ID is nil, want string \"abc\"")
	}
	// 字符串 ID 应原样保留 (含引号)
	if string(*m.ID) != `"abc"` {
		t.Errorf("ID raw = %q, want %q", string(*m.ID), `"abc"`)
	}
	if m.Error == nil {
		t.Fatalf("Error is nil, want populated")
	}
	if m.Error.Code != -32601 {
		t.Errorf("Error.Code = %d, want -32601", m.Error.Code)
	}
	if m.Error.Message != "Method not found" {
		t.Errorf("Error.Message = %q", m.Error.Message)
	}
}

func TestReadLSPMessage_Notification(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///x.go"}}}`
	m, err := ReadLSPMessage(bufio.NewReader(strings.NewReader(frame(body))))
	if err != nil {
		t.Fatalf("ReadLSPMessage: %v", err)
	}
	if !IsNotification(m) {
		t.Errorf("IsNotification = false, want true (ID absent)")
	}
	if m.Method != "textDocument/didOpen" {
		t.Errorf("Method = %q", m.Method)
	}
}

func TestReadLSPMessage_MissingContentLength(t *testing.T) {
	// header 完整结束但没有 Content-Length
	input := "Content-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n{}"
	_, err := ReadLSPMessage(bufio.NewReader(strings.NewReader(input)))
	if err == nil {
		t.Fatal("expected error for missing Content-Length, got nil")
	}
	if !strings.Contains(err.Error(), "Content-Length") {
		t.Errorf("error = %v; want mention of Content-Length", err)
	}
}

func TestReadLSPMessage_ShortBody(t *testing.T) {
	// 宣称 100 字节但实际只给 5 字节
	input := "Content-Length: 100\r\n\r\nshort"
	_, err := ReadLSPMessage(bufio.NewReader(strings.NewReader(input)))
	if err == nil {
		t.Fatal("expected error for short body, got nil")
	}
	if !strings.Contains(err.Error(), "body") {
		t.Errorf("error = %v; want mention of body", err)
	}
}

func TestReadLSPMessage_NonJSONBody(t *testing.T) {
	input := frame("this is not json")
	_, err := ReadLSPMessage(bufio.NewReader(strings.NewReader(input)))
	if err == nil {
		t.Fatal("expected error for non-JSON body, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error = %v; want mention of decode", err)
	}
}

func TestReadLSPMessage_MalformedHeader(t *testing.T) {
	// header 行没有冒号
	input := "BogusHeader\r\n\r\n{}"
	_, err := ReadLSPMessage(bufio.NewReader(strings.NewReader(input)))
	if err == nil {
		t.Fatal("expected error for malformed header, got nil")
	}
}

func TestReadLSPMessage_HeaderMissingCRLF(t *testing.T) {
	// LF 而非 CRLF
	input := "Content-Length: 2\n\n{}"
	_, err := ReadLSPMessage(bufio.NewReader(strings.NewReader(input)))
	if err == nil {
		t.Fatal("expected error for header without CRLF, got nil")
	}
}

func TestNewRequest_ShapesIDAndMethod(t *testing.T) {
	m, err := NewRequest(11, "textDocument/completion", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if m.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q", m.JSONRPC)
	}
	if m.ID == nil {
		t.Fatalf("ID is nil")
	}
	if string(*m.ID) != "11" {
		t.Errorf("ID raw = %q, want %q", string(*m.ID), "11")
	}
	if m.Method != "textDocument/completion" {
		t.Errorf("Method = %q", m.Method)
	}
	if IsNotification(m) {
		t.Errorf("IsNotification = true, want false")
	}
	// Params 应当是合法 JSON
	var got map[string]int
	if err := json.Unmarshal(m.Params, &got); err != nil {
		t.Fatalf("Params not valid JSON: %v (%s)", err, string(m.Params))
	}
	if got["x"] != 1 {
		t.Errorf("params[x] = %d, want 1", got["x"])
	}
}

func TestNewNotification_NoID(t *testing.T) {
	m, err := NewNotification("textDocument/didSave", nil)
	if err != nil {
		t.Fatalf("NewNotification: %v", err)
	}
	if m.ID != nil {
		t.Errorf("ID = %v, want nil for notification", m.ID)
	}
	if !IsNotification(m) {
		t.Errorf("IsNotification = false, want true")
	}
	if m.Method != "textDocument/didSave" {
		t.Errorf("Method = %q", m.Method)
	}
	if len(m.Params) != 0 {
		t.Errorf("Params = %q, want empty when nil params passed", string(m.Params))
	}
}

func TestDecodeParams_RoundTrip(t *testing.T) {
	type completionParams struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position struct {
			Line int `json:"line"`
			Char int `json:"character"`
		} `json:"position"`
	}
	var want completionParams
	want.TextDocument.URI = "file:///a/b.go"
	want.Position.Line = 12
	want.Position.Char = 4

	m, err := NewRequest(3, "textDocument/completion", want)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	var got completionParams
	if err := DecodeParams(m, &got); err != nil {
		t.Fatalf("DecodeParams: %v", err)
	}
	if got != want {
		t.Errorf("decoded = %+v, want %+v", got, want)
	}

	// 空消息上的 DecodeParams 应当安全
	if err := DecodeParams(nil, &got); err != nil {
		t.Errorf("DecodeParams(nil) = %v, want nil", err)
	}
	if err := DecodeParams(&LSPMessage{}, &got); err != nil {
		t.Errorf("DecodeParams(empty) = %v, want nil", err)
	}
}

func TestWriteLSPMessage_DefaultsJSONRPC(t *testing.T) {
	// JSONRPC 留空时应默认填 "2.0"
	m := &LSPMessage{Method: "ping"}
	var buf bytes.Buffer
	if err := WriteLSPMessage(&buf, m); err != nil {
		t.Fatalf("WriteLSPMessage: %v", err)
	}
	// 调用方传入的对象不被修改
	if m.JSONRPC != "" {
		t.Errorf("caller's message mutated: JSONRPC = %q", m.JSONRPC)
	}
	got, err := ReadLSPMessage(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("ReadLSPMessage: %v", err)
	}
	if got.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", got.JSONRPC, "2.0")
	}
}

func TestWriteLSPMessage_NilGuard(t *testing.T) {
	if err := WriteLSPMessage(nil, &LSPMessage{}); err == nil {
		t.Error("WriteLSPMessage(nil writer) = nil, want error")
	}
	var buf bytes.Buffer
	if err := WriteLSPMessage(&buf, nil); err == nil {
		t.Error("WriteLSPMessage(nil msg) = nil, want error")
	}
}

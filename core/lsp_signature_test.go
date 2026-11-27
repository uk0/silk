package core

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestSignatureHintUsesPerSignatureParameterAndUTF16Offsets(t *testing.T) {
	label := "𐐀calc(count int, 名称 string) int"
	active := "名称 string"
	index := strings.Index(label, active)
	units := func(s string) int { return len(utf16.Encode([]rune(s))) }
	wire := map[string]any{
		"activeSignature": 1, "activeParameter": 0,
		"signatures": []any{
			map[string]any{"label": "another()"},
			map[string]any{
				"label": label, "activeParameter": 1,
				"parameters": []map[string]any{
					{"label": "count int"},
					{"label": []int{units(label[:index]), units(label[:index+len(active)])}},
				},
			},
		},
	}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	c, in, out := newPipedClient(t)
	done := runFakeServer(t, in, out, func(method string, _ *json.RawMessage, _ json.RawMessage) *fakeReply {
		if method != "textDocument/signatureHelp" {
			t.Errorf("unexpected method %q", method)
		}
		return &fakeReply{Result: data}
	})
	defer func() {
		_ = in.Close()
		_ = out.Close()
		<-done
		_ = c.Close()
	}()
	got, err := c.SignatureHint("file:///qa.go", 1, 17)
	if err != nil || got.Label != label || got.ParameterStart != index || got.ParameterEnd != index+len(active) {
		t.Fatalf("selected signature: %+v, %v", got, err)
	}
	old, err := c.SignatureHelp("file:///qa.go", 1, 17)
	if err != nil || old == nil || old.Signatures[1].Parameters[1] != active || old.ActiveParameter != 0 {
		t.Fatalf("legacy parameter view changed: %+v, %v", old, err)
	}
}

func TestSignatureHintPlainFallbackAndProtocolDefaults(t *testing.T) {
	for _, tc := range []struct{ name, wire, fragment string }{
		{"defaults", `{"activeSignature":8,"activeParameter":7,"signatures":[{"label":"f(x int)","parameters":[{"label":"x int"}]}]}`, "x int"},
		{"legacy duplicate", `{"signatures":[{"label":"f(int, int)","parameters":[{"label":"int"},{"label":"int"}]}]}`, ""},
		{"middle of surrogate", `{"signatures":[{"label":"𐐀f(x int)","parameters":[{"label":[1,8]}]}]}`, ""},
		{"oversize", `{"signatures":[{"label":"f(x int)","parameters":[{"label":[2,100]}]}]}`, ""},
		{"negative", `{"signatures":[{"label":"f(x int)","parameters":[{"label":[-1,4]}]}]}`, ""},
		{"null boundary", `{"signatures":[{"label":"f(x int)","parameters":[{"label":[null,4]}]}]}`, ""},
		{"invalid tuple length", `{"signatures":[{"label":"f(x int)","parameters":[{"label":[0,1,2]}]}]}`, ""},
		{"no params", `{"signatures":[{"label":"f()"}]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := parseSignatureHelp(json.RawMessage(tc.wire))
			if err != nil {
				t.Fatal(err)
			}
			h := data.activeHint()
			if h.Label == "" {
				t.Fatal("bad parameter label discarded the signature")
			}
			if got := h.Label[h.ParameterStart:h.ParameterEnd]; got != tc.fragment {
				t.Fatalf("highlight %q, want %q", got, tc.fragment)
			}
		})
	}
}

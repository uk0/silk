package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// LSPActiveSignature is a plain callable signature with an optional highlighted
// parameter. Offsets are UTF-8 byte indices; equal offsets mean no highlight.
type LSPActiveSignature struct {
	Label                        string
	ParameterStart, ParameterEnd int
}

type rawSignatureHelp struct {
	Signatures []struct {
		Label           string          `json:"label"`
		Documentation   json.RawMessage `json:"documentation"`
		ActiveParameter *int            `json:"activeParameter"`
		Parameters      []struct {
			Label json.RawMessage `json:"label"`
		} `json:"parameters"`
	} `json:"signatures"`
	ActiveSignature int `json:"activeSignature"`
	ActiveParameter int `json:"activeParameter"`
}

// SignatureHelp requests textDocument/signatureHelp. The established SDK shape
// remains unchanged; both string and offset parameter labels are exposed as
// strings. A null result means there is no call at the current location.
func (c *LSPClient) SignatureHelp(uri string, line, character int) (*LSPSignatureHelp, error) {
	raw, err := c.requestSignatureHelp(uri, line, character)
	if err != nil || raw == nil {
		return nil, err
	}
	out := &LSPSignatureHelp{
		ActiveSignature: raw.ActiveSignature,
		ActiveParameter: raw.ActiveParameter,
		Signatures:      make([]LSPSignature, 0, len(raw.Signatures)),
	}
	for _, s := range raw.Signatures {
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
			text, _, _ := decodeSignatureParameter(s.Label, p.Label)
			sig.Parameters = append(sig.Parameters, text)
		}
		out.Signatures = append(out.Signatures, sig)
	}
	return out, nil
}

// SignatureHint requests the same server response as SignatureHelp and picks
// the active signature and parameter for presentation. A malformed or
// ambiguous parameter label does not discard the signature itself.
func (c *LSPClient) SignatureHint(uri string, line, character int) (LSPActiveSignature, error) {
	return c.SignatureHintContext(context.Background(), uri, line, character)
}

// SignatureHintContext lets an editor retire an obsolete signature request.
func (c *LSPClient) SignatureHintContext(ctx context.Context, uri string, line, character int) (LSPActiveSignature, error) {
	raw, err := c.requestSignatureHelpContext(ctx, uri, line, character)
	if err != nil || raw == nil {
		return LSPActiveSignature{}, err
	}
	return raw.activeHint(), nil
}

func (c *LSPClient) requestSignatureHelp(uri string, line, character int) (*rawSignatureHelp, error) {
	return c.requestSignatureHelpContext(context.Background(), uri, line, character)
}

func (c *LSPClient) requestSignatureHelpContext(ctx context.Context, uri string, line, character int) (*rawSignatureHelp, error) {
	resp, err := c.SendRequestContext(ctx, "textDocument/signatureHelp", textDocumentPositionParams(uri, line, character))
	if err != nil {
		return nil, err
	}
	return parseSignatureHelp(resp.Result)
}

func parseSignatureHelp(result json.RawMessage) (*rawSignatureHelp, error) {
	result = bytes.TrimSpace(result)
	if len(result) == 0 || bytes.Equal(result, []byte("null")) {
		return nil, nil
	}
	var sh rawSignatureHelp
	if err := json.Unmarshal(result, &sh); err != nil {
		return nil, fmt.Errorf("lspclient: decode signatureHelp result: %w", err)
	}
	return &sh, nil
}

func (h *rawSignatureHelp) activeHint() LSPActiveSignature {
	if len(h.Signatures) == 0 {
		return LSPActiveSignature{}
	}
	signature := h.ActiveSignature
	if signature < 0 || signature >= len(h.Signatures) {
		signature = 0
	}
	s := h.Signatures[signature]
	hint := LSPActiveSignature{Label: s.Label}
	if len(s.Parameters) == 0 {
		return hint
	}
	parameter := h.ActiveParameter
	if s.ActiveParameter != nil {
		parameter = *s.ActiveParameter
	}
	if parameter < 0 || parameter >= len(s.Parameters) {
		parameter = 0
	}
	_, hint.ParameterStart, hint.ParameterEnd = decodeSignatureParameter(s.Label, s.Parameters[parameter].Label)
	return hint
}

// LSP ParameterInformation labels are either strings or [start,end) UTF-16
// code-unit offsets in the original signature. Never clamp an invalid offset
// or highlight a similarly spelled but ambiguous substring.
func decodeSignatureParameter(label string, raw json.RawMessage) (text string, start, end int) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", 0, 0
	}
	if raw[0] == '"' {
		if json.Unmarshal(raw, &text) != nil || text == "" {
			return "", 0, 0
		}
		if i := strings.Index(label, text); i >= 0 && strings.LastIndex(label, text) == i {
			return text, i, i + len(text)
		}
		return text, 0, 0
	}
	var pair []*int
	if raw[0] != '[' || json.Unmarshal(raw, &pair) != nil || len(pair) != 2 || pair[0] == nil || pair[1] == nil || *pair[0] < 0 || *pair[0] >= *pair[1] {
		return "", 0, 0
	}
	left, ok := signatureUTF16Boundary(label, *pair[0])
	if !ok {
		return "", 0, 0
	}
	right, ok := signatureUTF16Boundary(label, *pair[1])
	if !ok {
		return "", 0, 0
	}
	return label[left:right], left, right
}

func signatureUTF16Boundary(s string, offset int) (int, bool) {
	units := 0
	for i, r := range s {
		if units == offset {
			return i, true
		}
		if units > offset {
			return 0, false // inside a surrogate pair
		}
		units += utf16RuneLen(r)
	}
	return len(s), units == offset
}

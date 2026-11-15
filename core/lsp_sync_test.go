package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/ide/lspedit"
)

func applyComputedLSPChange(t testing.TB, previous string, c LSPContentChange) string {
	t.Helper()
	if c.Range == nil {
		return c.Text
	}
	r := c.Range
	got, err := lspedit.ApplyEdits(previous, []lspedit.TextEdit{{Range: lspedit.Range{Start: lspedit.Position{Line: r.Start.Line, Character: r.Start.Character}, End: lspedit.Position{Line: r.End.Line, Character: r.End.Character}}, NewText: c.Text}})
	if err != nil {
		t.Fatalf("invalid computed range %+v on %q: %v", r, previous, err)
	}
	return got
}

func TestLSPTextChangeExactUTF16AndCRLF(t *testing.T) {
	pad := strings.Repeat("unchanged padding 😀 中文\r\n", 10)
	for _, p := range [][2]string{
		{"", ""}, {"a", "b"}, {"a", ""}, {"", "new"},
		{pad, pad + "tail"}, {pad + "tail", pad}, {pad + "é", pad + "ê"},
		{pad + "😀", pad + "😁"}, {pad + "𐀀", pad + "😀"}, {pad + "é", pad + "é"},
		{pad + "a👨‍👩‍👧b", pad + "a👨‍👩‍👦b"},
		{pad + "abc\r\ndef", pad + "abc\rx\ndef"},
		{pad + "abc\r\ndef", pad + "abc\ndef"},
		{pad + "abc\ndef", pad + "abc\r\ndef"},
		{pad + "abc\r\ndef", pad + "abc\r\n\r\ndef"},
		{pad + "abc\r\ndef", pad + "abc\r\n新😀def"},
		{"start\r\n" + pad + "end", "START\r\n" + pad + "END"},
		{pad, pad}, {pad + "\ufeff", pad + "\ufeffx"},
	} {
		c := ComputeLSPTextChange(p[0], p[1])
		if got := applyComputedLSPChange(t, p[0], c); got != p[1] {
			t.Fatalf("%q -> %q: got %q (%+v)", p[0], p[1], got, c)
		}
		if c.Range != nil && (!utf8.ValidString(c.Text) || strings.HasSuffix(c.Text, "\r")) {
			t.Fatalf("split encoding/CRLF: %+v", c)
		}
	}
	old := pad + "甲😀乙末\r\nlast"
	next := pad + "甲😀新末\r\nlast"
	c := ComputeLSPTextChange(old, next)
	want := LSPRange{Start: LSPPosition{Line: 10, Character: 3}, End: LSPPosition{Line: 10, Character: 4}}
	if c.Range == nil || *c.Range != want || c.Text != "新" {
		t.Fatalf("UTF-16 position not exact: %+v text=%q", c.Range, c.Text)
	}
	old = pad + "x\r\ny"
	next = pad + "xy"
	c = ComputeLSPTextChange(old, next)
	want = LSPRange{Start: LSPPosition{Line: 10, Character: 1}, End: LSPPosition{Line: 11}}
	if c.Range == nil || *c.Range != want || c.Text != "" {
		t.Fatalf("CRLF deletion split line ending: %+v text=%q", c.Range, c.Text)
	}
}

func TestLSPTextChangeUnsafeOrUnprofitableUsesFullReplacement(t *testing.T) {
	pad := strings.Repeat("unchanged prefix\n", 12)
	for _, p := range [][2]string{{"a", "b"}, {"", pad}, {pad, ""}, {pad + "\xff", pad + "valid"}, {pad + "valid", pad + "\xff"}, {pad + "\r", pad + "\rnew"}, {pad + "\rnew", pad + "\r\nnew"}, {pad + "\r\nnew", pad + "\rnew"}} {
		c := ComputeLSPTextChange(p[0], p[1])
		if c.Range != nil || c.Text != p[1] {
			t.Fatalf("unsafe/whole change used range: %+v", c)
		}
	}
	c := ComputeLSPTextChange(pad, pad)
	if c.Range == nil || *c.Range != (LSPRange{}) || c.Text != "" {
		t.Fatal("same text lost cheap version-only event", c)
	}
	next := pad + "x"
	if n := testing.AllocsPerRun(100, func() { _ = ComputeLSPTextChange(pad, next) }); n > 1 {
		t.Fatalf("diff allocated %v objects; no source-sized buffers expected", n)
	}
}

func TestLSPIncrementalSyncCapabilityAndWireShape(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`{"textDocumentSync":2}`, true}, {`{"textDocumentSync":{"openClose":true,"change":2}}`, true},
		{`{"textDocumentSync":2,"positionEncoding":"utf-16"}`, true},
		{`{}`, false}, {`{"textDocumentSync":0}`, false}, {`{"textDocumentSync":1}`, false},
		{`{"textDocumentSync":null}`, false}, {`{"textDocumentSync":true}`, false}, {`{"textDocumentSync":[]}`, false},
		{`{"textDocumentSync":2.0}`, false}, {`{"textDocumentSync":"2"}`, false}, {`{"textDocumentSync":{"change":"2"}}`, false},
		{`{"textDocumentSync":2,"positionEncoding":"utf-8"}`, false}, {`{"textDocumentSync":2,"positionEncoding":7}`, false},
		{`{"textDocumentSync":2,"positionEncoding":""}`, false}, {`{"textDocumentSync":2,"positionEncoding":null}`, false},
		{`{"TextDocumentSync":2}`, false}, {`{"textDocumentSync":{"Change":2}}`, false},
		{`{"textDocumentSync":2,"PositionEncoding":"utf-8"}`, true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			c, buf := advOfflineClient()
			defer c.Close()
			c.storeServerCapabilities(json.RawMessage(`{"capabilities":` + tc.raw + `}`))
			if c.SupportsIncrementalSync() != tc.want {
				t.Fatal("capability mismatch", tc.raw)
			}
			previous := strings.Repeat("// retained 😀\r\n", 20)
			next := previous + "var X=1\r\n"
			if err := c.DidChangeFrom("file:///a.go", 9, previous, next); err != nil {
				t.Fatal(err)
			}
			frame := advReadFrame(t, buf.String())
			var params lspContentChangeParams
			if err := json.Unmarshal(frame.Params, &params); err != nil {
				t.Fatal(err)
			}
			if frame.Method != "textDocument/didChange" || params.TextDocument.URI != "file:///a.go" || params.TextDocument.Version != 9 || len(params.ContentChanges) != 1 {
				t.Fatal("wrong wire envelope", string(frame.Params))
			}
			change := params.ContentChanges[0]
			if (change.Range != nil) != tc.want || applyComputedLSPChange(t, previous, change) != next {
				t.Fatal("wire change differs", change)
			}
			// The old SDK API remains an unconditional full replacement even when
			// an incremental server was negotiated.
			buf.Reset()
			if err := c.DidChange("file:///a.go", 10, next); err != nil {
				t.Fatal(err)
			}
			frame = advReadFrame(t, buf.String())
			params = lspContentChangeParams{} // omitted fields do not clear a reused JSON destination
			if err := json.Unmarshal(frame.Params, &params); err != nil {
				t.Fatal(err)
			}
			if len(params.ContentChanges) != 1 || !reflect.DeepEqual(params.ContentChanges[0], LSPContentChange{Text: next}) {
				t.Fatal("legacy DidChange shape changed", string(frame.Params))
			}
		})
	}
}

func FuzzLSPTextChangeRoundTrip(f *testing.F) {
	pad := strings.Repeat("// padding 😀 中文\r\n", 8)
	for _, source := range []string{"", pad, pad + "é", pad + "😀", pad + "\r\n", pad + "\xff"} {
		f.Add(source, uint16(0), uint16(len(source)), "new")
		f.Add(source, uint16(len(source)), uint16(len(source)), "😀")
	}
	f.Fuzz(func(t *testing.T, previous string, a, b uint16, replacement string) {
		if len(previous) > 8192 || len(replacement) > 256 {
			return
		}
		start, end := int(a)%(len(previous)+1), int(b)%(len(previous)+1)
		if start > end {
			start, end = end, start
		}
		next := previous[:start] + replacement + previous[end:]
		c := ComputeLSPTextChange(previous, next)
		if got := applyComputedLSPChange(t, previous, c); got != next {
			t.Fatalf("diff not exact: %q -> %q got %q", previous, next, got)
		}
	})
}

func BenchmarkLSPTextChangeJSON(b *testing.B) {
	previous := "package main\r\n" + strings.Repeat("func Item(x int) int { return x }\r\n", 5000) + "// editing "
	next := previous + "a"
	for _, incremental := range []bool{false, true} {
		name := "full"
		if incremental {
			name = "incremental"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(next)))
			wireBytes := 0
			for b.Loop() {
				change := LSPContentChange{Text: next}
				if incremental {
					change = ComputeLSPTextChange(previous, next)
				}
				raw, err := json.Marshal(lspContentChangeParams{TextDocument: lspChangedDocument{URI: "file:///large.go", Version: 2}, ContentChanges: []LSPContentChange{change}})
				if err != nil {
					b.Fatal(err)
				}
				wireBytes = len(raw)
			}
			b.ReportMetric(float64(wireBytes), "wire-B/op")
		})
	}
}

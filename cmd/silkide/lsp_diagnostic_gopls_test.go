package main

import (
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
)

func TestRealGoplsDiagnosticUsesUnsavedUnicodeColumn(t *testing.T) {
	bin := requireRealGopls(t)
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module diagnosticqa\n\ngo 1.26.11\n")
	const disk = "package diagnosticqa\r\nfunc probe() {}\r\n"
	path := writeFile(t, dir, "source.go", disk)
	uri := fileURIOf(path)

	client, err := core.LaunchLSPClient(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.Initialize(core.LSPInitializeParams{
		ProcessID: os.Getpid(), RootURI: fileURIOf(dir), Capabilities: ideLSPCapabilities(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.DidOpen(uri, "go", disk, 1); err != nil {
		t.Fatal(err)
	}
	const prefix = "func probe() { /*😀🧪中*/ _ = "
	const missing = "MissingDiagnosticQA"
	const unsaved = "package diagnosticqa\r\n" + prefix + missing + " }\r\n"
	ed := gui.NewCodeEditor()
	ed.SetText(disk)
	ed.ReplaceAllText(unsaved)
	if err := client.DidChange(uri, 2, unsaved); err != nil {
		t.Fatal(err)
	}
	wire := len(utf16.Encode([]rune(prefix)))
	column := len([]rune(prefix))
	if wire <= column {
		t.Fatal("fixture did not distinguish UTF-16 and rune columns")
	}

	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case m, ok := <-client.Notifications():
			if !ok {
				t.Fatal("gopls closed before publishing the unsaved diagnostic")
			}
			if m == nil || m.Method != "textDocument/publishDiagnostics" {
				continue
			}
			var p struct {
				URI         string
				Diagnostics []struct {
					Range   struct{ Start core.LSPPosition }
					Message string
				}
			}
			if err := core.DecodeParams(m, &p); err != nil {
				t.Fatal(err)
			}
			if uriToPath(p.URI) != uriToPath(uri) {
				continue
			}
			for _, d := range p.Diagnostics {
				if !strings.Contains(d.Message, missing) {
					continue
				}
				if d.Range.Start.Line != 1 || d.Range.Start.Character != wire {
					t.Fatalf("reported position=%+v, want line 1, UTF-16 column %d", d.Range.Start, wire)
				}
				rows := []ged.Problem{{Line: d.Range.Start.Line + 1, Col: d.Range.Start.Character, Message: d.Message}}
				normalizeLSPProblemColumns(rows, ed)
				if rows[0].Col != column {
					t.Fatalf("diagnostic column=%d, want %d", rows[0].Col, column)
				}
				got, err := os.ReadFile(path)
				if err != nil || string(got) != disk || !ed.Modified() {
					t.Fatalf("the fixture was not unsaved: file=%q, err=%v, modified=%v", got, err, ed.Modified())
				}
				t.Logf("bundled gopls diagnostic position %d UTF-16 units -> %d runes on unsaved CRLF input", wire, column)
				return
			}
		case <-timeout.C:
			t.Fatal("no diagnostic for the unsaved Unicode source")
		}
	}
}

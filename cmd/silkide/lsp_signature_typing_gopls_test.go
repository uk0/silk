package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func TestGoplsSignatureTracksBurstEdits(t *testing.T) {
	bin := requireRealGopls(t)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	const call = "    _ = 𐐀qa"
	const text = "package main\n\nfunc 𐐀qa(count int, 名称 string) int { return count }\nfunc main() {\n    label := \"valve\"\n" + call + "\n}\n"
	for name, content := range map[string]string{"go.mod": "module example.test/silksigburst\n\ngo 1.26\n", "main.go": text} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	client, err := core.LaunchLSPClient(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	go func() {
		for range client.Notifications() {
		}
	}()
	if _, err := client.Initialize(core.LSPInitializeParams{ProcessID: os.Getpid(), RootURI: fileURIOf(root), Capabilities: ideLSPCapabilities()}); err != nil {
		t.Fatal(err)
	}
	type result struct {
		request lspSnapshot
		hint    core.LSPActiveSignature
	}
	got := make(chan result, 8)
	d := newLSPDocumentWithSignatures(client, fileURIOf(path), text, nil, nil, func(s lspSnapshot, h core.LSPActiveSignature) { got <- result{s, h} })
	t.Cleanup(func() { d.close(); waitDocument(t, d) })
	ed := gui.NewCodeEditor()
	ed.SetFilePath(path)
	ed.SetText(text)
	ed.NavigateToLineCol(5, len([]rune(call)))
	t.Cleanup(func() { ed.CancelCompletion(); ed.CancelSymbolIndex(); ed.CancelGitStatus() })
	ver := 1
	ed.SigChanged(func(source string) {
		ver++
		epoch, intent := ed.SignatureHelpIntent()
		d.update(lspSnapshot{text: source, version: ver, line: ed.CursorLine(), col: ed.CursorUTF16Col(), signature: intent, signatureEpoch: epoch})
	})
	for _, s := range []string{"(", "1", "2", ",", "l", "a", "b", "e", "l"} {
		ed.OnTextInput(s)
	}
	r := waitSignatureTest(t, got)
	h := r.hint
	if r.request.version != ver || r.request.col != ed.CursorUTF16Col() || !strings.Contains(r.request.text, "𐐀qa(12,label)") {
		t.Fatalf("not the latest unsaved typing snapshot: %+v", r.request)
	}
	if h.ParameterStart >= h.ParameterEnd || h.Label[h.ParameterStart:h.ParameterEnd] != "名称 string" {
		t.Fatalf("gopls did not select the current parameter: %+v", h)
	}
	disk, err := os.ReadFile(path)
	if err != nil || string(disk) != text {
		t.Fatalf("test unexpectedly wrote disk: %v", err)
	}
}

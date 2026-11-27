package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/uk0/silk/core"
)

func TestGoplsSignatureHighlightsActiveUnicodeParameter(t *testing.T) {
	bin := requireRealGopls(t)
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	call := "func main() { 𐐀qa(1, \"active\") }"
	src := "package main\n\nfunc 𐐀qa(count int, 名称 string) {}\n" + call + "\n"
	for name, content := range map[string]string{"go.mod": "module example.test/paramqa\n\ngo 1.26\n", "main.go": src} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c, err := core.LaunchLSPClient(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	go func() {
		for range c.Notifications() {
		}
	}()
	if _, err = c.Initialize(core.LSPInitializeParams{ProcessID: os.Getpid(), RootURI: fileURIOf(root), Capabilities: ideLSPCapabilities()}); err != nil {
		t.Fatal(err)
	}
	if err := c.DidOpen(fileURIOf(path), "go", src, 1); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		idx  int
		want string
	}{
		{strings.Index(call, "(1") + 1, "count int"},
		{strings.Index(call, ",") + 1, "名称 string"},
	} {
		col := len(utf16.Encode([]rune(call[:tc.idx])))
		h, err := c.SignatureHint(fileURIOf(path), 3, col)
		if err != nil {
			t.Fatal(err)
		}
		if h.ParameterStart < 0 || h.ParameterEnd > len(h.Label) || h.ParameterStart >= h.ParameterEnd ||
			h.Label[h.ParameterStart:h.ParameterEnd] != tc.want {
			t.Fatalf("column %d: %+v, want %q", col, h, tc.want)
		}
	}
}

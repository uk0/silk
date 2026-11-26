package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/uk0/silk/core"
)

func TestRealGoplsIncrementalUnicodeCRLFEdits(t *testing.T) {
	bin := requireRealGopls(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "增量 空间.go")
	uri := fileURIOf(path)
	base := "package demo\r\n" + strings.Repeat("// unchanged padding 😀 中文\r\n", 24) + "var 文 = \"😀\"; var Initial = 1\r\nfunc use(){ _ = Initial }\r\n"
	writeFile(t, dir, "go.mod", "module incrementalqa\n\ngo 1.26.11\n")
	writeFile(t, dir, filepath.Base(path), base)
	c, err := core.LaunchLSPClient(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	go func() {
		for range c.Notifications() {
		}
	}()
	if _, err = c.Initialize(core.LSPInitializeParams{ProcessID: os.Getpid(), RootURI: fileURIOf(dir), Capabilities: ideLSPCapabilities()}); err != nil {
		t.Fatal(err)
	}
	if !c.SupportsIncrementalSync() {
		t.Fatal("real bundled gopls did not negotiate UTF-16 incremental sync", string(c.ServerCapabilities()))
	}
	d := newLSPDocument(c, uri, base, nil, nil)
	defer func() { d.close(); waitDocument(t, d) }()
	if _, err = d.sync(); err != nil {
		t.Fatal(err)
	}
	next := strings.ReplaceAll(base, "Initial", "ChangedOne")
	next = strings.Replace(next, "\"😀\"", "\"中文🧪\"", 1)
	versions := []struct {
		text, name string
		version    int
	}{{next, "ChangedOne", 7}}
	next = strings.ReplaceAll(next, "ChangedOne", "ChangedTwo")
	next = strings.Replace(next, "var 文", "\r\nvar 文", 1)
	versions = append(versions, struct {
		text, name string
		version    int
	}{next, "ChangedTwo", 41})
	next = strings.ReplaceAll(next, "\r\n", "\n")
	next = strings.ReplaceAll(next, "ChangedTwo", "ChangedThree")
	versions = append(versions, struct {
		text, name string
		version    int
	}{next, "ChangedThree", 99})
	for _, s := range versions {
		d.update(lspSnapshot{text: s.text, version: s.version})
		if _, err = d.sync(); err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(s.text, "\n")
		declarationLine, useLine := -1, -1
		var declarationPrefix, usePrefix string
		for i, line := range lines {
			if at := strings.Index(line, "var "+s.name); at >= 0 {
				declarationLine = i
				declarationPrefix = line[:at+4]
			}
			if at := strings.Index(line, "_ = "+s.name); at >= 0 {
				useLine = i
				usePrefix = line[:at+4]
			}
		}
		if declarationLine < 0 || useLine < 0 {
			t.Fatal("fixture lost target")
		}
		defs, err := c.Definition(uri, useLine, len(utf16.Encode([]rune(usePrefix))))
		if err != nil {
			t.Fatal(err)
		}
		if len(defs) != 1 || defs[0].URI != uri || defs[0].Range.Start.Line != declarationLine || defs[0].Range.Start.Character != len(utf16.Encode([]rune(declarationPrefix))) {
			t.Fatalf("stale/wrong UTF-16 definition v%d: %+v", s.version, defs)
		}
		symbols, err := c.DocumentSymbol(uri)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, symbol := range symbols {
			if symbol.Name == s.name {
				found = true
				if symbol.Line != declarationLine {
					t.Fatal("stale symbol coordinate", symbol)
				}
			}
			if symbol.Name == "Initial" {
				t.Fatal("disk baseline leaked into unsaved symbols")
			}
		}
		if !found {
			t.Fatalf("unsaved %q absent: %+v", s.name, symbols)
		}
	}
	// A version-only update sends an empty range, not the full large source.
	d.update(lspSnapshot{text: next, version: 100})
	if _, err = d.sync(); err != nil {
		t.Fatal(err)
	}
	if _, err = c.DocumentSymbol(uri); err != nil {
		t.Fatal(err)
	}
	// Full replacement/empty outline and restoration must rebase the next diff,
	// not silently retain the original document baseline.
	d.update(lspSnapshot{text: "package demo\n", version: 101})
	if _, err = d.sync(); err != nil {
		t.Fatal(err)
	}
	symbols, err := c.DocumentSymbol(uri)
	if err != nil || len(symbols) != 0 {
		t.Fatal("empty replacement not synchronized", symbols, err)
	}
	d.update(lspSnapshot{text: base, version: 102})
	if _, err = d.sync(); err != nil {
		t.Fatal(err)
	}
	symbols, err = c.DocumentSymbol(uri)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range symbols {
		if s.Name == "Initial" {
			found = true
		}
	}
	if !found {
		t.Fatal("restored source not synchronized", symbols)
	}
	disk, err := os.ReadFile(path)
	if err != nil || string(disk) != base {
		t.Fatal("unsaved sync modified disk", err)
	}
	t.Log("real gopls consumed coalesced UTF-16/CRLF and LF edits, empty replacement and restoration; disk unchanged")
}

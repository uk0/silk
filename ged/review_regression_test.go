package ged

import (
	"context"
	"errors"
	"github.com/uk0/silk/gui"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildOutputPreservesOtherDiagnosticSources(t *testing.T) {
	p := NewProblemsPanel()
	for _, src := range []ProblemSource{{Kind: ProblemLanguage, Doc: "a.go"}, {Kind: ProblemLanguage, Doc: "b.go"}, {Kind: ProblemMacro, Doc: "a.silkgo"}, LoadSource("a.silkui")} {
		p.SetProblems(MergeProblems(p.Problems(), src, []Problem{{Message: "persistent"}}))
	}
	p.SetOutput("build.go:4:2: broken")
	if len(p.Problems()) != 5 {
		t.Fatal(p.Problems())
	}
	p.SetOutput("silkgen: wrote a.go")
	if len(p.Problems()) != 4 {
		t.Fatal("build cleared LSP/macro/load diagnostics", p.Problems())
	}
}

// Reader without WriterTo forces io.Copy's destination ReaderFrom path, which
// used to bypass the cap via an embedded bytes.Buffer.ReadFrom.
type explorerPlainReader struct{ io.Reader }

func TestExplorerOutputCapCannotBeBypassed(t *testing.T) {
	out := explorerOutput{limit: 32}
	_, err := io.Copy(&out, explorerPlainReader{strings.NewReader(strings.Repeat("x", 4096))})
	if !errors.Is(err, io.ErrShortBuffer) || out.Len() > 32 {
		t.Fatalf("unbounded output: %d %v", out.Len(), err)
	}
	if _, ok := any(&out).(io.ReaderFrom); ok {
		t.Fatal("ReaderFrom bypass exposed")
	}
}

func TestExplorerDirectoryLinksAreNotOpenableFiles(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "not-traversed.go"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	r := scanExplorer(context.Background(), explorerScanRequest{root: root, expanded: map[string]bool{root: true, link: true}})
	if r.err != nil || r.root == nil || len(r.root.children) != 1 {
		t.Fatalf("%+v", r)
	}
	row := r.root.children[0]
	if !row.isDir || !row.isSymlink || len(row.children) != 0 {
		t.Fatalf("link misclassified: %+v", row)
	}
	e := NewFileExplorer()
	defer e.CancelScan()
	e.rootDir = root
	e.root = r.root
	e.rebuildFlatList()
	e.selectedIdx = 1
	called := false
	e.SigFileOpen(func(string) { called = true })
	e.onDoubleClick(1)
	e.OnKeyDown(gui.KeyEnter, false)
	if called || row.expanded {
		t.Fatal("directory link opened/traversed")
	}
}

func TestTreeOnlyScanPreservesBadgesAndGitGeneration(t *testing.T) {
	e := NewFileExplorer()
	e.post = func(func()) {}
	defer e.CancelScan()
	e.rootDir = t.TempDir()
	e.root = &fileEntry{path: e.rootDir, isDir: true, expanded: true}
	e.rebuildFlatList()
	e.gitStatus = map[string]rune{filepath.Join(e.rootDir, "a.go"): 'M'}
	e.gitGeneration = 11
	e.scanGeneration = 7
	e.applyScan(explorerScanResult{generation: 7, root: e.root})
	if len(e.gitStatus) != 1 {
		t.Fatal("tree cleared git badges")
	}
	e.setExpanded(e.root, false)
	if e.gitGeneration != 11 || e.gitWorker != nil || len(e.gitStatus) != 1 {
		t.Fatal("expansion launched or cancelled git")
	}
	// A result started before the tree expansion remains current for Git.
	e.applyScan(explorerScanResult{generation: 11, gitOnly: true, gitStatus: map[string]rune{"new": 'A'}})
	if e.gitStatus["new"] != 'A' {
		t.Fatal("tree expansion retired independent git result")
	}
}

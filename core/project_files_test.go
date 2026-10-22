package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uk0/silk/diagnostic"
)

func projectTestFile(t *testing.T, dir, name string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte("fixture"), 0644); e != nil {
		t.Fatal(e)
	}
}
func TestProjectFilesBoundedAndTyped(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"main.go", "中文.silkgo", ".hidden.go", ".git/config", "vendor/a.go", "image.PNG", "sub/b.go"} {
		projectTestFile(t, root, name)
	}
	result, err := ScanProjectFiles(context.Background(), root, ProjectScanOptions{ExcludeDirs: []string{"vendor"}, ExcludeExtensions: []string{".png"}})
	if err != nil || result.Incomplete || len(result.Files) != 3 {
		t.Fatalf("scan=%+v err=%v", result, err)
	}
	for _, f := range result.Files {
		if !filepath.IsAbs(f.Path) || filepath.Join(result.Root, f.Relative) != f.Path {
			t.Fatal(f)
		}
	}
	limited, err := ScanProjectFiles(context.Background(), root, ProjectScanOptions{MaxEntries: 2})
	if diagnostic.CodeOf(err) != diagnostic.FileScanLimit || !limited.Incomplete || limited.Visited != 2 {
		t.Fatalf("unreported limit: %+v %v", limited, err)
	}
	_, err = ScanProjectFiles(context.Background(), filepath.Join(root, "missing"), ProjectScanOptions{})
	if diagnostic.CodeOf(err) != diagnostic.FileScan || !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}
func TestProjectFilesDepthAndHiddenOptions(t *testing.T) {
	root := t.TempDir()
	projectTestFile(t, root, "a/b/c/file.go")
	projectTestFile(t, root, ".hidden.go")
	r, e := ScanProjectFiles(context.Background(), root, ProjectScanOptions{MaxDepth: 1, IncludeHidden: true})
	if diagnostic.CodeOf(e) != diagnostic.FileScanLimit || !r.Incomplete || len(r.Files) != 1 || r.Files[0].Relative != ".hidden.go" {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestProjectFilesNeverFollowsChildSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	projectTestFile(t, root, "inside.go")
	projectTestFile(t, outside, "outside.go")
	if e := os.Symlink(outside, filepath.Join(root, "escape")); e != nil {
		t.Skipf("symlinks unavailable: %v", e)
	}
	if e := os.Symlink(filepath.Join(outside, "outside.go"), filepath.Join(root, "linked.go")); e != nil {
		t.Fatal(e)
	}
	r, e := ScanProjectFiles(context.Background(), root, ProjectScanOptions{})
	if e != nil || len(r.Files) != 1 || r.Files[0].Relative != "inside.go" {
		t.Fatalf("symlink indexed: %+v %v", r, e)
	}
}
func TestProjectFilesCancellationAndRelativeRoot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, e := ScanProjectFiles(ctx, "does-not-exist", ProjectScanOptions{})
	if !errors.Is(e, context.Canceled) || !r.Incomplete || r.Visited != 0 {
		t.Fatalf("%+v %v", r, e)
	}
	root := t.TempDir()
	projectTestFile(t, root, "main.go")
	t.Chdir(root) // TempDir and checkout may be on different Windows drives.
	cwd, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	relative, e := filepath.Rel(cwd, root)
	if e != nil {
		t.Fatal(e)
	}
	r, e = ScanProjectFiles(context.Background(), relative, ProjectScanOptions{})
	if e != nil || len(r.Files) != 1 || !filepath.IsAbs(r.Files[0].Path) {
		t.Fatalf("%+v %v", r, e)
	}
}

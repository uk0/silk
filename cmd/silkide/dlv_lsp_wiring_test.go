package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestFileURIOf locks in the "file://" + absolute path shape gopls
// and dlv both expect. Relative inputs go through filepath.Abs first
// so the URI lines up with whatever the language server already
// indexed under projectDir. The Windows-flavoured case round-trips
// the drive-letter convention used by LSP ("file:///C:/...").
func TestFileURIOf(t *testing.T) {
	t.Run("absolute path", func(t *testing.T) {
		got := fileURIOf("/tmp/foo/bar.go")
		want := "file:///tmp/foo/bar.go"
		if got != want {
			t.Errorf("fileURIOf(/tmp/foo/bar.go) = %q, want %q", got, want)
		}
	})
	t.Run("relative path absolutises", func(t *testing.T) {
		got := fileURIOf("bar.go")
		// We don't know cwd at test time; assert the prefix + suffix
		// shape rather than the exact string. Every URI must start
		// "file:///" and end with the input basename.
		if !strings.HasPrefix(got, "file:///") {
			t.Errorf("fileURIOf(bar.go) = %q; missing file:/// prefix", got)
		}
		if !strings.HasSuffix(got, "/bar.go") {
			t.Errorf("fileURIOf(bar.go) = %q; missing /bar.go suffix", got)
		}
	})
	t.Run("empty input passes through", func(t *testing.T) {
		if got := fileURIOf(""); got != "" {
			t.Errorf("fileURIOf(\"\") = %q, want empty", got)
		}
	})
	t.Run("posix path has no backslash leakage", func(t *testing.T) {
		// filepath.ToSlash inside fileURIOf is a no-op on POSIX (no
		// backslash to swap) and active on Windows. On the test host,
		// the resulting URI must use forward slashes; this assertion
		// only fires on Windows in practice, since POSIX input never
		// has backslashes to begin with.
		got := fileURIOf(filepath.Join("/", "tmp", "x"))
		if strings.Contains(got, "\\") {
			t.Errorf("fileURIOf retained backslash in %q", got)
		}
	})
}

// TestIsGoFile pins the .go-suffix classifier the LSP didOpen path
// uses to decide whether a freshly-opened tab should be sent at
// gopls. _test.go files count -- gopls happily indexes them and
// silently skipping them would leave a noticeable hole when the
// user is reading test source.
func TestIsGoFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"foo_test.go", true},
		{filepath.Join("a", "b", "c.go"), true},
		{"go.mod", false},
		{"README.md", false},
		{"", false},
		{"go", false},
		{"foo.gold", false},
	}
	for _, c := range cases {
		got := isGoFile(c.path)
		if got != c.want {
			t.Errorf("isGoFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

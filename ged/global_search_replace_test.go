package ged

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReplaceInContent exercises the pure replacement core that ReplaceAll
// builds on. It mirrors the search matching semantics: case-insensitive mode
// matches what GlobalSearchPanel.Search finds.
func TestReplaceInContent(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		query       string
		replacement string
		ci          bool // case-insensitive
		want        string
		wantCount   int
	}{
		{
			name:      "zero matches leaves content unchanged",
			content:   "package main\nfunc main() {}\n",
			query:     "absent",
			want:      "package main\nfunc main() {}\n",
			wantCount: 0,
		},
		{
			name:        "empty query is a no-op",
			content:     "hello",
			query:       "",
			replacement: "x",
			want:        "hello",
			wantCount:   0,
		},
		{
			name:        "multiple matches on one line",
			content:     "foo foo foo",
			query:       "foo",
			replacement: "bar",
			want:        "bar bar bar",
			wantCount:   3,
		},
		{
			name:        "matches across multiple lines",
			content:     "foo\nbar foo\nfoo baz\n",
			query:       "foo",
			replacement: "X",
			want:        "X\nbar X\nX baz\n",
			wantCount:   3,
		},
		{
			name:        "case-insensitive matches mixed casing and preserves surrounding text",
			content:     "Foo FOO foo fOo",
			query:       "foo",
			replacement: "bar",
			ci:          true,
			want:        "bar bar bar bar",
			wantCount:   4,
		},
		{
			name:        "case-sensitive only replaces exact casing",
			content:     "Foo FOO foo",
			query:       "foo",
			replacement: "bar",
			ci:          false,
			want:        "Foo FOO bar",
			wantCount:   1,
		},
		{
			name:        "empty replacement deletes the match",
			content:     "abXYZcd XYZ",
			query:       "XYZ",
			replacement: "",
			want:        "abcd ",
			wantCount:   2,
		},
		{
			name:        "replacement containing the query is not re-matched (case-insensitive)",
			content:     "cat cat",
			query:       "cat",
			replacement: "concatenate",
			ci:          true,
			want:        "concatenate concatenate",
			wantCount:   2,
		},
		{
			name:        "replacement containing the query is not re-matched (case-sensitive)",
			content:     "ab ab",
			query:       "ab",
			replacement: "xabx",
			ci:          false,
			want:        "xabx xabx",
			wantCount:   2,
		},
		{
			name:        "overlapping potential matches are handled non-overlapping",
			content:     "aaaa",
			query:       "aa",
			replacement: "b",
			want:        "bb",
			wantCount:   2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, count := replaceInContent(tc.content, tc.query, tc.replacement, tc.ci)
			if got != tc.want {
				t.Errorf("content = %q, want %q", got, tc.want)
			}
			if count != tc.wantCount {
				t.Errorf("count = %d, want %d", count, tc.wantCount)
			}
		})
	}
}

// TestReplaceInContentNoChangeReturnsOriginal verifies that when nothing
// matches the returned string is the unmodified original (count 0), which is
// what ReplaceAll relies on to skip writing untouched files.
func TestReplaceInContentNoChangeReturnsOriginal(t *testing.T) {
	const content = "the quick brown fox"
	got, count := replaceInContent(content, "cat", "dog", true)
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
	if got != content {
		t.Fatalf("content = %q, want unchanged %q", got, content)
	}
}

// TestReplaceAllRewritesMatchingFiles is an integration test: it points a
// headless GlobalSearchPanel at a temp dir, runs a search + Replace All, and
// asserts that files with matches are rewritten (case-insensitively, like the
// search) while non-matching and non-.go files are left byte-identical.
func TestReplaceAllRewritesMatchingFiles(t *testing.T) {
	dir := t.TempDir()

	// Two .go files that contain the query (in different casings).
	a := filepath.Join(dir, "a.go")
	b := filepath.Join(dir, "b.go")
	// A .go file with no match — must stay byte-identical.
	noMatch := filepath.Join(dir, "nomatch.go")
	// A non-.go file containing the query — search ignores it, so it must
	// stay byte-identical too.
	txt := filepath.Join(dir, "notes.txt")

	aContent := "var Widget = 1\n// widget reference\n"
	bContent := "type WIDGET struct{}\n"
	noMatchContent := "package main\nfunc main() {}\n"
	txtContent := "this widget should not be touched\n"

	writeFile(t, a, aContent)
	writeFile(t, b, bContent)
	writeFile(t, noMatch, noMatchContent)
	writeFile(t, txt, txtContent)

	p := NewGlobalSearchPanel()
	p.SetRootDir(dir)

	// Search is case-insensitive, so "widget" matches Widget, widget, WIDGET.
	p.Search("widget")
	if got := p.totalMatchCount(); got != 3 {
		t.Fatalf("pre-replace match count = %d, want 3", got)
	}
	if got := p.totalFileCount(); got != 2 {
		t.Fatalf("pre-replace file count = %d, want 2", got)
	}

	// Set replace text and run Replace All.
	p.replaceRunes = []rune("Gadget")
	p.ReplaceAll()

	// After replacing, the rewritten files no longer contain "widget" in any
	// casing, so the refreshed search must report zero matches.
	if got := p.totalMatchCount(); got != 0 {
		t.Fatalf("post-replace match count = %d, want 0", got)
	}

	assertFileEquals(t, a, "var Gadget = 1\n// Gadget reference\n")
	assertFileEquals(t, b, "type Gadget struct{}\n")

	// Untouched files must be byte-for-byte identical.
	assertFileEquals(t, noMatch, noMatchContent)
	assertFileEquals(t, txt, txtContent)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileEquals(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", filepath.Base(path), string(got), want)
	}
}

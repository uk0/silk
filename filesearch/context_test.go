package filesearch

import (
	"context"
	"errors"
	"github.com/uk0/silk/internal/regexscan"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func contextFile(t *testing.T, root, name, text string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestSearchContextExactUnicodeAndAnchors(t *testing.T) {
	path := contextFile(t, t.TempDir(), "a.txt", "中🙂 foo foofoo FOO\r\nfoo\n")
	cases := []struct {
		query string
		opt   ContextOptions
		count int
	}{
		{"foo", ContextOptions{Options: Options{IgnoreCase: true}, WholeWord: true}, 3},
		{`^foo|\bFOO$`, ContextOptions{Options: Options{Regex: true}}, 2},
		{`^`, ContextOptions{Options: Options{Regex: true}}, 0},
		{`foo|foofoo`, ContextOptions{}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			got, err := SearchContext(context.Background(), path, tc.query, tc.opt)
			if err != nil || !got.Complete() || len(got.Matches) != tc.count {
				t.Fatalf("%+v %v", got, err)
			}
			for _, m := range got.Matches {
				if m.Length <= 0 || m.Col < 1 || m.Col-1+m.Length > len(m.Text) {
					t.Fatal(m)
				}
			}
		})
	}
	r, err := SearchContext(context.Background(), path, "foo", ContextOptions{})
	if err != nil || r.Matches[0].Col != len("中🙂 ")+1 || r.Matches[0].Length != 3 {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestSearchContextFiltersAndPartialCoverage(t *testing.T) {
	root := t.TempDir()
	contextFile(t, root, "a.go", "hit\n")
	contextFile(t, root, "generated/g.go", "hit\n")
	contextFile(t, root, "a_test.go", "hit\n")
	contextFile(t, root, "a.txt", "hit\n")
	contextFile(t, root, ".git/config", "hit\n")
	contextFile(t, root, "b.go", "hit\x00binary")
	opt := ContextOptions{Options: Options{Include: []string{"go"}, SkipBinary: true}, Exclude: []string{"generated", "*_test.go"}}
	r, err := SearchContext(context.Background(), root, "hit", opt)
	if err != nil || !r.Complete() || len(r.Matches) != 1 || filepath.Base(r.Matches[0].Path) != "a.go" {
		t.Fatalf("%+v %v", r, err)
	}
	path := contextFile(t, root, "long.txt", strings.Repeat("x", 300000)+"\nhit\n")
	r, err = SearchContext(context.Background(), path, "hit", ContextOptions{})
	if err != nil || r.Complete() || r.SkippedLongLines != 1 || len(r.Matches) != 1 || r.Matches[0].Line != 2 {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = SearchContext(context.Background(), path, "hit", ContextOptions{Limits: SearchLimits{FileBytes: 10}})
	if err != nil || r.Complete() || r.SkippedLargeFiles != 1 || len(r.Matches) != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestSearchContextBudgets(t *testing.T) {
	root := t.TempDir()
	path := contextFile(t, root, "a.txt", strings.Repeat("a ", 10000))
	for _, l := range []SearchLimits{{Matches: 3}, {TotalBytes: 100}, {ResultBytes: 30}} {
		r, err := SearchContext(context.Background(), path, "a", ContextOptions{Limits: l})
		if !errors.Is(err, ErrSearchLimit) || r.Complete() || !r.Truncated {
			t.Fatalf("%+v: %+v %v", l, r, err)
		}
		if l.Matches > 0 && len(r.Matches) != 3 {
			t.Fatal(len(r.Matches))
		}
		if l.TotalBytes > 0 && r.BytesRead > l.TotalBytes+1 {
			t.Fatal("read budget", r.BytesRead)
		}
	}
	for i := 0; i < 4; i++ {
		contextFile(t, root, string(rune('b'+i))+".txt", "a")
	}
	for _, l := range []SearchLimits{{Files: 2}, {Entries: 2}} {
		r, err := SearchContext(context.Background(), root, "a", ContextOptions{Limits: l})
		if !errors.Is(err, ErrSearchLimit) || r.Complete() {
			t.Fatalf("%+v %v", r, err)
		}
	}
	path = contextFile(t, t.TempDir(), "binary", strings.Repeat("\x00", 65536))
	r, err := SearchContext(context.Background(), path, "a", ContextOptions{Options: Options{SkipBinary: true}, Limits: SearchLimits{TotalBytes: 2048}})
	if !errors.Is(err, ErrSearchLimit) || r.BytesRead > 2049 {
		t.Fatalf("binary peek unbounded: %+v %v", r, err)
	}
	root = t.TempDir()
	contextFile(t, root, "d/e/f.txt", "a")
	r, err = SearchContext(context.Background(), root, "a", ContextOptions{Limits: SearchLimits{Depth: 1}})
	if !errors.Is(err, ErrSearchLimit) || !r.Truncated {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestSearchContextErrorsCancellationAndSymlink(t *testing.T) {
	root := t.TempDir()
	path := contextFile(t, root, "a.txt", strings.Repeat("a", 200000))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, err := SearchContext(ctx, path, "a", ContextOptions{})
	if !errors.Is(err, context.Canceled) || len(r.Matches) != 0 {
		t.Fatal(r, err)
	}
	for _, query := range []string{"[", strings.Repeat("a", regexscan.QueryLimit+1), "(?:a{1000}){1000}"} {
		_, err := SearchContext(context.Background(), root, query, ContextOptions{Options: Options{Regex: true}})
		if err == nil {
			t.Fatal("invalid/oversize expression accepted", len(query))
		}
	}
	_, err = SearchContext(context.Background(), root, "a", ContextOptions{Exclude: []string{"["}})
	if err == nil {
		t.Fatal("bad glob accepted")
	}
	_, err = SearchContext(context.Background(), filepath.Join(root, "missing"), "a", ContextOptions{})
	if err == nil {
		t.Fatal("missing root accepted")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	r, err = SearchContext(ctx, path, `a.*z`, ContextOptions{Options: Options{Regex: true}})
	if !errors.Is(err, context.DeadlineExceeded) || len(r.Matches) != 0 {
		t.Fatal(r, err)
	}
	if err = os.Symlink(path, filepath.Join(root, "link.txt")); err != nil {
		t.Log("symlink unavailable:", err)
		return
	}
	if err := os.WriteFile(path, []byte("aa"), 0644); err != nil {
		t.Fatal(err)
	}
	r, err = SearchContext(context.Background(), root, "aa", ContextOptions{})
	if err != nil || r.Files != 1 {
		t.Fatalf("symlink followed: files=%d matches=%d err=%v", r.Files, len(r.Matches), err)
	}
	_, err = SearchContext(context.Background(), filepath.Join(root, "link.txt"), "a", ContextOptions{})
	if err == nil {
		t.Fatal("symlink root accepted")
	}
}
func TestReplaceTextContextExactAndLimits(t *testing.T) {
	for _, tc := range []struct {
		text, query, repl, want string
		regex, word             bool
		n                       int
	}{
		{"中🙂 foo\r\nfoo\nfoo", "foo", "$1中", "中🙂 $1中\r\n$1中\n$1中", false, false, 3},
		{"foo_12\r\nfoo_34", `foo_(?P<id>\d+)`, "${id}_中$$", "12_中$\r\n34_中$", true, false, 2},
		{"foo foobar", "foo", "x", "x foobar", false, true, 1},
		{"abc\r\nabc", `^`, "x", "abc\r\nabc", true, false, 0},
		{"a ab a", `\ba\b`, "x", "x ab x", true, false, 2},
	} {
		got, n, err := ReplaceTextContext(context.Background(), tc.text, tc.query, tc.repl, ContextOptions{Options: Options{Regex: tc.regex}, WholeWord: tc.word}, 1024)
		if got != tc.want || n != tc.n || err != nil {
			t.Fatalf("%q => %q %d %v", tc.query, got, n, err)
		}
	}
	got, n, err := ReplaceTextContext(context.Background(), "aaaa", "a", strings.Repeat("z", 16), ContextOptions{}, 32)
	if !errors.Is(err, ErrReplacementLimit) || got != "" || n != 0 {
		t.Fatal(got, n, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, n, err = ReplaceTextContext(ctx, "a", "a", "b", ContextOptions{}, 32)
	if !errors.Is(err, context.Canceled) || got != "" || n != 0 {
		t.Fatal(got, n, err)
	}
}

package core

import (
	"slices"
	"strings"
	"testing"
)

// 假 runner(gitArgvRecorder / newRecordedGitOps)定义在 gitops_test.go, 同包共用.

func TestGitRefsArgv(t *testing.T) {
	tests := []struct {
		name string
		call func(*GitOps) error
		want []string
	}{
		{"list branches", func(g *GitOps) error { _, err := g.ListBranches(); return err },
			[]string{"branch", "--list", "--all", "--format=" + gitBranchFormat}},
		{"create branch", func(g *GitOps) error { return g.CreateBranch("feature", "") },
			[]string{"branch", "feature"}},
		{"create branch at start point", func(g *GitOps) error { return g.CreateBranch("feature", "origin/main") },
			[]string{"branch", "feature", "origin/main"}},
		{"checkout branch", func(g *GitOps) error { return g.CheckoutBranch("feature") },
			[]string{"checkout", "feature"}},
		{"delete branch safe", func(g *GitOps) error { return g.DeleteBranch("feature", false) },
			[]string{"branch", "-d", "feature"}},
		{"delete branch forced", func(g *GitOps) error { return g.DeleteBranch("feature", true) },
			[]string{"branch", "-D", "feature"}},
		{"list remotes", func(g *GitOps) error { _, err := g.ListRemotes(); return err },
			[]string{"remote", "-v"}},
		{"add remote", func(g *GitOps) error { return g.AddRemote("fork", "git@example.com:me/a.git") },
			[]string{"remote", "add", "fork", "git@example.com:me/a.git"}},
		{"status", func(g *GitOps) error { _, err := g.Status(); return err },
			[]string{"status", "--porcelain=v1", "-z"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, rec := newRecordedGitOps("")
			if err := tc.call(g); err != nil {
				t.Fatalf("call returned error: %v", err)
			}
			rec.wantOneCall(t, tc.want...)
		})
	}
}

// TestGitRefsRejectsBadArgs 分支/远程名的校验: 报错且一条 git 都不发
func TestGitRefsRejectsBadArgs(t *testing.T) {
	tests := []struct {
		name string
		call func(*GitOps) error
	}{
		{"create empty", func(g *GitOps) error { return g.CreateBranch("", "") }},
		{"create blank", func(g *GitOps) error { return g.CreateBranch("   ", "") }},
		{"create dash", func(g *GitOps) error { return g.CreateBranch("--force", "") }},
		{"create dash start point", func(g *GitOps) error { return g.CreateBranch("feature", "-x") }},
		{"checkout empty", func(g *GitOps) error { return g.CheckoutBranch("") }},
		{"checkout dash", func(g *GitOps) error { return g.CheckoutBranch("-f") }},
		{"delete dash", func(g *GitOps) error { return g.DeleteBranch("-D", false) }},
		{"add remote empty name", func(g *GitOps) error { return g.AddRemote("", "https://example.com/a.git") }},
		{"add remote dash name", func(g *GitOps) error { return g.AddRemote("-x", "https://example.com/a.git") }},
		{"add remote empty url", func(g *GitOps) error { return g.AddRemote("fork", "") }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, rec := newRecordedGitOps("")
			if err := tc.call(g); err == nil {
				t.Fatal("want error, got nil")
			}
			if len(rec.calls) != 0 {
				t.Fatalf("git was invoked despite invalid args: %v", rec.calls)
			}
		})
	}
}

// gitBranchLine 按 gitBranchFormat 的字段顺序拼一行, 省得测试里数 \x1f
func gitBranchLine(head, ref, short, hash, upstream, track, subject string) string {
	return strings.Join([]string{head, ref, short, hash, upstream, track, subject}, "\x1f")
}

// TestParseGitBranches 取自真实 `git branch --list --all --format=...` 的输出
// 覆盖: 当前分支标记、上游 + ahead/behind、[gone]、无上游、远程跟踪分支,
// 以及两类必须被跳过的行(refs/remotes/origin/HEAD 符号引用、字段不齐的畸形行).
func TestParseGitBranches(t *testing.T) {
	out := strings.Join([]string{
		gitBranchLine(" ", "refs/heads/behindbr", "behindbr", "3e7c50d", "origin/main", "[ahead 1, behind 1]", "diverge"),
		gitBranchLine(" ", "refs/heads/doomed", "doomed", "d249096", "origin/doomed", "[gone]", "doomed1"),
		gitBranchLine("*", "refs/heads/main", "main", "b80be27", "origin/main", "[ahead 1]", "local-only"),
		gitBranchLine(" ", "refs/heads/plain", "plain", "aaa1111", "", "", "no upstream here"),
		// refs/remotes/<remote>/HEAD 的 refname:short 会缩成 "origin", 必须跳过
		gitBranchLine(" ", "refs/remotes/origin/HEAD", "origin", "6bed392", "", "", "c2"),
		gitBranchLine(" ", "refs/remotes/origin/main", "origin/main", "6bed392", "", "", "c2"),
		"malformed line without separators",
		"",
	}, "\n")

	got := ParseGitBranches(out)
	want := []GitBranch{
		{Name: "behindbr", Ref: "refs/heads/behindbr", Hash: "3e7c50d", Subject: "diverge",
			Upstream: "origin/main", Ahead: 1, Behind: 1},
		{Name: "doomed", Ref: "refs/heads/doomed", Hash: "d249096", Subject: "doomed1",
			Upstream: "origin/doomed", Gone: true},
		{Name: "main", Ref: "refs/heads/main", Hash: "b80be27", Subject: "local-only",
			Upstream: "origin/main", Ahead: 1, Current: true},
		{Name: "plain", Ref: "refs/heads/plain", Hash: "aaa1111", Subject: "no upstream here"},
		{Name: "origin/main", Ref: "refs/remotes/origin/main", Hash: "6bed392", Subject: "c2", Remote: true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d branches, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("branch[%d] =\n %+v\nwant\n %+v", i, got[i], want[i])
		}
	}
}

// TestParseGitBranchesDetached 游离 HEAD 时 git branch 会多出一个伪条目
// "(HEAD detached at abc1234)" —— 它不是引用, 必须被跳过, 且此时没有任何分支是 Current.
func TestParseGitBranchesDetached(t *testing.T) {
	out := strings.Join([]string{
		gitBranchLine("*", "(HEAD detached at 3e7c50d)", "(HEAD detached at 3e7c50d)", "3e7c50d", "", "", "c1"),
		gitBranchLine(" ", "refs/heads/main", "main", "b80be27", "", "", "local-only"),
	}, "\n")

	got := ParseGitBranches(out)
	if len(got) != 1 {
		t.Fatalf("got %d branches, want 1 (pseudo entry skipped): %+v", len(got), got)
	}
	if got[0].Name != "main" || got[0].Current {
		t.Fatalf("branch[0] = %+v, want main with Current=false", got[0])
	}
}

func TestParseGitTrack(t *testing.T) {
	tests := []struct {
		in     string
		ahead  int
		behind int
		gone   bool
	}{
		{"", 0, 0, false},
		{"[ahead 2]", 2, 0, false},
		{"[behind 3]", 0, 3, false},
		{"[ahead 1, behind 2]", 1, 2, false},
		{"[gone]", 0, 0, true},
		{"[ahead 12, behind 34]", 12, 34, false},
		{"[ahead x]", 0, 0, false},   // 数字解析不了就不猜
		{"[]", 0, 0, false},          // 空括号
		{"garbage", 0, 0, false},     // 没有括号
		{"[unknown 5]", 0, 0, false}, // 认不出的关键字
	}
	for _, tc := range tests {
		ahead, behind, gone := parseGitTrack(tc.in)
		if ahead != tc.ahead || behind != tc.behind || gone != tc.gone {
			t.Errorf("parseGitTrack(%q) = (%d,%d,%v), want (%d,%d,%v)",
				tc.in, ahead, behind, gone, tc.ahead, tc.behind, tc.gone)
		}
	}
}

// TestParseGitRemotes 取自真实 `git remote -v`: 同一远程两行(fetch/push),
// 并覆盖 pushurl 与 fetch URL 不同的情况, 以及不带 -v 的纯名字行.
func TestParseGitRemotes(t *testing.T) {
	out := strings.Join([]string{
		"origin\thttps://example.com/a.git (fetch)",
		"origin\thttps://example.com/a.git (push)",
		"fork\tgit@example.com:me/a.git (fetch)",
		"fork\tgit@example.com:me/mirror.git (push)",
		"bare-name",
		"",
	}, "\n")

	got := ParseGitRemotes(out)
	want := []GitRemote{
		{Name: "origin", FetchURL: "https://example.com/a.git", PushURL: "https://example.com/a.git"},
		{Name: "fork", FetchURL: "git@example.com:me/a.git", PushURL: "git@example.com:me/mirror.git"},
		{Name: "bare-name"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d remotes, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("remote[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestParseGitPorcelainZ NUL 分隔形态(GitOps.Status 用的就是它)
// 输入按真实字节序拼: 重命名占两个字段且新路径在前, 未跟踪/冲突各占一个字段,
// 路径原样输出(引号、空格、TAB 都不转义).
func TestParseGitPorcelainZ(t *testing.T) {
	data := " M keep.txt\x00" +
		"R  new name.txt\x00old.txt\x00" + // 重命名: 先新后旧
		"?? un tracked \"q\".txt\x00" + // 未跟踪, 名字里有空格和双引号
		"?? weird\ttab.txt\x00" + // 未跟踪, 名字里有 TAB
		"UU c.txt\x00" + // 冲突(双方都改)
		"A  added-by-them.txt\x00" + // 合并带进来的新文件
		"C  copy.txt\x00source.txt\x00" + // 复制也带旧路径
		"XX\x00" + // 畸形: 长度不足
		"MM|no-space\x00" // 畸形: 第三字节不是空格

	got := ParseGitPorcelainZ(data)
	want := []GitStatusEntry{
		{Staged: ' ', Unstaged: 'M', Path: "keep.txt"},
		{Staged: 'R', Unstaged: ' ', Path: "new name.txt", OrigPath: "old.txt"},
		{Staged: '?', Unstaged: '?', Path: `un tracked "q".txt`},
		{Staged: '?', Unstaged: '?', Path: "weird\ttab.txt"},
		{Staged: 'U', Unstaged: 'U', Path: "c.txt"},
		{Staged: 'A', Unstaged: ' ', Path: "added-by-them.txt"},
		{Staged: 'C', Unstaged: ' ', Path: "copy.txt", OrigPath: "source.txt"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseGitPorcelainZEmpty(t *testing.T) {
	if got := ParseGitPorcelainZ(""); got != nil {
		t.Fatalf("clean worktree should parse to nil, got %+v", got)
	}
}

// TestParseGitPorcelainLines 不带 -z 的换行形态: 重命名是 "old -> new"(顺序与 -z 相反),
// 特殊路径被加引号 + C 风格转义, 必须完整还原.
// 最后一行是关键用例: 旧路径本身含 " -> ", 朴素的 strings.Index 会切错位置.
func TestParseGitPorcelainLines(t *testing.T) {
	out := strings.Join([]string{
		` M keep.txt`,
		`R  old.txt -> "new name.txt"`,
		`?? "un tracked \"q\".txt"`,
		`?? "weird\ttab.txt"`,
		`A  "\346\265\213\350\257\225\346\226\207\344\273\266.txt"`,
		`R  "a -> b.txt" -> c.txt`,
		`UU c.txt`,
		`x`, // 畸形: 太短
		"",
	}, "\n")

	got := ParseGitPorcelainLines(out)
	want := []GitStatusEntry{
		{Staged: ' ', Unstaged: 'M', Path: "keep.txt"},
		{Staged: 'R', Unstaged: ' ', Path: "new name.txt", OrigPath: "old.txt"},
		{Staged: '?', Unstaged: '?', Path: `un tracked "q".txt`},
		{Staged: '?', Unstaged: '?', Path: "weird\ttab.txt"},
		{Staged: 'A', Unstaged: ' ', Path: "测试文件.txt"},
		{Staged: 'R', Unstaged: ' ', Path: "c.txt", OrigPath: "a -> b.txt"},
		{Staged: 'U', Unstaged: 'U', Path: "c.txt"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSplitGitRenameArrow(t *testing.T) {
	tests := []struct {
		in         string
		orig, path string
		ok         bool
	}{
		{`old.txt -> new.txt`, "old.txt", "new.txt", true},
		{`old.txt -> "new name.txt"`, "old.txt", `"new name.txt"`, true},
		// 引号内部的箭头不算分隔符
		{`"a -> b.txt" -> c.txt`, `"a -> b.txt"`, "c.txt", true},
		// 引号内的 \" 是转义的引号, 不能当收尾引号
		{`"a\" -> b.txt" -> c.txt`, `"a\" -> b.txt"`, "c.txt", true},
		{`plain.txt`, "", "", false},
		{`"quoted -> only.txt"`, "", "", false},
	}
	for _, tc := range tests {
		orig, path, ok := splitGitRenameArrow(tc.in)
		if ok != tc.ok || orig != tc.orig || path != tc.path {
			t.Errorf("splitGitRenameArrow(%q) = (%q,%q,%v), want (%q,%q,%v)",
				tc.in, orig, path, ok, tc.orig, tc.path, tc.ok)
		}
	}
}

// TestDecodeGitQuotedPath C 风格引号路径的反转义
// core/git.go 的老实现只剥引号, 于是 "\346\265\213" 会带着反斜杠泄漏出去; 这里必须还原成字节.
func TestDecodeGitQuotedPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain path untouched", `plain.txt`, `plain.txt`},
		{"quotes stripped", `"quoted.txt"`, `quoted.txt`},
		{"escaped tab", `"weird\ttab.txt"`, "weird\ttab.txt"},
		{"escaped newline", `"line\nbreak.txt"`, "line\nbreak.txt"},
		{"escaped quote", `"un tracked \"q\".txt"`, `un tracked "q".txt`},
		{"escaped backslash", `"back\\slash.txt"`, `back\slash.txt`},
		{"bell cr ff vt bs", `"a\ab\bc\fd\re\v"`, "a\x07b\bc\fd\re\v"},
		{"octal utf8", `"\346\265\213\350\257\225.txt"`, "测试.txt"},
		{"octal mixed with ascii", `"dir/\346\265\213-1.txt"`, "dir/测-1.txt"},
		{"unknown escape kept", `"unknown\qescape"`, `unknown\qescape`},
		{"short octal kept", `"\4"`, `\4`},
		{"trailing lone backslash kept", `"trailing\"`, `trailing\`},
		{"unterminated quote untouched", `"noclose.txt`, `"noclose.txt`},
		{"empty quotes", `""`, ``},
		{"empty string", ``, ``},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecodeGitQuotedPath(tc.in); got != tc.want {
				t.Fatalf("DecodeGitQuotedPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestGitOpsStatusParsesRunnerOutput 端到端(但不跑 git): 注入的输出走完
// Status → ParseGitPorcelainZ 这条链, 确认方法真的把 -z 输出交给了 NUL 解析器.
func TestGitOpsStatusParsesRunnerOutput(t *testing.T) {
	g, _ := newRecordedGitOps("R  new name.txt\x00old.txt\x00UU c.txt\x00")
	got, err := g.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	want := []GitStatusEntry{
		{Staged: 'R', Unstaged: ' ', Path: "new name.txt", OrigPath: "old.txt"},
		{Staged: 'U', Unstaged: 'U', Path: "c.txt"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Status() = %+v, want %+v", got, want)
	}
}

// TestGitOpsListBranchesParsesRunnerOutput 同上: ListBranches 走 ParseGitBranches
func TestGitOpsListBranchesParsesRunnerOutput(t *testing.T) {
	out := gitBranchLine("*", "refs/heads/main", "main", "b80be27", "origin/main", "[ahead 2]", "wip") + "\n"
	g, _ := newRecordedGitOps(out)
	got, err := g.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d branches, want 1: %+v", len(got), got)
	}
	if !got[0].Current || got[0].Name != "main" || got[0].Upstream != "origin/main" || got[0].Ahead != 2 {
		t.Fatalf("branch = %+v", got[0])
	}
}

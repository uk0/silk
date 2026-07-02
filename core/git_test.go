package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupRepo 在一个全新的 t.TempDir() 里建一个真实 git 仓库并落一个首提交
// 故意不复用 silk 仓库本身(我们正处在它的某个 worktree 里), 用 TempDir 保证
// 测试与外部仓库状态完全隔离, 不会被嵌套仓库/worktree 状态干扰.
// 返回仓库目录和初始提交里那个文件的相对路径.
func setupRepo(t *testing.T) (dir, file string) {
	t.Helper()
	dir = t.TempDir()

	// init 后把 user.email/user.name 设到本地配置, 否则 commit 会因缺身份失败.
	// 用 git config --local 写进 .git/config, 不污染全局.
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "--local", "user.email", "test@example.com")
	mustGit(t, dir, "config", "--local", "user.name", "Test User")

	file = "hello.txt"
	writeFile(t, dir, file, "line one\nline two\n")
	mustGit(t, dir, "add", file)
	mustGit(t, dir, "commit", "-m", "initial commit")
	return dir, file
}

// mustGit 在 dir 下跑一条 git 命令, 失败即 Fatal(带上合并输出便于排查)
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// writeFile 把内容写到 dir/rel
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestGitAvailable(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	// 在 dev box 上应当可用; 这里只断言不 panic 且返回 true
	if !GitAvailable() {
		t.Fatal("GitAvailable returned false on a box that has git")
	}
}

func TestIsGitRepo(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, _ := setupRepo(t)
	if !IsGitRepo(dir) {
		t.Errorf("IsGitRepo(%q) = false, want true", dir)
	}

	// 一个全新的空 TempDir 不是仓库
	bare := t.TempDir()
	if IsGitRepo(bare) {
		t.Errorf("IsGitRepo(%q) = true for non-repo tempdir, want false", bare)
	}
}

func TestGitCurrentBranch(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, _ := setupRepo(t)
	branch, err := GitCurrentBranch(dir)
	if err != nil {
		t.Fatalf("GitCurrentBranch: %v", err)
	}
	// 不硬编码 main vs master(git 默认分支名因版本/配置而异), 只要非空即可
	if branch == "" {
		t.Error("GitCurrentBranch returned empty branch name")
	}
}

func TestGitDiffHead(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, file := setupRepo(t)

	// 干净工作树: 无 diff
	clean, err := GitDiffHead(dir)
	if err != nil {
		t.Fatalf("GitDiffHead (clean): %v", err)
	}
	if strings.TrimSpace(clean) != "" {
		t.Errorf("GitDiffHead on clean tree = %q, want empty", clean)
	}

	// 改动已跟踪文件后应当出现 diff
	writeFile(t, dir, file, "line one\nline two CHANGED\nline three\n")
	diff, err := GitDiffHead(dir)
	if err != nil {
		t.Fatalf("GitDiffHead (dirty): %v", err)
	}
	if !strings.Contains(diff, "CHANGED") {
		t.Errorf("GitDiffHead output missing changed line; got:\n%s", diff)
	}

	// 集成 sanity: 输出能被已合并的 ParseUnifiedDiff 解析
	files, perr := ParseUnifiedDiff(diff)
	if perr != nil {
		t.Fatalf("ParseUnifiedDiff(GitDiffHead): %v", perr)
	}
	if len(files) != 1 {
		t.Fatalf("ParseUnifiedDiff returned %d files, want 1", len(files))
	}
	if got := files[0].NewPath; got != file {
		t.Errorf("parsed NewPath = %q, want %q", got, file)
	}

	// 单文件 diff 也应当包含改动
	fdiff, err := GitDiffFile(dir, file)
	if err != nil {
		t.Fatalf("GitDiffFile: %v", err)
	}
	if !strings.Contains(fdiff, "CHANGED") {
		t.Errorf("GitDiffFile output missing changed line; got:\n%s", fdiff)
	}
}

func TestGitStatusPorcelain(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, file := setupRepo(t)

	// 改一个已跟踪文件 + 新增一个未跟踪文件
	writeFile(t, dir, file, "line one\nmodified\n")
	writeFile(t, dir, "untracked.txt", "brand new\n")

	entries, err := GitStatusPorcelain(dir)
	if err != nil {
		t.Fatalf("GitStatusPorcelain: %v", err)
	}

	byPath := map[string]GitStatusEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}

	// 已跟踪但未暂存的改动: Y(Unstaged)列应为 'M'
	mod, ok := byPath[file]
	if !ok {
		t.Fatalf("status missing entry for %q; entries=%+v", file, entries)
	}
	if mod.Unstaged != 'M' {
		t.Errorf("entry for %q Unstaged = %q, want 'M'", file, string(mod.Unstaged))
	}

	// 未跟踪文件: 两列都应为 '?'
	un, ok := byPath["untracked.txt"]
	if !ok {
		t.Fatalf("status missing entry for untracked.txt; entries=%+v", entries)
	}
	if un.Staged != '?' || un.Unstaged != '?' {
		t.Errorf("untracked entry = %q%q, want '??'", string(un.Staged), string(un.Unstaged))
	}
}

func TestGitStatusPorcelainRename(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, file := setupRepo(t)

	// 暂存一次重命名, 让 porcelain 输出 "R  old -> new" 形式
	mustGit(t, dir, "mv", file, "renamed.txt")

	entries, err := GitStatusPorcelain(dir)
	if err != nil {
		t.Fatalf("GitStatusPorcelain (rename): %v", err)
	}

	var found bool
	for _, e := range entries {
		if e.Path == "renamed.txt" {
			found = true
			if e.Staged != 'R' {
				t.Errorf("rename entry Staged = %q, want 'R'", string(e.Staged))
			}
			if e.OrigPath != file {
				t.Errorf("rename entry OrigPath = %q, want %q", e.OrigPath, file)
			}
		}
	}
	if !found {
		t.Errorf("status missing renamed.txt entry; entries=%+v", entries)
	}
}

func TestGitShortLog(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, _ := setupRepo(t)

	commits, err := GitShortLog(dir, 5)
	if err != nil {
		t.Fatalf("GitShortLog: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("GitShortLog returned %d commits, want 1", len(commits))
	}
	c := commits[0]
	if c.Subject != "initial commit" {
		t.Errorf("commit Subject = %q, want %q", c.Subject, "initial commit")
	}
	if c.Hash == "" {
		t.Error("commit Hash is empty")
	}
	if c.Author != "Test User" {
		t.Errorf("commit Author = %q, want %q", c.Author, "Test User")
	}
	if c.Date == "" {
		t.Error("commit Date is empty")
	}
}

// isHex40Test 判定 s 是否恰好 40 位十六进制(测试侧独立实现, 不依赖被测私有助手)
func isHex40Test(s string) bool {
	if len(s) != 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func TestGitRevParse(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, _ := setupRepo(t)

	// 首提交之后, HEAD 应能解析成一个 40-hex 完整 SHA
	sha, err := GitRevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("GitRevParse(HEAD): %v", err)
	}
	if !isHex40Test(sha) {
		t.Errorf("GitRevParse(HEAD) = %q, want a 40-hex SHA", sha)
	}

	// 不存在的 ref 应当报错(而非 panic)且不返回 SHA
	if got, err := GitRevParse(dir, "no-such-ref-xyz"); err == nil {
		t.Errorf("GitRevParse on bogus ref returned nil error; got SHA=%q", got)
	}
}

func TestGitShow(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, file := setupRepo(t)

	// HEAD:file 应返回首提交里的内容
	const committed = "line one\nline two\n"
	got, err := GitShow(dir, "HEAD", file)
	if err != nil {
		t.Fatalf("GitShow(HEAD, %q): %v", file, err)
	}
	if got != committed {
		t.Errorf("GitShow(HEAD) = %q, want committed content %q", got, committed)
	}

	// 提交之后改动工作副本: GitShow 仍应返回已提交(旧)内容, 而非工作副本
	writeFile(t, dir, file, "WORKING COPY CHANGED\n")
	got2, err := GitShow(dir, "HEAD", file)
	if err != nil {
		t.Fatalf("GitShow(HEAD, %q) after worktree edit: %v", file, err)
	}
	if got2 != committed {
		t.Errorf("GitShow(HEAD) after edit = %q, want committed content %q (not the working copy)", got2, committed)
	}

	// 不存在的路径应当报错(而非 panic)
	if _, err := GitShow(dir, "HEAD", "no-such-file.txt"); err == nil {
		t.Error("GitShow on missing path returned nil error, want error")
	}
}

func TestGitLogFile(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, file := setupRepo(t)

	// 在该文件上再追加一次提交, 让它有两条历史
	writeFile(t, dir, file, "line one\nline two\nline three\n")
	mustGit(t, dir, "add", file)
	mustGit(t, dir, "commit", "-m", "second commit")

	commits, err := GitLogFile(dir, file, 10)
	if err != nil {
		t.Fatalf("GitLogFile: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("GitLogFile returned %d commits, want 2; got=%+v", len(commits), commits)
	}
	// git log 默认新提交在前
	if commits[0].Subject != "second commit" {
		t.Errorf("commits[0].Subject = %q, want %q", commits[0].Subject, "second commit")
	}
	if commits[1].Subject != "initial commit" {
		t.Errorf("commits[1].Subject = %q, want %q", commits[1].Subject, "initial commit")
	}
	for i, c := range commits {
		if c.Hash == "" {
			t.Errorf("commits[%d].Hash is empty", i)
		}
		if c.Author != "Test User" {
			t.Errorf("commits[%d].Author = %q, want %q", i, c.Author, "Test User")
		}
	}

	// 共享的 parseShortLog 重构不应改变 GitShortLog 的行为:
	// 此仓库现有两条提交, 取 1 条仍应是最新的 "second commit"
	short, err := GitShortLog(dir, 1)
	if err != nil {
		t.Fatalf("GitShortLog after refactor: %v", err)
	}
	if len(short) != 1 {
		t.Fatalf("GitShortLog(1) returned %d commits, want 1", len(short))
	}
	if short[0].Subject != "second commit" {
		t.Errorf("GitShortLog(1)[0].Subject = %q, want %q", short[0].Subject, "second commit")
	}
}

func TestGitBlame(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "--local", "user.email", "test@example.com")
	mustGit(t, dir, "config", "--local", "user.name", "Test User")

	const file = "blame.txt"
	writeFile(t, dir, file, "alpha\nbeta\ngamma\n")
	mustGit(t, dir, "add", file)
	mustGit(t, dir, "commit", "-m", "blame seed")

	lines, err := GitBlame(dir, file)
	if err != nil {
		t.Fatalf("GitBlame: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("GitBlame returned %d lines, want 3; got=%+v", len(lines), lines)
	}

	wantContent := []string{"alpha", "beta", "gamma"}
	for i, bl := range lines {
		if bl.Line != i+1 {
			t.Errorf("lines[%d].Line = %d, want %d (1-based)", i, bl.Line, i+1)
		}
		if bl.Content != wantContent[i] {
			t.Errorf("lines[%d].Content = %q, want %q", i, bl.Content, wantContent[i])
		}
		if bl.Hash == "" {
			t.Errorf("lines[%d].Hash is empty", i)
		}
		if bl.Author != "Test User" {
			t.Errorf("lines[%d].Author = %q, want %q", i, bl.Author, "Test User")
		}
	}
}

func TestRunGitBogusSubcommandErrors(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, _ := setupRepo(t)

	// 超时路径无法稳定地人为触发挂死; 这里验证非零退出被转成 error 而非 panic.
	// runGit 是私有的, 同包测试可直接调用.
	out, err := runGit(dir, "this-is-not-a-real-git-subcommand")
	if err == nil {
		t.Fatalf("runGit with bogus subcommand returned nil error; out=%q", out)
	}
	// error 应当带上命令上下文(stderr 首行被并入)
	if !strings.Contains(err.Error(), "git this-is-not-a-real-git-subcommand") {
		t.Errorf("error missing command context: %v", err)
	}
}

func TestGitStageCommit(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, file := setupRepo(t)

	// 干净工作树: 暂存区应为空
	staged, err := GitHasStagedChanges(dir)
	if err != nil {
		t.Fatalf("GitHasStagedChanges (clean): %v", err)
	}
	if staged {
		t.Fatal("GitHasStagedChanges on clean tree = true, want false")
	}

	// 改一个已跟踪文件并显式暂存它
	writeFile(t, dir, file, "line one\nline two CHANGED\n")
	if err := GitStage(dir, []string{file}); err != nil {
		t.Fatalf("GitStage: %v", err)
	}

	// 暂存后应报告有暂存改动(退出码 1 被映射成 true)
	staged, err = GitHasStagedChanges(dir)
	if err != nil {
		t.Fatalf("GitHasStagedChanges (staged): %v", err)
	}
	if !staged {
		t.Fatal("GitHasStagedChanges after stage = false, want true")
	}

	// 提交, 应返回一个非空短 hash
	hash, err := GitCommitChanges(dir, "second commit")
	if err != nil {
		t.Fatalf("GitCommit: %v", err)
	}

	// 返回的短 hash 应是新 HEAD 完整 SHA 的前缀
	full, err := GitRevParse(dir, "HEAD")
	if err != nil {
		t.Fatalf("GitRevParse(HEAD): %v", err)
	}
	if hash == "" || !strings.HasPrefix(full, hash) {
		t.Errorf("GitCommitChanges hash = %q, want a non-empty prefix of HEAD SHA %q", hash, full)
	}

	// 提交后暂存区应重新为空
	staged, err = GitHasStagedChanges(dir)
	if err != nil {
		t.Fatalf("GitHasStagedChanges (after commit): %v", err)
	}
	if staged {
		t.Error("GitHasStagedChanges after commit = true, want false")
	}

	// 日志最上面一条应是刚才的提交
	commits, err := GitShortLog(dir, 1)
	if err != nil {
		t.Fatalf("GitShortLog: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("GitShortLog returned %d commits, want 1", len(commits))
	}
	if commits[0].Subject != "second commit" {
		t.Errorf("top commit Subject = %q, want %q", commits[0].Subject, "second commit")
	}

	// 工作树现在应当干净(改动已提交)
	entries, err := GitStatusPorcelain(dir)
	if err != nil {
		t.Fatalf("GitStatusPorcelain: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("working tree not clean after commit; entries=%+v", entries)
	}
}

func TestGitUnstage(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, file := setupRepo(t)

	// 改动并暂存
	writeFile(t, dir, file, "line one\nunstage me\n")
	if err := GitStage(dir, []string{file}); err != nil {
		t.Fatalf("GitStage: %v", err)
	}
	staged, err := GitHasStagedChanges(dir)
	if err != nil {
		t.Fatalf("GitHasStagedChanges (staged): %v", err)
	}
	if !staged {
		t.Fatal("expected staged changes before unstage")
	}

	// 取消暂存: 暂存区应清空, 但工作树改动保留
	if err := GitUnstage(dir, []string{file}); err != nil {
		t.Fatalf("GitUnstage: %v", err)
	}
	staged, err = GitHasStagedChanges(dir)
	if err != nil {
		t.Fatalf("GitHasStagedChanges (after unstage): %v", err)
	}
	if staged {
		t.Error("GitHasStagedChanges after unstage = true, want false")
	}

	// 工作树改动应仍在(Unstaged 列为 'M')
	entries, err := GitStatusPorcelain(dir)
	if err != nil {
		t.Fatalf("GitStatusPorcelain: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.Path == file {
			found = true
			if e.Unstaged != 'M' {
				t.Errorf("after unstage %q Unstaged = %q, want 'M'", file, string(e.Unstaged))
			}
		}
	}
	if !found {
		t.Errorf("status missing %q after unstage; entries=%+v", file, entries)
	}
}

func TestGitCommitErrors(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, _ := setupRepo(t)

	// 空信息应快速失败, 不 panic, 且不返回 hash
	if got, err := GitCommitChanges(dir, ""); err == nil {
		t.Errorf("GitCommitChanges with empty message returned nil error; hash=%q", got)
	}
	// 纯空白信息同样按空处理
	if got, err := GitCommitChanges(dir, "   "); err == nil {
		t.Errorf("GitCommitChanges with whitespace message returned nil error; hash=%q", got)
	}

	// 暂存区为空时 git commit 非零退出, 应转成 error 而非 panic
	if got, err := GitCommitChanges(dir, "nothing staged"); err == nil {
		t.Errorf("GitCommitChanges with nothing staged returned nil error; hash=%q", got)
	}
}

func TestGitStageEmptyPaths(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, file := setupRepo(t)

	// 改动一个文件但不暂存
	writeFile(t, dir, file, "line one\nnot staged\n")

	// 空 paths 应是 no-op 并返回 nil —— 不应暂存任何东西
	if err := GitStage(dir, nil); err != nil {
		t.Fatalf("GitStage(nil) = %v, want nil", err)
	}
	if err := GitStage(dir, []string{}); err != nil {
		t.Fatalf("GitStage([]) = %v, want nil", err)
	}

	// 确认没有东西被暂存
	staged, err := GitHasStagedChanges(dir)
	if err != nil {
		t.Fatalf("GitHasStagedChanges: %v", err)
	}
	if staged {
		t.Error("GitStage with empty paths staged something; want no-op")
	}

	// GitUnstage 空 paths 同样是 no-op
	if err := GitUnstage(dir, nil); err != nil {
		t.Fatalf("GitUnstage(nil) = %v, want nil", err)
	}
}

func TestGitStageAll(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not installed")
	}
	dir, file := setupRepo(t)

	// 一个已跟踪文件的修改 + 一个未跟踪的新文件
	writeFile(t, dir, file, "line one\nmodified\n")
	writeFile(t, dir, "brand-new.txt", "fresh\n")

	// 暂存前无暂存改动
	staged, err := GitHasStagedChanges(dir)
	if err != nil {
		t.Fatalf("GitHasStagedChanges (before): %v", err)
	}
	if staged {
		t.Fatal("unexpected staged changes before GitStageAll")
	}

	if err := GitStageAll(dir); err != nil {
		t.Fatalf("GitStageAll: %v", err)
	}

	// 两个文件都应进入暂存区: 已改文件 Staged='M', 新文件 Staged='A'
	entries, err := GitStatusPorcelain(dir)
	if err != nil {
		t.Fatalf("GitStatusPorcelain: %v", err)
	}
	byPath := map[string]GitStatusEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	mod, ok := byPath[file]
	if !ok {
		t.Fatalf("status missing %q after GitStageAll; entries=%+v", file, entries)
	}
	if mod.Staged != 'M' {
		t.Errorf("modified file Staged = %q, want 'M'", string(mod.Staged))
	}
	nw, ok := byPath["brand-new.txt"]
	if !ok {
		t.Fatalf("status missing brand-new.txt after GitStageAll; entries=%+v", entries)
	}
	if nw.Staged != 'A' {
		t.Errorf("new file Staged = %q, want 'A'", string(nw.Staged))
	}

	// 汇总: 应报告有暂存改动
	staged, err = GitHasStagedChanges(dir)
	if err != nil {
		t.Fatalf("GitHasStagedChanges (after): %v", err)
	}
	if !staged {
		t.Error("GitHasStagedChanges after GitStageAll = false, want true")
	}
}

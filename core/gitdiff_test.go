package core

import (
	"reflect"
	"strings"
	"testing"
)

// singleFileSingleHunkDiff 是混合 context/+/- 的标准 unified diff 片段
// 还故意带 "index ..." 行, 以验证元数据被跳过
const singleFileSingleHunkDiff = `diff --git a/foo.go b/foo.go
index abc1234..def5678 100644
--- a/foo.go
+++ b/foo.go
@@ -10,7 +10,9 @@ func Foo() {
 ctx := ctx
+	newLine := 1
+	another := 2
 oldLine := 0
-	removed := -1
 unchangedTail
 final
 last
`

func TestParseUnifiedDiffSingleFileSingleHunk(t *testing.T) {
	files, err := ParseUnifiedDiff(singleFileSingleHunkDiff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}
	f := files[0]
	if f.OldPath != "foo.go" || f.NewPath != "foo.go" {
		t.Errorf("paths = %q/%q, want foo.go/foo.go", f.OldPath, f.NewPath)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(f.Hunks))
	}
	h := f.Hunks[0]
	if h.OldStart != 10 || h.OldCount != 7 || h.NewStart != 10 || h.NewCount != 9 {
		t.Errorf("hunk range = -%d,%d +%d,%d, want -10,7 +10,9",
			h.OldStart, h.OldCount, h.NewStart, h.NewCount)
	}
	// 期望: ctx(C), newLine(A), another(A), oldLine(C), removed(R), unchangedTail(C), final(C), last(C)
	wantKinds := []DiffLineKind{
		DiffLineContext, DiffLineAdded, DiffLineAdded,
		DiffLineContext, DiffLineRemoved,
		DiffLineContext, DiffLineContext, DiffLineContext,
	}
	if len(h.Lines) != len(wantKinds) {
		t.Fatalf("lines = %d, want %d", len(h.Lines), len(wantKinds))
	}
	for i, k := range wantKinds {
		if h.Lines[i].Kind != k {
			t.Errorf("lines[%d].Kind = %v, want %v (text=%q)", i, h.Lines[i].Kind, k, h.Lines[i].Text)
		}
	}
	// 文本去掉了前导标记字符
	if h.Lines[1].Text != "\tnewLine := 1" {
		t.Errorf("added line text = %q, want with tab and no leading +", h.Lines[1].Text)
	}
	if h.Lines[4].Text != "\tremoved := -1" {
		t.Errorf("removed line text = %q", h.Lines[4].Text)
	}
}

const twoFilesTwoHunksDiff = `diff --git a/a.go b/a.go
index 1111111..2222222 100644
--- a/a.go
+++ b/a.go
@@ -1,3 +1,4 @@
 keep1
+addedInA
 keep2
 keep3
@@ -10,2 +11,2 @@
-removedInA
+replacedInA
 tail
diff --git a/b.go b/b.go
index 3333333..4444444 100644
--- a/b.go
+++ b/b.go
@@ -5,2 +5,3 @@
 keepB
+addedInB
 tailB
`

func TestParseUnifiedDiffMultipleFilesAndHunks(t *testing.T) {
	files, err := ParseUnifiedDiff(twoFilesTwoHunksDiff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
	a, b := files[0], files[1]
	if a.NewPath != "a.go" {
		t.Errorf("file[0].NewPath = %q, want a.go", a.NewPath)
	}
	if len(a.Hunks) != 2 {
		t.Errorf("a.go hunks = %d, want 2", len(a.Hunks))
	}
	if b.NewPath != "b.go" {
		t.Errorf("file[1].NewPath = %q, want b.go", b.NewPath)
	}
	if len(b.Hunks) != 1 {
		t.Errorf("b.go hunks = %d, want 1", len(b.Hunks))
	}
	// 第二 hunk 是单删单加: -10,2 +11,2 (两侧都是 2 行)
	h2 := a.Hunks[1]
	if h2.OldStart != 10 || h2.OldCount != 2 || h2.NewStart != 11 || h2.NewCount != 2 {
		t.Errorf("a.go hunk[1] = -%d,%d +%d,%d", h2.OldStart, h2.OldCount, h2.NewStart, h2.NewCount)
	}
	if RemovedLineCount(a) != 1 || RemovedLineCount(b) != 0 {
		t.Errorf("removed counts = %d/%d, want 1/0", RemovedLineCount(a), RemovedLineCount(b))
	}
}

const newFileDiff = `diff --git a/new.go b/new.go
new file mode 100644
index 0000000..abcd123
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+line1
+line2
+line3
`

func TestParseUnifiedDiffNewFile(t *testing.T) {
	files, err := ParseUnifiedDiff(newFileDiff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}
	f := files[0]
	if f.OldPath != "" {
		t.Errorf("OldPath = %q, want empty for /dev/null", f.OldPath)
	}
	if f.NewPath != "new.go" {
		t.Errorf("NewPath = %q, want new.go", f.NewPath)
	}
	// 新建文件的旧侧范围是 0,0
	h := f.Hunks[0]
	if h.OldStart != 0 || h.OldCount != 0 {
		t.Errorf("old range = %d,%d, want 0,0", h.OldStart, h.OldCount)
	}
	if h.NewStart != 1 || h.NewCount != 3 {
		t.Errorf("new range = %d,%d, want 1,3", h.NewStart, h.NewCount)
	}
	for i, ln := range h.Lines {
		if ln.Kind != DiffLineAdded {
			t.Errorf("line[%d].Kind = %v, want Added", i, ln.Kind)
		}
	}
}

const deletedFileDiff = `diff --git a/gone.go b/gone.go
deleted file mode 100644
index abcd123..0000000
--- a/gone.go
+++ /dev/null
@@ -1,2 +0,0 @@
-bye
-cruel
`

func TestParseUnifiedDiffDeletedFile(t *testing.T) {
	files, err := ParseUnifiedDiff(deletedFileDiff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}
	f := files[0]
	if f.OldPath != "gone.go" {
		t.Errorf("OldPath = %q, want gone.go", f.OldPath)
	}
	if f.NewPath != "" {
		t.Errorf("NewPath = %q, want empty for /dev/null", f.NewPath)
	}
	if RemovedLineCount(f) != 2 {
		t.Errorf("removed count = %d, want 2", RemovedLineCount(f))
	}
	// 纯删除文件不应出现在 AddedLinesByFile 输出里
	added := AddedLinesByFile(files)
	if _, exists := added[""]; exists {
		t.Error(`AddedLinesByFile should skip files with empty NewPath`)
	}
	if len(added) != 0 {
		t.Errorf("added map size = %d, want 0", len(added))
	}
}

func TestParseUnifiedDiffHunkWithoutCount(t *testing.T) {
	// "@@ -5 +5,2 @@" 的 -5 部分省略了 ,N, 按规范默认 1
	src := `--- a/x.go
+++ b/x.go
@@ -5 +5,2 @@
-only
+two
+lines
`
	files, err := ParseUnifiedDiff(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || len(files[0].Hunks) != 1 {
		t.Fatalf("structure wrong: files=%d hunks=%d", len(files), len(files[0].Hunks))
	}
	h := files[0].Hunks[0]
	if h.OldStart != 5 || h.OldCount != 1 {
		t.Errorf("old range = %d,%d, want 5,1", h.OldStart, h.OldCount)
	}
	if h.NewStart != 5 || h.NewCount != 2 {
		t.Errorf("new range = %d,%d, want 5,2", h.NewStart, h.NewCount)
	}
}

func TestParseUnifiedDiffNoNewlineAtEnd(t *testing.T) {
	src := `--- a/x.go
+++ b/x.go
@@ -1,2 +1,2 @@
 keep
-old
\ No newline at end of file
+new
\ No newline at end of file
`
	files, err := ParseUnifiedDiff(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || len(files[0].Hunks) != 1 {
		t.Fatalf("structure wrong")
	}
	h := files[0].Hunks[0]
	// 3 个实质行 (keep/-old/+new) + 2 个 NoNewline 标记 = 5 行
	if len(h.Lines) != 5 {
		t.Fatalf("lines = %d, want 5", len(h.Lines))
	}
	noNew := 0
	for _, ln := range h.Lines {
		if ln.Kind == DiffLineNoNewline {
			noNew++
			if ln.Text != "" {
				t.Errorf("NoNewline text should be empty, got %q", ln.Text)
			}
		}
	}
	if noNew != 2 {
		t.Errorf("NoNewline lines = %d, want 2", noNew)
	}
}

func TestParseUnifiedDiffMalformedHunkContinues(t *testing.T) {
	src := `--- a/x.go
+++ b/x.go
@@ NOT-A-HUNK @@
 ignored-context-because-hunk-was-rejected
@@ -1,1 +1,2 @@
 keep
+added
--- a/y.go
+++ b/y.go
@@ -1 +1 @@
-old
+new
`
	files, err := ParseUnifiedDiff(src)
	if err == nil {
		t.Fatal("expected error for malformed @@, got nil")
	}
	if !strings.Contains(err.Error(), "hunk") {
		t.Errorf("err = %v, want it to mention hunk", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2 (parser should continue past malformed @@)", len(files))
	}
	// 第一文件: 只剩下后来那条合法 hunk
	if len(files[0].Hunks) != 1 {
		t.Errorf("file[0].Hunks = %d, want 1 (malformed dropped)", len(files[0].Hunks))
	}
	if files[0].NewPath != "x.go" || files[1].NewPath != "y.go" {
		t.Errorf("paths = %q,%q, want x.go,y.go", files[0].NewPath, files[1].NewPath)
	}
	// 第二文件解析仍然完整
	if len(files[1].Hunks) != 1 || RemovedLineCount(files[1]) != 1 {
		t.Errorf("file[1] hunks=%d removed=%d, want 1/1",
			len(files[1].Hunks), RemovedLineCount(files[1]))
	}
}

func TestParseUnifiedDiffEmptyInput(t *testing.T) {
	files, err := ParseUnifiedDiff("")
	if err != nil {
		t.Errorf("err = %v, want nil for empty input", err)
	}
	if files != nil {
		t.Errorf("files = %v, want nil", files)
	}
	// 仅空白同样视作空
	files, err = ParseUnifiedDiff("   \n\n\t")
	if err != nil {
		t.Errorf("err = %v, want nil for whitespace input", err)
	}
	if files != nil {
		t.Errorf("files = %v, want nil", files)
	}
}

func TestParseUnifiedDiffPrefixStripping(t *testing.T) {
	// 既支持 a/ b/ 前缀 (git 默认), 也支持没有前缀 (其他 diff 工具)
	src := `--- foo/bar.go
+++ foo/bar.go
@@ -1 +1 @@
-old
+new
`
	files, err := ParseUnifiedDiff(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files[0].OldPath != "foo/bar.go" || files[0].NewPath != "foo/bar.go" {
		t.Errorf("paths = %q/%q, want foo/bar.go (no prefix)",
			files[0].OldPath, files[0].NewPath)
	}
}

func TestParseUnifiedDiffPathTimestampStripped(t *testing.T) {
	// 标准 unified diff 允许 path 后接 TAB + 时间戳
	src := "--- a/foo.go\t2024-01-02 03:04:05.000000000 +0000\n" +
		"+++ b/foo.go\t2024-02-02 03:04:05.000000000 +0000\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n"
	files, err := ParseUnifiedDiff(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files[0].OldPath != "foo.go" || files[0].NewPath != "foo.go" {
		t.Errorf("paths = %q/%q, want foo.go", files[0].OldPath, files[0].NewPath)
	}
}

func TestGitDiffAddedLinesByFileFixture(t *testing.T) {
	// 把前面三段拼接, 验证 AddedLinesByFile 输出与 hunk NewStart 一致
	src := singleFileSingleHunkDiff + twoFilesTwoHunksDiff + newFileDiff
	files, err := ParseUnifiedDiff(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := AddedLinesByFile(files)

	// foo.go: @@ -10,7 +10,9; 在新文件视角 newLine=11, another=12
	// 序列: ctx(10) +(11) +(12) ctx(13) -() ctx(14) ctx(15) ctx(16)
	// 注: cursor 从 10 起步, 第一行是 context → 占 10
	want := map[string][]int{
		"foo.go": {11, 12},
		// a.go: hunk1 @@ -1,3 +1,4: keep(1) +addedInA(2) keep(3) keep(4)
		//       hunk2 @@ -10,2 +11,2: -() +(11) ctx(12)
		"a.go": {2, 11},
		// b.go: hunk @@ -5,2 +5,3: keepB(5) +addedInB(6) tailB(7)
		"b.go": {6},
		// new.go: @@ -0,0 +1,3: 三个连续 added → 1,2,3
		"new.go": {1, 2, 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AddedLinesByFile mismatch\n got=%v\nwant=%v", got, want)
	}
}

func TestGitDiffRemovedLineCount(t *testing.T) {
	files, err := ParseUnifiedDiff(singleFileSingleHunkDiff + deletedFileDiff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
	if n := RemovedLineCount(files[0]); n != 1 {
		t.Errorf("foo.go removed = %d, want 1", n)
	}
	if n := RemovedLineCount(files[1]); n != 2 {
		t.Errorf("gone.go removed = %d, want 2", n)
	}
	// 空文件兜底
	if n := RemovedLineCount(DiffFile{}); n != 0 {
		t.Errorf("zero-value removed = %d, want 0", n)
	}
}

func TestParseUnifiedDiffSkipsBinaryAndIndexLines(t *testing.T) {
	// 即便夹杂 "Binary files", "similarity index" 这类不该渲染的元数据,
	// 解析器仍应把后面的真实 hunk 抓出来
	src := `diff --git a/old.png b/new.png
similarity index 100%
rename from old.png
rename to new.png
diff --git a/code.go b/code.go
index abc..def 100644
Binary files a/code.go and b/code.go differ
--- a/code.go
+++ b/code.go
@@ -1 +1 @@
-old
+new
`
	files, err := ParseUnifiedDiff(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 第一个 diff --git 创建了一个空文件 (rename 无 hunk), 第二个真正改了内容
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
	if files[1].NewPath != "code.go" || len(files[1].Hunks) != 1 {
		t.Errorf("code.go file shape wrong: path=%q hunks=%d",
			files[1].NewPath, len(files[1].Hunks))
	}
}

func TestParseUnifiedDiffNoPanicOnGarbage(t *testing.T) {
	// 全是垃圾不应 panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked on garbage input: %v", r)
		}
	}()
	_, _ = ParseUnifiedDiff("@@@\nrandom\n####\n+stray-plus-without-hunk\n")
}

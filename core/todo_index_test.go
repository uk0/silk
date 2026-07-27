package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Go 源码里"长得像注释的字符串字面量"是行级正则的经典假阳性; go/scanner 路径只看
// 注释 token, 所以串内的 `// TODO ...` 必须一条都不产出, 而真注释照旧命中.
func TestTodoScannerSkipsGoStringLiterals(t *testing.T) {
	sc := NewTodoScanner()
	content := "package a\n" + // 1
		"const s = \"// TODO fake marker\"\n" + // 2: 串内诱饵
		"const r = `/* FIXME also fake */`\n" + // 3: 反引号原始串诱饵
		"\n" + // 4
		"// TODO real marker\n" + // 5: 真标记
		"func f() {}\n" // 6

	got := sc.ScanContent("a.go", content)
	want := []TodoItem{
		{File: "a.go", Line: 5, Kind: TodoTODO, Text: "real marker"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScanContent mismatch:\n got  %+v\n want %+v", got, want)
	}
}

// 跨行块注释: 行号要落在标记所在的那一行(注释起始行 + 行内偏移), 并且注释里裸写的
// 标记(无 `//`/`*` 引导)也要识别, 尾部 `*/` 不能留在文本里.
func TestTodoScannerGoBlockComment(t *testing.T) {
	sc := NewTodoScanner()
	content := "package a\n" + // 1
		"/*\n" + // 2
		"FIXME: bare inside block\n" + // 3
		"*/\n" + // 4
		"/* HACK: single line */\n" // 5

	got := sc.ScanContent("a.go", content)
	want := []TodoItem{
		{File: "a.go", Line: 3, Kind: TodoFIXME, Text: "bare inside block"},
		{File: "a.go", Line: 5, Kind: TodoHACK, Text: "single line"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScanContent mismatch:\n got  %+v\n want %+v", got, want)
	}
}

// 多语言注释识别: C 系走 `//` 与 `/* */`, shell/python 系走 `#`, 引导符不属于该
// 语言的注释语法时不命中(C 里的 `#define`, python 里的 `//`), 不认识的后缀不扫描.
func TestTodoScannerMultiLanguage(t *testing.T) {
	sc := NewTodoScanner()
	cases := []struct {
		name    string
		path    string
		content string
		want    []TodoItem
	}{
		{
			name:    "c-like slash slash",
			path:    "main.c",
			content: "int x; // TODO: c marker\n",
			want:    []TodoItem{{File: "main.c", Line: 1, Kind: TodoTODO, Text: "c marker"}},
		},
		{
			name:    "c-like block",
			path:    "app.js",
			content: "let a = 1\n/* FIXME js marker */\n",
			want:    []TodoItem{{File: "app.js", Line: 2, Kind: TodoFIXME, Text: "js marker */"}},
		},
		{
			name:    "python hash",
			path:    "tool.py",
			content: "x = 1\n# BUG: python marker\n",
			want:    []TodoItem{{File: "tool.py", Line: 2, Kind: TodoBUG, Text: "python marker"}},
		},
		{
			name:    "shell hash",
			path:    "run.sh",
			content: "#!/bin/sh\n# XXX shell marker\n",
			want:    []TodoItem{{File: "run.sh", Line: 2, Kind: TodoXXX, Text: "shell marker"}},
		},
		{
			// C 系里 `#` 是预处理指令而不是注释 -> 不算标记
			name:    "hash lead-in rejected in c",
			path:    "main.c",
			content: "# TODO not a c comment\n",
			want:    nil,
		},
		{
			// python 里 `//` 是整除运算符而不是注释 -> 不算标记
			name:    "slash lead-in rejected in python",
			path:    "tool.py",
			content: "n = a // TODO_LIKE\nm = b // NOTE\n",
			want:    nil,
		},
		{
			// 不认识的后缀根本不扫描
			name:    "unknown extension ignored",
			path:    "README.md",
			content: "# TODO ignore markdown\n",
			want:    nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sc.ScanContent(c.path, c.content)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("ScanContent(%s) = %+v, want %+v", c.path, got, c.want)
			}
		})
	}
}

// 标记集可配置: 自定义集合只认自己的标记(默认集里的 TODO 不再命中), 且默认集包含 BUG.
func TestTodoScannerConfigurableTags(t *testing.T) {
	custom := NewTodoScanner("REVIEW", "WIP")
	content := "package a\n// TODO ignored\n// REVIEW: check this\n// WIP later\n"

	got := custom.ScanContent("a.go", content)
	want := []TodoItem{
		{File: "a.go", Line: 3, Kind: TodoKind("REVIEW"), Text: "check this"},
		{File: "a.go", Line: 4, Kind: TodoKind("WIP"), Text: "later"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("custom tags mismatch:\n got  %+v\n want %+v", got, want)
	}

	// 空标记集回落到默认集, 默认集含 BUG.
	def := NewTodoScanner()
	if got := def.ScanContent("a.go", "// BUG: default set covers BUG\n"); len(got) != 1 || got[0].Kind != TodoBUG {
		t.Fatalf("default tags missed BUG: %+v", got)
	}
	if want := DefaultTodoTags(); !reflect.DeepEqual(def.Tags(), want) {
		t.Errorf("Tags() = %v, want %v", def.Tags(), want)
	}
}

// TodoIndex 的增量语义: UpdateFile 覆盖单个文件的桶, 改动后的内容立刻反映到
// ByFile/ByTag/All/Len, 标记清空或 RemoveFile 后该文件彻底消失, 其它文件不受影响.
func TestTodoIndexIncrementalUpdateRemove(t *testing.T) {
	ix := NewTodoIndex()

	ix.UpdateFile("a.go", "package a\n// TODO alpha\n// FIXME beta\n")
	ix.UpdateFile("b.go", "package b\n// TODO gamma\n")

	if got := ix.Len(); got != 3 {
		t.Fatalf("Len after seed = %d, want 3", got)
	}
	if got, want := ix.Files(), []string{"a.go", "b.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Files = %v, want %v", got, want)
	}

	// ByFile: 只看单个文件, 按行号.
	wantA := []TodoItem{
		{File: "a.go", Line: 2, Kind: TodoTODO, Text: "alpha"},
		{File: "a.go", Line: 3, Kind: TodoFIXME, Text: "beta"},
	}
	if got := ix.ByFile("a.go"); !reflect.DeepEqual(got, wantA) {
		t.Fatalf("ByFile(a.go) = %+v, want %+v", got, wantA)
	}

	// ByTag: 跨文件, 按 File 再 Line.
	wantTODO := []TodoItem{
		{File: "a.go", Line: 2, Kind: TodoTODO, Text: "alpha"},
		{File: "b.go", Line: 2, Kind: TodoTODO, Text: "gamma"},
	}
	if got := ix.ByTag(TodoTODO); !reflect.DeepEqual(got, wantTODO) {
		t.Fatalf("ByTag(TODO) = %+v, want %+v", got, wantTODO)
	}
	if got := ix.ByTag(TodoNOTE); len(got) != 0 {
		t.Errorf("ByTag(NOTE) = %+v, want empty", got)
	}

	// 增量更新 a.go: 旧标记必须整桶换掉, 不能留残留.
	ix.UpdateFile("a.go", "package a\n\n\n// NOTE delta\n")
	wantAfter := []TodoItem{
		{File: "a.go", Line: 4, Kind: TodoNOTE, Text: "delta"},
	}
	if got := ix.ByFile("a.go"); !reflect.DeepEqual(got, wantAfter) {
		t.Fatalf("ByFile(a.go) after update = %+v, want %+v", got, wantAfter)
	}
	if got := ix.ByTag(TodoTODO); len(got) != 1 || got[0].File != "b.go" {
		t.Fatalf("ByTag(TODO) after update = %+v, want only b.go", got)
	}
	if got := ix.Len(); got != 2 {
		t.Errorf("Len after update = %d, want 2", got)
	}

	// 标记被删干净的文件不留空桶.
	ix.UpdateFile("a.go", "package a\nfunc f() {}\n")
	if got := ix.Files(); !reflect.DeepEqual(got, []string{"b.go"}) {
		t.Fatalf("Files after clearing a.go = %v, want [b.go]", got)
	}

	// RemoveFile 摘掉最后一个文件 -> 索引空.
	ix.RemoveFile("b.go")
	if got := ix.Len(); got != 0 {
		t.Errorf("Len after RemoveFile = %d, want 0", got)
	}
	if got := ix.All(); len(got) != 0 {
		t.Errorf("All after RemoveFile = %+v, want empty", got)
	}
	// 不存在的文件 RemoveFile 不应 panic.
	ix.RemoveFile("nope.go")
}

// 自定义标记集也要贯穿到索引; ScanDir 播种后仍可继续增量更新.
func TestTodoIndexScanDirThenUpdate(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	py := filepath.Join(dir, "tool.py")

	writeTodoTestFile(t, a, "package a\n// TODO alpha\n")
	writeTodoTestFile(t, py, "x = 1\n# TODO python\n")
	writeTodoTestFile(t, filepath.Join(dir, "README.md"), "# TODO ignored\n")

	ix := NewTodoIndex()
	if err := ix.ScanDir(dir); err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	if got, want := ix.Files(), []string{a, py}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Files after ScanDir = %v, want %v", got, want)
	}

	// 编辑器改了 a.go 但还没落盘: 索引直接吃内容.
	ix.UpdateFile(a, "package a\n// TODO alpha\n// BUG beta\n")
	if got := ix.Len(); got != 3 {
		t.Fatalf("Len after UpdateFile = %d, want 3", got)
	}
	if got := ix.ByTag(TodoBUG); len(got) != 1 || got[0].Line != 3 {
		t.Fatalf("ByTag(BUG) = %+v, want one hit on line 3", got)
	}

	// 文件在磁盘上被删掉 -> RemoveFile 后不再出现在查询里.
	if err := os.Remove(py); err != nil {
		t.Fatalf("remove %s: %v", py, err)
	}
	ix.RemoveFile(py)
	if got := ix.ByFile(py); len(got) != 0 {
		t.Errorf("ByFile(removed) = %+v, want empty", got)
	}

	// 自定义标记集的索引只认自己的标记.
	custom := NewTodoIndex("REVIEW")
	custom.UpdateFile(a, "package a\n// TODO ignored\n// REVIEW: look\n")
	if got := custom.Len(); got != 1 {
		t.Fatalf("custom index Len = %d, want 1", got)
	}
	if got := custom.ByTag(TodoKind("REVIEW")); len(got) != 1 || got[0].Text != "look" {
		t.Fatalf("custom ByTag(REVIEW) = %+v, want one hit with text \"look\"", got)
	}
}

package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// parseTodoLine 的行级启发式: 覆盖命中/未命中两类, 并把已知边界固化成断言.
func TestParseTodoLine(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantKind TodoKind
		wantText string
		wantOK   bool
	}{
		{"todo with colon", "// TODO: fix this", TodoTODO, "fix this", true},
		{"fixme with space", "// FIXME broken", TodoFIXME, "broken", true},
		{"xxx bare indented", "    // XXX", TodoXXX, "", true},
		{"note plain", "// NOTE remember this", TodoNOTE, "remember this", true},
		// 块注释续行以 `*` 引导也应识别
		{"hack block continuation", "* HACK: leftover", TodoHACK, "leftover", true},
		// 小写且无注释引导 -> 不命中(大小写敏感)
		{"lowercase no comment", "x := todo()", "", "", false},
		// 关键字在字符串里且串前无引导符 -> 不命中(需要注释引导)
		{"keyword in string no leadin", `s := "TODO in string"`, "", "", false},
		// 普通代码行 -> 不命中
		{"plain code", "y := a + b", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, text, ok := parseTodoLine(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("parseTodoLine(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if kind != tc.wantKind {
				t.Errorf("parseTodoLine(%q) kind = %q, want %q", tc.line, kind, tc.wantKind)
			}
			if text != tc.wantText {
				t.Errorf("parseTodoLine(%q) text = %q, want %q", tc.line, text, tc.wantText)
			}
		})
	}
}

// ScanTodos 在临时目录上端到端跑一遍: 多个 .go 文件 + 子目录 + vendor/(应跳过)
// + 非 .go 文件(应忽略) + 串内诱饵(不应命中), 断言条目 File/Line/Kind/Text 及排序.
func TestScanTodos(t *testing.T) {
	dir := t.TempDir()

	writeTodoTestFile(t, filepath.Join(dir, "a.go"),
		"package a\n"+
			"// TODO: alpha\n"+
			"func A() {} // FIXME beta\n")

	// b.go: 一个真标记 + 一个字符串诱饵(串前无引导符, 不应命中)
	writeTodoTestFile(t, filepath.Join(dir, "b.go"),
		"package b\n"+
			"const s = \"TODO not a comment\"\n"+
			"// XXX gamma\n")

	// 子目录里的 .go 也要被扫到
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	writeTodoTestFile(t, filepath.Join(subDir, "c.go"),
		"package sub\n"+
			"/* nothing here */\n"+
			"// NOTE delta\n")

	// vendor/ 整棵跳过
	vendorDir := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatalf("mkdir vendor: %v", err)
	}
	writeTodoTestFile(t, filepath.Join(vendorDir, "lib.go"),
		"package lib\n// TODO should be skipped\n")

	// 非 .go 文件忽略
	writeTodoTestFile(t, filepath.Join(dir, "README.md"),
		"# TODO ignore markdown\n")

	got, err := ScanTodos(dir)
	if err != nil {
		t.Fatalf("ScanTodos error: %v", err)
	}

	want := []TodoItem{
		{File: filepath.Join(dir, "a.go"), Line: 2, Kind: TodoTODO, Text: "alpha"},
		{File: filepath.Join(dir, "a.go"), Line: 3, Kind: TodoFIXME, Text: "beta"},
		{File: filepath.Join(dir, "b.go"), Line: 3, Kind: TodoXXX, Text: "gamma"},
		{File: filepath.Join(subDir, "c.go"), Line: 3, Kind: TodoNOTE, Text: "delta"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScanTodos mismatch:\n got  %+v\n want %+v", got, want)
	}
}

// 空目录 -> 空切片, 无错误
func TestScanTodosEmpty(t *testing.T) {
	dir := t.TempDir()
	got, err := ScanTodos(dir)
	if err != nil {
		t.Fatalf("ScanTodos(empty) error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ScanTodos(empty) = %+v, want empty", got)
	}
}

func writeTodoTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

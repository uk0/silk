package core

import (
	"strings"
	"testing"
)

const sampleProfile = `mode: set
silk/ged/foo.go:10.13,15.2 3 1
silk/ged/foo.go:17.2,17.10 1 0
silk/ged/foo.go:20.2,22.5 2 4
silk/ged/bar.go:5.1,10.3 2 5
silk/ged/bar.go:12.2,12.20 1 0
silk/ged/bar.go:30.2,35.4 4 7
`

func TestParseCoverageBasic(t *testing.T) {
	mode, blocks, err := ParseCoverage(sampleProfile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != "set" {
		t.Errorf("mode = %q, want set", mode)
	}
	if len(blocks) != 6 {
		t.Fatalf("blocks count = %d, want 6", len(blocks))
	}
	// 抽查第一个
	b0 := blocks[0]
	if b0.File != "silk/ged/foo.go" || b0.StartLine != 10 || b0.EndLine != 15 || b0.Statements != 3 || !b0.Covered {
		t.Errorf("blocks[0] = %+v, fields wrong", b0)
	}
	// 第二个是 zero-count
	b1 := blocks[1]
	if b1.Covered {
		t.Errorf("blocks[1].Covered = true, want false (count=0)")
	}
	if b1.Statements != 1 {
		t.Errorf("blocks[1].Statements = %d, want 1", b1.Statements)
	}
	// 末尾块
	bL := blocks[5]
	if bL.File != "silk/ged/bar.go" || bL.StartLine != 30 || bL.EndLine != 35 || !bL.Covered {
		t.Errorf("blocks[5] = %+v, fields wrong", bL)
	}
}

func TestParseCoverageBlankLinesTolerated(t *testing.T) {
	profile := `mode: count

silk/a.go:1.1,2.2 1 3

silk/a.go:3.1,4.2 1 0
`
	mode, blocks, err := ParseCoverage(profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != "count" {
		t.Errorf("mode = %q, want count", mode)
	}
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
}

func TestParseCoverageMissingHeaderDefaultsToSet(t *testing.T) {
	profile := `silk/x.go:1.1,5.2 1 1
silk/x.go:7.1,8.2 1 0
`
	mode, blocks, err := ParseCoverage(profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != "set" {
		t.Errorf("mode = %q, want set (default)", mode)
	}
	if len(blocks) != 2 {
		t.Errorf("blocks = %d, want 2", len(blocks))
	}
}

func TestParseCoverageMalformedLines(t *testing.T) {
	profile := `mode: set
silk/ok.go:1.1,2.2 1 1
not-a-coverage-line
silk/ok.go:3.1,4.2 NOTANUMBER 1
silk/ok.go:5.1,6.2 1 NOTANUMBER
silk/ok.go:7.1 1 1
silk/ok.go:10.1,12.2 1 0
`
	mode, blocks, err := ParseCoverage(profile)
	if err == nil {
		t.Fatal("expected error for malformed lines, got nil")
	}
	if mode != "set" {
		t.Errorf("mode = %q, want set", mode)
	}
	// 应当解析出 2 个合法块 (第 2 行和最后一行)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2 (skip-and-collect); err=%v", len(blocks), err)
	}
	if blocks[0].File != "silk/ok.go" || !blocks[0].Covered {
		t.Errorf("blocks[0] wrong: %+v", blocks[0])
	}
	if blocks[1].Covered {
		t.Errorf("blocks[1].Covered = true, want false")
	}
	// 错误信息中应当包含坏行的行号
	msg := err.Error()
	for _, want := range []string{"3", "4", "5", "6"} {
		if !strings.Contains(msg, want) {
			t.Errorf("err msg %q missing bad line number %s", msg, want)
		}
	}
}

func TestParseCoverageEmpty(t *testing.T) {
	mode, blocks, err := ParseCoverage("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != "set" {
		t.Errorf("mode = %q, want set", mode)
	}
	if len(blocks) != 0 {
		t.Errorf("blocks = %d, want 0", len(blocks))
	}
}

func TestParseCoverageWindowsPath(t *testing.T) {
	// 防御性: 路径里出现盘符冒号时, 解析器仍应按最右冒号切
	profile := `mode: set
C:\src\foo\bar.go:5.1,10.3 2 1
`
	_, blocks, err := ParseCoverage(profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(blocks))
	}
	if blocks[0].File != `C:\src\foo\bar.go` {
		t.Errorf("file = %q, want windows path", blocks[0].File)
	}
}

func TestBuildFileCoverageCoveredWins(t *testing.T) {
	// 同一文件, 多个 block 覆盖重叠行段; 关键检验: 一旦 covered=true, 不会被后续 covered=false 翻掉
	blocks := []CoverageBlock{
		{File: "a.go", StartLine: 1, EndLine: 5, Covered: true, Statements: 3},
		{File: "a.go", StartLine: 3, EndLine: 7, Covered: false, Statements: 2},
		{File: "a.go", StartLine: 10, EndLine: 12, Covered: false, Statements: 1},
		// 之后又来个 covered 覆盖到第 11 行
		{File: "a.go", StartLine: 11, EndLine: 11, Covered: true, Statements: 1},
	}
	out := BuildFileCoverage(blocks)
	fc, ok := out["a.go"]
	if !ok {
		t.Fatal("a.go not in result")
	}
	// 行 1..5: 全部 covered
	for ln := 1; ln <= 5; ln++ {
		if !fc.Covered[ln] {
			t.Errorf("line %d should be covered (block1)", ln)
		}
	}
	// 行 6..7: 仅来自 covered=false 的 block, 应为 false 且存在 key
	for ln := 6; ln <= 7; ln++ {
		v, exists := fc.Covered[ln]
		if !exists {
			t.Errorf("line %d missing from Covered map", ln)
		}
		if v {
			t.Errorf("line %d should be uncovered", ln)
		}
	}
	// 行 8..9: 没有任何 block 提到, 不应出现
	if _, exists := fc.Covered[8]; exists {
		t.Errorf("line 8 should not appear (no block mentions it)")
	}
	// 行 10: 只有 uncovered, 应为 false
	if v, exists := fc.Covered[10]; !exists || v {
		t.Errorf("line 10 should be present and uncovered (got exists=%v v=%v)", exists, v)
	}
	// 行 11: 既被 uncovered 又被 covered 提到, 应为 true ("covered wins")
	if !fc.Covered[11] {
		t.Error("line 11 should be covered (covered wins)")
	}
	// 行 12: 仅 uncovered
	if v := fc.Covered[12]; v {
		t.Error("line 12 should be uncovered")
	}
	// 块计数
	if fc.BlocksTotal != 4 {
		t.Errorf("BlocksTotal = %d, want 4", fc.BlocksTotal)
	}
	if fc.BlocksCovered != 2 {
		t.Errorf("BlocksCovered = %d, want 2", fc.BlocksCovered)
	}
}

func TestBuildFileCoverageOrderIndependent(t *testing.T) {
	// covered wins 与 block 出现顺序无关: 先 uncovered 再 covered, 与反过来效果相同
	forward := []CoverageBlock{
		{File: "x.go", StartLine: 1, EndLine: 3, Covered: false, Statements: 1},
		{File: "x.go", StartLine: 2, EndLine: 4, Covered: true, Statements: 1},
	}
	backward := []CoverageBlock{
		{File: "x.go", StartLine: 2, EndLine: 4, Covered: true, Statements: 1},
		{File: "x.go", StartLine: 1, EndLine: 3, Covered: false, Statements: 1},
	}
	a := BuildFileCoverage(forward)["x.go"]
	b := BuildFileCoverage(backward)["x.go"]
	// 行 2 和 3 在两种顺序下都应该是 covered
	for _, ln := range []int{2, 3} {
		if !a.Covered[ln] {
			t.Errorf("forward order: line %d should be covered", ln)
		}
		if !b.Covered[ln] {
			t.Errorf("backward order: line %d should be covered", ln)
		}
	}
	// 行 1 只被 uncovered 覆盖, 两种顺序都应为 false
	if a.Covered[1] || b.Covered[1] {
		t.Errorf("line 1 should be uncovered in both orders (forward=%v backward=%v)",
			a.Covered[1], b.Covered[1])
	}
	// 行 4 只被 covered 覆盖, 两种顺序都应为 true
	if !a.Covered[4] || !b.Covered[4] {
		t.Errorf("line 4 should be covered in both orders")
	}
}

func TestBuildFileCoverageMultipleFiles(t *testing.T) {
	_, blocks, err := ParseCoverage(sampleProfile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := BuildFileCoverage(blocks)
	if len(out) != 2 {
		t.Fatalf("files = %d, want 2", len(out))
	}
	foo := out["silk/ged/foo.go"]
	if foo == nil {
		t.Fatal("foo.go missing")
	}
	if foo.BlocksTotal != 3 || foo.BlocksCovered != 2 {
		t.Errorf("foo: total=%d covered=%d, want 3/2", foo.BlocksTotal, foo.BlocksCovered)
	}
	// 行 17 在 foo.go 中只被 uncovered 块覆盖
	if foo.Covered[17] {
		t.Error("foo.go line 17 should be uncovered")
	}
	// 行 10 在 covered 块中
	if !foo.Covered[10] {
		t.Error("foo.go line 10 should be covered")
	}
}

func TestCoveragePercent(t *testing.T) {
	// 空
	if p := CoveragePercent(nil); p != 0 {
		t.Errorf("nil percent = %v, want 0", p)
	}
	if p := CoveragePercent(&FileCoverage{}); p != 0 {
		t.Errorf("zero-total percent = %v, want 0", p)
	}
	// 100%
	fc := &FileCoverage{BlocksCovered: 5, BlocksTotal: 5}
	if p := CoveragePercent(fc); p != 100 {
		t.Errorf("100%% percent = %v, want 100", p)
	}
	// 部分
	fc2 := &FileCoverage{BlocksCovered: 1, BlocksTotal: 4}
	if p := CoveragePercent(fc2); p != 25 {
		t.Errorf("25%% percent = %v, want 25", p)
	}
	// 0%
	fc3 := &FileCoverage{BlocksCovered: 0, BlocksTotal: 3}
	if p := CoveragePercent(fc3); p != 0 {
		t.Errorf("0%% percent = %v, want 0", p)
	}
}

func TestParseCoverageNoPanicOnGarbage(t *testing.T) {
	// 全是垃圾, 不应 panic; 应返回 0 blocks + error
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked on garbage input: %v", r)
		}
	}()
	garbage := "@@@\n###\n^^^\n"
	_, blocks, err := ParseCoverage(garbage)
	if err == nil {
		t.Error("expected error for garbage")
	}
	if len(blocks) != 0 {
		t.Errorf("blocks = %d, want 0", len(blocks))
	}
}

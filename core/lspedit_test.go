package core

import (
	"math/rand"
	"testing"
	"unicode/utf16"
)

// edit 是构造 LSPTextEdit 的便捷 helper, 让测试用例读起来更紧凑.
func edit(sl, sc, el, ec int, newText string) LSPTextEdit {
	return LSPTextEdit{
		Range: LSPRange{
			Start: LSPPosition{Line: sl, Character: sc},
			End:   LSPPosition{Line: el, Character: ec},
		},
		NewText: newText,
	}
}

func TestApplyTextEdits_EmptyEdits(t *testing.T) {
	const text = "hello\nworld\n"
	got, err := ApplyTextEdits(text, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != text {
		t.Fatalf("nil edits should be no-op, got %q", got)
	}
	got, err = ApplyTextEdits(text, []LSPTextEdit{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != text {
		t.Fatalf("empty edits should be no-op, got %q", got)
	}
}

func TestApplyTextEdits_ReplaceMiddleSpan(t *testing.T) {
	// "hello world": 把 "world" 换成 "there".
	const text = "hello world"
	got, err := ApplyTextEdits(text, []LSPTextEdit{edit(0, 6, 0, 11, "there")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "hello there"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTextEdits_InsertAtPoint(t *testing.T) {
	// 空 Range = 纯插入. 在 "ac" 的下标 1 处插入 "b".
	const text = "ac"
	got, err := ApplyTextEdits(text, []LSPTextEdit{edit(0, 1, 0, 1, "b")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "abc"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTextEdits_DeleteSpan(t *testing.T) {
	// 空 NewText = 删除. 删掉 "hello " 里的 " world".
	const text = "hello world"
	got, err := ApplyTextEdits(text, []LSPTextEdit{edit(0, 5, 0, 11, "")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "hello"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTextEdits_InsertAtEndOfDocument(t *testing.T) {
	// 在文档末尾插入 (range start == end == 文档末). 末行无尾随换行.
	const text = "line1\nline2"
	// line2 长度 5, 在 (1,5) 处追加 "!".
	got, err := ApplyTextEdits(text, []LSPTextEdit{edit(1, 5, 1, 5, "!")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "line1\nline2!"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTextEdits_MultiLineRangeReplacement(t *testing.T) {
	// 跨行替换: 从 (0,3) 到 (2,2), 把中间一整段换成 "X".
	const text = "abcdef\nghijkl\nmnopqr"
	// start = "abc|def" 之后, byte 3; end = "mn|opqr" 之前, byte = 14+2 = 16.
	got, err := ApplyTextEdits(text, []LSPTextEdit{edit(0, 3, 2, 2, "X")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "abcXopqr"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTextEdits_MultipleNonOverlapping_OrderIndependent(t *testing.T) {
	// 三个互不重叠的 edit; 无论输入顺序如何, 结果必须一致 (验证降序应用逻辑).
	const text = "the quick brown fox"
	edits := []LSPTextEdit{
		edit(0, 0, 0, 3, "a"),     // "the" -> "a"
		edit(0, 4, 0, 9, "slow"),  // "quick" -> "slow"
		edit(0, 16, 0, 19, "cat"), // "fox" -> "cat"
	}
	const want = "a slow brown cat"

	// 已排序顺序.
	got, err := ApplyTextEdits(text, edits)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("sorted order: got %q want %q", got, want)
	}

	// 多次随机打乱, 每次结果都必须等于 want, 且不得改动调用方切片.
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 20; trial++ {
		shuffled := make([]LSPTextEdit, len(edits))
		copy(shuffled, edits)
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		before := make([]LSPTextEdit, len(shuffled))
		copy(before, shuffled)

		got, err := ApplyTextEdits(text, shuffled)
		if err != nil {
			t.Fatalf("trial %d: unexpected error: %v", trial, err)
		}
		if got != want {
			t.Fatalf("trial %d: shuffled order produced %q want %q", trial, got, want)
		}
		// 调用方切片不能被内部排序破坏.
		for i := range before {
			if shuffled[i] != before[i] {
				t.Fatalf("trial %d: caller slice mutated at %d: %+v != %+v", trial, i, shuffled[i], before[i])
			}
		}
	}
}

func TestApplyTextEdits_StableOrderSameStart(t *testing.T) {
	// 两个 start 相同的纯插入 (零宽 range), 必须按输入次序拼接.
	const text = "XY"
	edits := []LSPTextEdit{
		edit(0, 1, 0, 1, "a"),
		edit(0, 1, 0, 1, "b"),
	}
	got, err := ApplyTextEdits(text, edits)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 稳定次序: 先 "a" 后 "b" -> "XabY".
	if want := "XabY"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTextEdits_GofmtStyleReindent(t *testing.T) {
	// 模拟 gofmt 把 3 行用空格缩进改成 tab 缩进: 每行行首的空格区间替换成 "\t".
	// 原文 (空格缩进):
	//   func f() {
	//       a := 1
	//       b := 2
	//       return
	//   }
	src := "func f() {\n    a := 1\n    b := 2\n    return\n}\n"
	edits := []LSPTextEdit{
		edit(1, 0, 1, 4, "\t"), // 第 1 行 4 空格 -> tab
		edit(2, 0, 2, 4, "\t"), // 第 2 行
		edit(3, 0, 3, 4, "\t"), // 第 3 行
	}
	got, err := ApplyTextEdits(src, edits)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "func f() {\n\ta := 1\n\tb := 2\n\treturn\n}\n"
	if got != want {
		t.Fatalf("reindent:\n got %q\nwant %q", got, want)
	}
}

func TestApplyTextEdits_UTF16_NonBMPEmoji(t *testing.T) {
	// 关键 UTF-16 正确性测试.
	// 行: "x😀y". 😀 = U+1F600, 在 UTF-16 下占 2 个 code unit (代理对),
	// 在 UTF-8 下占 4 个字节, 是 1 个 rune.
	//   col 0 -> 'x'   (byte 0)
	//   col 1 -> 😀     (byte 1)
	//   col 3 -> 'y'   (byte 5)   <- 注意是 1+2 = 3, 不是 byte 偏移 5 也不是 rune 下标 2
	//   col 4 -> 行尾   (byte 6)
	const line = "x😀y"

	// 先验证底层换算: 'y' 之后 (col 4) 的字节偏移应为 6 (= len("x😀y")).
	off, err := positionToByteOffset(line, 0, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if off != len(line) {
		t.Fatalf("col 4 byte offset = %d, want %d", off, len(line))
	}
	// 'y' 自身 (col 3) 起点应为 byte 5.
	off, err = positionToByteOffset(line, 0, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if off != 5 {
		t.Fatalf("col 3 byte offset = %d, want 5 (must use UTF-16 units, not bytes=%d or runes=2)", off, len(line))
	}

	// 在 emoji 之后插入: 把 "y" 换成 "Y". 用 UTF-16 列 3..4.
	got, err := ApplyTextEdits(line, []LSPTextEdit{edit(0, 3, 0, 4, "Y")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "x😀Y"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// 在 emoji 后 (col 3) 纯插入 "!" -> "x😀!y".
	got, err = ApplyTextEdits(line, []LSPTextEdit{edit(0, 3, 0, 3, "!")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "x😀!y"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTextEdits_UTF16_MultiByteBMP(t *testing.T) {
	// "café": é = U+00E9, BMP 内 -> 1 个 UTF-16 单元, 但 UTF-8 占 2 字节.
	//   col 0 -> 'c' (byte 0)
	//   col 3 -> 'é' (byte 3)
	//   col 4 -> 行尾 (byte 5)  <- byte 偏移 5, 但 UTF-16 列只到 4
	const line = "café"
	off, err := positionToByteOffset(line, 0, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if off != len(line) {
		t.Fatalf("col 4 byte offset = %d, want %d", off, len(line))
	}
	// 在 "café" 末尾 (col 4) 追加 " au lait".
	got, err := ApplyTextEdits(line, []LSPTextEdit{edit(0, 4, 0, 4, " au lait")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "café au lait"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTextEdits_UTF16_EditAfterNonASCIIOnSameLine(t *testing.T) {
	// 综合: 一行里既有中文又有 ASCII, edit 落在非 ASCII 之后.
	// "你好 world": "你" U+4F60 (1 unit, 3 bytes), "好" U+597D (1 unit, 3 bytes),
	// " " (1 unit, 1 byte), 然后 "world".
	//   "你好 " 共 3 个 UTF-16 单元, byte 7. "world" 从 col 3 / byte 7 开始.
	const line = "你好 world"
	// 把 "world" (col 3..8) 换成 "世界".
	got, err := ApplyTextEdits(line, []LSPTextEdit{edit(0, 3, 0, 8, "世界")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "你好 世界"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTextEdits_OverlappingEdits_Error(t *testing.T) {
	// 两个区间交叠: [0,5) 与 [3,8). 必须报错, 且不返回半成品 (返回原文).
	const text = "abcdefghij"
	edits := []LSPTextEdit{
		edit(0, 0, 0, 5, "AAAAA"),
		edit(0, 3, 0, 8, "BBBBB"),
	}
	got, err := ApplyTextEdits(text, edits)
	if err == nil {
		t.Fatalf("expected overlap error, got nil (result %q)", got)
	}
	if got != text {
		t.Fatalf("on overlap the original text must be returned unchanged, got %q", got)
	}
}

func TestApplyTextEdits_AdjacentEditsNotOverlapping(t *testing.T) {
	// 相接但不重叠: [0,3) 与 [3,6). 半开区间下不算重叠, 应正常应用.
	const text = "abcdef"
	edits := []LSPTextEdit{
		edit(0, 0, 0, 3, "X"),
		edit(0, 3, 0, 6, "Y"),
	}
	got, err := ApplyTextEdits(text, edits)
	if err != nil {
		t.Fatalf("adjacent edits should not error: %v", err)
	}
	if want := "XY"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTextEdits_ClampPastEndOfLine(t *testing.T) {
	// 列号超过行长 -> 钳到行尾, 不 panic. 行 "ab" 长 2, 用 col 99 当 end.
	const text = "ab\ncd"
	got, err := ApplyTextEdits(text, []LSPTextEdit{edit(0, 0, 0, 99, "Z")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// (0,0)..(0,99) 钳成整行 "ab" -> "Z", 保留换行与第二行.
	if want := "Z\ncd"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTextEdits_ClampPastEndOfDocument(t *testing.T) {
	// 行号超过文档行数 -> 钳到文档末尾. 用 line 99 当插入点.
	const text = "ab\ncd"
	got, err := ApplyTextEdits(text, []LSPTextEdit{edit(99, 0, 99, 0, "!")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "ab\ncd!"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyTextEdits_CRLFLine(t *testing.T) {
	// "\r\n" 文档: "\r" 留在行内容里参与 UTF-16 计数.
	// 行 0 = "abc\r" (UTF-16 列: a=0 b=1 c=2 \r=3, 行尾=4).
	// 把 "abc" (col 0..3) 换成 "XY", 保留 "\r\n" 与第二行.
	const text = "abc\r\ndef"
	got, err := ApplyTextEdits(text, []LSPTextEdit{edit(0, 0, 0, 3, "XY")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "XY\r\ndef"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPositionToByteOffset_Basics(t *testing.T) {
	const text = "hello\nworld\n"
	cases := []struct {
		line, char int
		want       int
	}{
		{0, 0, 0},   // 文档起点
		{0, 5, 5},   // 行 0 末尾 ("\n" 前)
		{1, 0, 6},   // 行 1 起点 ("\n" 之后)
		{1, 5, 11},  // 行 1 末尾
		{2, 0, 12},  // 末尾空行 (尾随 "\n" 之后)
		{0, 99, 5},  // 列越界 -> 钳到行尾
		{-1, 0, 0},  // 负行号 -> 当作 0
		{99, 0, 12}, // 行越界 -> 钳到文档末尾
	}
	for _, c := range cases {
		got, err := positionToByteOffset(text, c.line, c.char)
		if err != nil {
			t.Fatalf("(%d,%d): unexpected error: %v", c.line, c.char, err)
		}
		if got != c.want {
			t.Fatalf("positionToByteOffset(%d,%d) = %d, want %d", c.line, c.char, got, c.want)
		}
	}
}

func TestColumnUTF16ToByteOffset_AgainstStdlib(t *testing.T) {
	// 用 stdlib utf16.Encode 反推每个 rune 边界的 UTF-16 列, 交叉验证我们的换算.
	lines := []string{
		"plain ascii",
		"café résumé",
		"你好世界",
		"x😀y🎉z",
		"mix 漢字 and 😀 emoji",
	}
	for _, line := range lines {
		// 逐个 rune 边界检查: 在第 k 个 rune 起点处, 其 UTF-16 列应映射回该 rune 的字节偏移.
		var col int
		for byteIdx, r := range line {
			got := columnUTF16ToByteOffset(line, col)
			if got != byteIdx {
				t.Fatalf("line %q col %d -> byte %d, want %d", line, col, got, byteIdx)
			}
			col += len(utf16.Encode([]rune{r}))
		}
		// 行尾列应钳/落到 len(line).
		if got := columnUTF16ToByteOffset(line, col); got != len(line) {
			t.Fatalf("line %q end col %d -> byte %d, want %d", line, col, got, len(line))
		}
	}
}

func TestUTF16RuneLen(t *testing.T) {
	cases := []struct {
		r    rune
		want int
	}{
		{'a', 1},
		{'é', 1},      // U+00E9 BMP
		{'你', 1},      // U+4F60 BMP
		{'\r', 1},     // 控制字符
		{'😀', 2},      // U+1F600 non-BMP
		{'🎉', 2},      // U+1F389 non-BMP
		{0x10FFFF, 2}, // 最大合法码点
	}
	for _, c := range cases {
		if got := utf16RuneLen(c.r); got != c.want {
			t.Fatalf("utf16RuneLen(%U) = %d, want %d", c.r, got, c.want)
		}
	}
}

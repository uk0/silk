package core

import (
	"fmt"
	"sort"
	"unicode/utf16"
)

// LSP 文本编辑的应用层: 把 gopls 从 textDocument/formatting 与
// textDocument/rename (WorkspaceEdit 里某个文件的 edit 列表) 返回的
// []LSPTextEdit 正确地落到一段文档文本上.
//
// 这里只暴露一个纯函数 ApplyTextEdits, 不碰 IO / 不依赖 LSPClient, 方便单测.
// LSPTextEdit / LSPRange / LSPPosition 复用 core/lspclient.go 里已有的类型.
//
// 三个容易踩错的点, 也是这个文件存在的理由:
//
//  1. LSP 坐标是 0 基的, 且 Position.Character 是 *UTF-16 code unit* 偏移,
//     不是字节偏移, 也不是 rune 下标. 对纯 ASCII 三者重合; 一旦出现多字节
//     rune (中文/重音字符) 或 non-BMP 字符 (emoji), 就会分叉:
//       - 一个 rune < 0x10000  记 1 个 UTF-16 单元
//       - 一个 rune >= 0x10000 记 2 个 UTF-16 单元 (代理对)
//     我们把每个 (Line, Character) 先定位到行, 再在行内按 rune 累加 UTF-16
//     单元数, 直到凑够 Character, 由此换算出绝对字节偏移. 见 positionToByteOffset.
//
//  2. 多个 edit 的坐标都基于 *原始* 文档. 若从前往后应用, 前面的替换会改变
//     后面 edit 的字节偏移, 造成 offset drift. 解决办法是按 start 位置 *降序*
//     应用 (文档末尾的 edit 先改), 这样每次替换都不影响尚未应用的、位于其
//     之前的 edit 的偏移. start 相同的多个 edit 保持稳定次序 (sort.SliceStable
//     + 原始下标 tie-break).
//
//  3. LSP 禁止 edit 区间互相重叠. server 真要发来重叠的 edit (畸形响应),
//     与其默默改坏文档, 不如直接报错. 我们在排序后检测相邻区间是否交叠.
//
// 行终止符: gopls 输出统一用 "\n". 我们直接在原始字节上把 "\n" 当行边界,
// "\r" (若有) 作为行内容的一部分参与 UTF-16 计数 —— 这样对 gopls 的 "\n"
// 输出是无损 round-trip 的; 即便文档是 "\r\n", 偏移换算依然自洽, 因为
// server 报的 Character 也是按同一套 "行内含 \r" 的视图算出来的.

// ApplyTextEdits 把一组 LSP TextEdit 应用到 text 上, 返回编辑后的文本
// 语义:
//   - edits 为空 -> 原样返回 text, 无错误.
//   - 每个 edit 的 Range 用 NewText 替换 (空 Range = 插入, 空 NewText = 删除).
//   - 所有 edit 的坐标都按 *原始* text 解释; 内部按 start 降序应用以避免偏移漂移.
//   - 区间重叠 -> 返回错误, 不产出半成品文本.
//   - 行/列越界 -> 钳到文档末尾 (不 panic), 用于容忍 server 给的边界外位置.
//
// 不修改调用方传入的 edits 切片 (内部排序的是副本).
func ApplyTextEdits(text string, edits []LSPTextEdit) (string, error) {
	if len(edits) == 0 {
		return text, nil
	}

	// 预先把每行的起始字节偏移算好, 供所有 edit 复用, 避免每次都重扫一遍 text.
	lineStarts := computeLineStarts(text)

	// 把每个 edit 解析成绝对字节区间 [start, end). 同时记录原始下标做稳定排序.
	type resolved struct {
		start   int
		end     int
		newText string
		order   int // 原始下标, 用于 start 相同的 tie-break
	}
	rs := make([]resolved, 0, len(edits))
	for i, e := range edits {
		start, err := offsetFromLineStarts(text, lineStarts, e.Range.Start.Line, e.Range.Start.Character)
		if err != nil {
			return text, fmt.Errorf("lspedit: edit %d start: %w", i, err)
		}
		end, err := offsetFromLineStarts(text, lineStarts, e.Range.End.Line, e.Range.End.Character)
		if err != nil {
			return text, fmt.Errorf("lspedit: edit %d end: %w", i, err)
		}
		if start > end {
			return text, fmt.Errorf("lspedit: edit %d has inverted range: start byte %d > end byte %d", i, start, end)
		}
		rs = append(rs, resolved{start: start, end: end, newText: e.NewText, order: i})
	}

	// 升序排 (start, 然后原始 order) 便于做重叠检测; 应用时再反向遍历.
	sort.SliceStable(rs, func(a, b int) bool {
		if rs[a].start != rs[b].start {
			return rs[a].start < rs[b].start
		}
		return rs[a].order < rs[b].order
	})

	// 重叠检测: 升序下, 前一个 edit 的 end 不能越过后一个 edit 的 start.
	// 注意 [start, end) 是半开区间, 故相接 (prev.end == next.start) 不算重叠.
	for i := 1; i < len(rs); i++ {
		if rs[i].start < rs[i-1].end {
			return text, fmt.Errorf("lspedit: overlapping edits: [%d,%d) and [%d,%d)",
				rs[i-1].start, rs[i-1].end, rs[i].start, rs[i].end)
		}
	}

	// 降序应用: 从文档末尾的 edit 开始, 这样每次替换都不影响其左侧 edit 的偏移.
	out := text
	for i := len(rs) - 1; i >= 0; i-- {
		r := rs[i]
		out = out[:r.start] + r.newText + out[r.end:]
	}
	return out, nil
}

// computeLineStarts 返回每一行起始处的字节偏移
// 第 0 行从 0 开始; 每遇到一个 "\n", 其后一个字节是下一行的起点.
// "\r" 不作为行边界 —— 它留在行内容里参与后续 UTF-16 计数 (见文件头说明).
// 结果长度 = 行数; lineStarts[i] 是第 i 行第一个字节在 text 中的下标.
func computeLineStarts(text string) []int {
	starts := make([]int, 0, 16)
	starts = append(starts, 0)
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// positionToByteOffset 把一个 LSP (line, utf16Char) 坐标换算成 text 里的字节偏移
// line / utf16Char 均为 0 基; utf16Char 是行内的 UTF-16 code unit 偏移.
// 越界处理 (钳位, 不报错):
//   - line < 0           -> 当作 0
//   - line 超过最后一行   -> 钳到文档末尾 (len(text))
//   - utf16Char 超过行长 -> 钳到该行末尾 (下一行起点前, 不含 "\n")
//
// 单独导出成包内 helper 以便独立单测.
func positionToByteOffset(text string, line, utf16Char int) (int, error) {
	return offsetFromLineStarts(text, computeLineStarts(text), line, utf16Char)
}

// offsetFromLineStarts 是 positionToByteOffset 的实现, 复用预算好的 lineStarts
// 抽出来是因为 ApplyTextEdits 对同一段 text 要换算很多个坐标, 行起点只算一次.
func offsetFromLineStarts(text string, lineStarts []int, line, utf16Char int) (int, error) {
	if line < 0 {
		line = 0
	}
	if line >= len(lineStarts) {
		// 行号越过文档 -> 钳到末尾.
		return len(text), nil
	}

	lineStart := lineStarts[line]
	// 行的结束: 下一行起点前 (即 "\n" 的位置); 最后一行则到 text 末尾.
	var lineEnd int
	if line+1 < len(lineStarts) {
		// lineStarts[line+1] 指向 "\n" 之后一个字节, 减 1 得到 "\n" 自身的位置.
		lineEnd = lineStarts[line+1] - 1
	} else {
		lineEnd = len(text)
	}

	return columnUTF16ToByteOffset(text[lineStart:lineEnd], utf16Char) + lineStart, nil
}

// columnUTF16ToByteOffset 在 *单行* 内容 lineText 上, 把 UTF-16 列偏移换算成字节偏移
// 算法: 按 rune 遍历 lineText, 每个 rune 累加它占的 UTF-16 code unit 数
// (>= 0x10000 的记 2, 其余记 1), 一旦累计的单元数到达/越过 utf16Char,
// 就返回当前 rune 起始处的字节偏移.
// 边界:
//   - utf16Char <= 0           -> 0 (行首)
//   - utf16Char 落在某个代理对中间 (理论上 server 不该这么发) -> 取该 rune 起点,
//     不会把字节偏移切到 rune 中间.
//   - utf16Char 超过整行 -> 返回 len(lineText) (行尾, 钳位).
//
// lineText 不含行尾 "\n"; 若原文是 "\r\n", "\r" 会留在 lineText 里, 按 1 个
// UTF-16 单元计数 —— 与 server 的视图一致.
func columnUTF16ToByteOffset(lineText string, utf16Char int) int {
	if utf16Char <= 0 {
		return 0
	}
	units := 0
	for i, r := range lineText {
		if units >= utf16Char {
			return i
		}
		units += utf16RuneLen(r)
	}
	// 走完整行都没凑够 utf16Char -> 钳到行尾.
	return len(lineText)
}

// utf16RuneLen 返回一个 rune 在 UTF-16 下占的 code unit 数
// BMP 内 (< 0x10000) 记 1; 其余 (需要代理对编码, 例如多数 emoji) 记 2.
// 用 utf16.EncodeRune 判别: 它对需要代理对的码点返回一对非 0xFFFD 的单元.
func utf16RuneLen(r rune) int {
	if utf16.IsSurrogate(r) {
		// rune 本身落在代理区 (畸形输入), 按 1 个单元保守处理, 不 panic.
		return 1
	}
	r1, _ := utf16.EncodeRune(r)
	if r1 == 0xFFFD {
		// EncodeRune 对不可编码 (非法码点) 的 rune 返回 (U+FFFD, U+FFFD),
		// 那是个单一 BMP 替换字符, 记 1 个单元.
		return 1
	}
	return 2
}

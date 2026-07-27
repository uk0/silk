package core

import (
	"fmt"
	"slices"
	"strings"
)

// 三方合并 (diff3) 引擎
// 设计目标: 给 IDE 的冲突编辑器提供纯数据层的合并结果 —— 不调用 git,
// 不碰文件系统, 只吃三份行切片(base/ours/theirs), 吐出一串 MergeChunk.
//
// 算法是经典 diff3:
//  1. 分别对 (base, ours) 与 (base, theirs) 做 LCS, 得到 base 每一行在
//     对侧的行号(对不上的记 -1);
//  2. 沿 base 同时推进三个游标: 三方都停在同一行时属于 stable 区; 否则
//     向前找下一个"双方都还留着"的同步点, 中间那段就是一个变更区;
//  3. 变更区按内容归类: 只有一侧改 → ours/theirs(可自动合并); 两侧改成
//     完全相同的内容 → stable(重复改动折叠); 两侧改得不一样 → conflict.
//
// 另外提供 git 冲突标记(<<<<<<< ||||||| ======= >>>>>>>)的解析与渲染,
// 于是"已经带标记的冲突文件"也能进同一个编辑器.
//
// 复杂度与 gui/diffview.go 的行 diff 一致: LCS 是 O(m*n) 的 int 表,
// 对"两个文件快照"这种规模足够, 不做分块优化.

// MergeKind 标记一个合并块的性质
type MergeKind int

const (
	MergeStable   MergeKind = iota // 三方一致, 或双方做了完全相同的改动
	MergeOurs                      // 只有 ours 改动了这一段
	MergeTheirs                    // 只有 theirs 改动了这一段
	MergeConflict                  // 双方对同一段 base 做了不同的改动
)

// String 给出 kind 的稳定名字, 面板表头与测试都用它
func (k MergeKind) String() string {
	switch k {
	case MergeStable:
		return "stable"
	case MergeOurs:
		return "ours"
	case MergeTheirs:
		return "theirs"
	case MergeConflict:
		return "conflict"
	}
	return "unknown"
}

// MergeChunk 是合并结果里的一段, 三个字段分别是该段在 base/ours/theirs
// 中的原始行:
//   - MergeStable:   Ours 与 Theirs 内容相同(即最终文本), 但 Base 可能不同
//     —— 双方做了同样的改动时就是这种情况;
//   - MergeOurs:     Theirs == Base, 最终取 Ours;
//   - MergeTheirs:   Ours == Base, 最终取 Theirs;
//   - MergeConflict: 三者互不相同, 需要人工选边.
//
// 空的一侧用 nil 表示(纯插入/纯删除), 便于 reflect.DeepEqual 断言.
// 所有切片都是新分配的副本, 不会别名到调用方传进来的 base/ours/theirs.
type MergeChunk struct {
	Kind   MergeKind
	Base   []string
	Ours   []string
	Theirs []string
}

// Resolved 返回该块在最终文本中应当出现的行.
// conflict 块返回 nil —— 它必须先由调用方(冲突编辑器)选边.
// 返回的是内部切片, 调用方只读不改.
func (c MergeChunk) Resolved() []string {
	switch c.Kind {
	case MergeStable, MergeOurs:
		return c.Ours
	case MergeTheirs:
		return c.Theirs
	}
	return nil
}

// Merge3 对 base/ours/theirs 三份行切片做 diff3 式三方合并, 返回按 base
// 顺序排列的块列表. 不改动入参; 三方全空时返回 nil.
//
// 非重叠的改动被自动合并(块的 Kind 是 ours 或 theirs), 只有双方对同一段
// base 改出不同内容时才产出 MergeConflict. 相邻的 stable 块会被并成一块,
// 于是"重复改动"折叠后不会在列表里留下碎片.
func Merge3(base, ours, theirs []string) []MergeChunk {
	mo := mergeMatchIndex(base, ours)
	mt := mergeMatchIndex(base, theirs)

	var out []MergeChunk
	i, o, t := 0, 0, 0
	for i < len(base) || o < len(ours) || t < len(theirs) {
		// stable 区: base[i] 在两侧都恰好落在当前游标上, 一路吃到不成立为止
		if i < len(base) && mo[i] == o && mt[i] == t {
			start := i
			for i < len(base) && mo[i] == o && mt[i] == t {
				i++
				o++
				t++
			}
			seg := base[start:i]
			out = appendMergeChunk(out, MergeChunk{
				Kind:   MergeStable,
				Base:   cloneMergeLines(seg),
				Ours:   cloneMergeLines(seg),
				Theirs: cloneMergeLines(seg),
			})
			continue
		}

		// 变更区: 推进到下一个同步点, 中间三段各自成为该块的三面
		i2, o2, t2 := nextMergeSync(mo, mt, i, o, t, len(base), len(ours), len(theirs))
		out = appendMergeChunk(out, classifyMergeRegion(base[i:i2], ours[o:o2], theirs[t:t2]))
		i, o, t = i2, o2, t2
	}
	return out
}

// nextMergeSync 找出从 base 下标 i 起的第一个同步点: 该 base 行在两侧都
// 有对应行, 且都不在已消费过的位置之前. 找不到时返回三方的末尾, 于是最后
// 一个变更区一直吃到文件尾.
//
// 注意扫描从 i(而不是 i+1) 开始: base[i] 本身可能已对齐但对侧游标还落在
// 它前面(某侧在此处插入了行), 这时同步点就是 i, 变更区的 base 一侧为空.
func nextMergeSync(mo, mt []int, i, o, t, nBase, nOurs, nTheirs int) (int, int, int) {
	for k := i; k < nBase; k++ {
		if mo[k] >= 0 && mt[k] >= 0 && mo[k] >= o && mt[k] >= t {
			return k, mo[k], mt[k]
		}
	}
	return nBase, nOurs, nTheirs
}

// classifyMergeRegion 判定一个变更区的性质并生成对应的块
func classifyMergeRegion(baseSeg, oursSeg, theirsSeg []string) MergeChunk {
	c := MergeChunk{
		Base:   cloneMergeLines(baseSeg),
		Ours:   cloneMergeLines(oursSeg),
		Theirs: cloneMergeLines(theirsSeg),
	}
	ourChanged := !slices.Equal(oursSeg, baseSeg)
	theirChanged := !slices.Equal(theirsSeg, baseSeg)
	switch {
	case !ourChanged && !theirChanged:
		// LCS 没配上但内容其实一样(重排的边界), 当作未改动
		c.Kind = MergeStable
	case ourChanged && !theirChanged:
		c.Kind = MergeOurs
	case !ourChanged && theirChanged:
		c.Kind = MergeTheirs
	case slices.Equal(oursSeg, theirsSeg):
		// 双方改成了同样的内容: 折叠成 stable, 最终文本取这份改动
		c.Kind = MergeStable
	default:
		c.Kind = MergeConflict
	}
	return c
}

// appendMergeChunk 追加一个块, 并把相邻的 stable 块并成一块.
// 只有 stable 块可能相邻 —— 变更区总是被同步点(即 stable 区)隔开,
// 唯一的例外是"未改动区 + 重复改动区"这种连续 stable.
func appendMergeChunk(dst []MergeChunk, c MergeChunk) []MergeChunk {
	if n := len(dst); n > 0 && c.Kind == MergeStable && dst[n-1].Kind == MergeStable {
		// 三面都是新分配的副本, 直接 append 不会写到调用方的切片里
		dst[n-1].Base = append(dst[n-1].Base, c.Base...)
		dst[n-1].Ours = append(dst[n-1].Ours, c.Ours...)
		dst[n-1].Theirs = append(dst[n-1].Theirs, c.Theirs...)
		return dst
	}
	return append(dst, c)
}

// mergeMatchIndex 用 LCS 把 base 的每一行对齐到 b 的行号上
// 返回长度为 len(base) 的切片: 对齐上的行给出 b 的下标, 对不上的给 -1.
// 结果在已匹配的下标上严格单调递增(LCS 的性质), Merge3 的游标推进依赖这点.
func mergeMatchIndex(base, b []string) []int {
	m, n := len(base), len(b)
	match := make([]int, m)
	for i := range match {
		match[i] = -1
	}
	if m == 0 || n == 0 {
		return match
	}

	// 后缀方向的 LCS 长度表: dp[i][j] = LCS(base[i:], b[j:])
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if base[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	// 从头往后走一遍表, 相等即配对; 否则丢掉"损失更小"的那一侧
	for i, j := 0, 0; i < m && j < n; {
		switch {
		case base[i] == b[j]:
			match[i] = j
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return match
}

// cloneMergeLines 复制一段行; 空段统一规整为 nil, 一是让 reflect.DeepEqual
// 断言稳定, 二是保证块里的切片不会别名到调用方的底层数组(否则 append 合并
// 相邻 stable 块时会踩坏入参).
func cloneMergeLines(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}

// git 冲突标记. 三方合并留下的记号一律是 7 个字符, 后面可跟一个空格和说明文字.
const (
	ConflictMarkerOurs   = "<<<<<<<"
	ConflictMarkerBase   = "|||||||"
	ConflictMarkerSep    = "======="
	ConflictMarkerTheirs = ">>>>>>>"
)

// MergeLabels 是渲染冲突标记时跟在标记后面的说明文字(git 里通常是
// "HEAD" 和分支名). Base 为空表示按 git 默认的 merge 风格输出 —— 不写
// "|||||||" 段; 非空则输出 diff3 风格, 保留 base 段.
type MergeLabels struct {
	Ours   string
	Base   string
	Theirs string
}

// RenderConflictMarkers 把块列表渲染回文本行:
// 非冲突块直接输出 Resolved() 的行, 冲突块套上 git 冲突标记.
// 与 ParseConflictMarkers 互逆(标记后的说明文字来自 labels, 不存在块里).
func RenderConflictMarkers(chunks []MergeChunk, labels MergeLabels) []string {
	var out []string
	for _, c := range chunks {
		if c.Kind != MergeConflict {
			out = append(out, c.Resolved()...)
			continue
		}
		out = append(out, markerLine(ConflictMarkerOurs, labels.Ours))
		out = append(out, c.Ours...)
		if labels.Base != "" {
			out = append(out, markerLine(ConflictMarkerBase, labels.Base))
			out = append(out, c.Base...)
		}
		out = append(out, ConflictMarkerSep)
		out = append(out, c.Theirs...)
		out = append(out, markerLine(ConflictMarkerTheirs, labels.Theirs))
	}
	return out
}

// ParseConflictMarkers 把一份已经带 git 冲突标记的文本解析回块列表:
// 标记之外的普通文本成为 stable 块(三面同内容), 每个标记块成为一个
// MergeConflict 块(没有 "|||||||" 段时 Base 为 nil).
//
// 容错策略与 ParseUnifiedDiff 一致: 出错也返回已解析出的块, 错误汇总到
// 一个 wrapped error, 永不 panic. 具体地
//   - 冲突块内又出现 "<<<<<<<": 记一条错, 丢掉半个块重新开始;
//   - 缺 "=======" 就直接 ">>>>>>>": 记一条错, theirs 侧当空;
//   - 文本结束时块还没闭合: 记一条错, 把已累积的三面作为冲突块交出.
//
// "=======" / "|||||||" 只在冲突块内部才当分隔符, 于是 Markdown 的下划线
// 和注释里的分隔线不会被误判; 长度不等于 7 的连字符串同样只是普通文本.
func ParseConflictMarkers(lines []string) ([]MergeChunk, error) {
	const (
		stText = iota
		stOurs
		stBase
		stTheirs
	)

	var (
		out                      []MergeChunk
		errs                     []string
		state                    = stText
		text, ours, base, theirs []string
	)

	flushText := func() {
		if len(text) == 0 {
			return
		}
		out = append(out, MergeChunk{
			Kind:   MergeStable,
			Base:   cloneMergeLines(text),
			Ours:   cloneMergeLines(text),
			Theirs: cloneMergeLines(text),
		})
		text = nil
	}
	flushConflict := func() {
		out = append(out, MergeChunk{
			Kind:   MergeConflict,
			Base:   base,
			Ours:   ours,
			Theirs: theirs,
		})
		ours, base, theirs = nil, nil, nil
	}

	for idx, line := range lines {
		switch {
		case isConflictMarker(line, ConflictMarkerOurs):
			if state != stText {
				errs = append(errs, fmt.Sprintf("line %d: nested %q", idx+1, ConflictMarkerOurs))
				ours, base, theirs = nil, nil, nil
			}
			flushText()
			state = stOurs

		case state == stOurs && isConflictMarker(line, ConflictMarkerBase):
			state = stBase

		case (state == stOurs || state == stBase) && isConflictMarker(line, ConflictMarkerSep):
			state = stTheirs

		case (state == stOurs || state == stBase) && isConflictMarker(line, ConflictMarkerTheirs):
			errs = append(errs, fmt.Sprintf("line %d: %q before %q", idx+1, ConflictMarkerTheirs, ConflictMarkerSep))
			flushConflict()
			state = stText

		case state == stTheirs && isConflictMarker(line, ConflictMarkerTheirs):
			flushConflict()
			state = stText

		default:
			switch state {
			case stOurs:
				ours = append(ours, line)
			case stBase:
				base = append(base, line)
			case stTheirs:
				theirs = append(theirs, line)
			default:
				text = append(text, line)
			}
		}
	}

	if state != stText {
		errs = append(errs, fmt.Sprintf("unterminated conflict: missing %q", ConflictMarkerTheirs))
		flushConflict()
	}
	flushText()

	if len(errs) > 0 {
		return out, fmt.Errorf("conflict markers: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

// isConflictMarker 判断 line 是否是给定的冲突标记行: 必须以那 7 个字符
// 开头, 之后要么行尾, 要么一个空格(后面是说明文字). 于是 "========"
// (8 个) 永远只是普通文本.
func isConflictMarker(line, marker string) bool {
	if !strings.HasPrefix(line, marker) {
		return false
	}
	rest := line[len(marker):]
	return rest == "" || rest[0] == ' '
}

// markerLine 拼出 "<<<<<<< label" 这样的标记行; label 为空时只留标记
func markerLine(marker, label string) string {
	if label == "" {
		return marker
	}
	return marker + " " + label
}

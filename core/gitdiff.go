package core

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// Unified diff (即 `git diff` 默认输出) 的最小解析器
// 设计目标: 让 IDE 把 "vs HEAD" 的差异渲染到编辑器侧边栏,
// 不复刻 git 自带的全部上下文头(rename/copy/similarity 等), 只覆盖渲染所需的几项:
//   - "diff --git a/X b/Y"   起一个新文件(可选, 某些 diff 工具会省略)
//   - "--- <path>"           旧路径(/dev/null 表示新建)
//   - "+++ <path>"           新路径(/dev/null 表示删除)
//   - "@@ -A,B +C,D @@ ..."  hunk 头, B/D 缺省为 1
//   - " "/"+"/"-"            上下文/新增/删除
//   - "\ No newline at end of file"
// 其余行(index/similarity/Binary files/...) 一律跳过.
// 跳错收集: 单个 @@ 头格式不合法时, 该 hunk 被丢弃, 错误累计到返回的 wrapped error,
// 解析器继续读后续内容, 永远不 panic.

// DiffLineKind 枚举 hunk 内单行的语义
type DiffLineKind int

const (
	DiffLineContext   DiffLineKind = iota // " " 上下文行, 新旧文件都有
	DiffLineAdded                         // "+" 仅出现在新文件
	DiffLineRemoved                       // "-" 仅出现在旧文件
	DiffLineNoNewline                     // "\ No newline at end of file" 占位
)

// DiffFile 描述一个文件的差异. OldPath/NewPath 已经剥去 "a/"/"b/" 前缀;
// /dev/null 被规整为 "", 调用方据此区分新建(OldPath=="")和删除(NewPath=="").
type DiffFile struct {
	OldPath string
	NewPath string
	Hunks   []DiffHunk
}

// DiffHunk 描述一个 @@ 头及其下属行
// OldStart/NewStart 为 1-based 起始行号; OldCount/NewCount 在 @@ 省略时默认为 1.
type DiffHunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []DiffLine
}

// DiffLine 是 hunk 中的一行
// Text 不包含开头的 +/-/空格 标记字符; 对 NoNewline 类型 Text 固定为空.
type DiffLine struct {
	Kind DiffLineKind
	Text string
}

// ParseUnifiedDiff 解析 unified diff 文本, 返回每个文件的差异
// 行边界用 bufio.Scanner 取, 不依赖文末换行; 永远不 panic;
// 单条畸形 @@ 头汇总到 wrapped error, 但其余文件/hunk 仍会出现在返回切片里.
// 当 src 为空(去掉首尾空白后)时返回 (nil, nil).
func ParseUnifiedDiff(src string) ([]DiffFile, error) {
	if strings.TrimSpace(src) == "" {
		return nil, nil
	}

	var files []DiffFile
	var errs []string

	// 当前正在累积的文件和 hunk; 用指针便于 "向当前位置追加行" 这种迭代
	var cur *DiffFile
	var hunk *DiffHunk

	// flushHunk 把当前 hunk 落盘到当前文件
	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
		}
		hunk = nil
	}
	// flushFile 把当前文件落盘到结果切片
	flushFile := func() {
		flushHunk()
		if cur != nil {
			files = append(files, *cur)
		}
		cur = nil
	}

	scanner := bufio.NewScanner(strings.NewReader(src))
	// diff 行理论上不会很长, 但代码文件里偶有 minified 行, 把上限放宽到 4 MiB
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "diff --git "):
			// 新文件起点; 此处不抽 a/X b/Y, 真正的路径来自后续 --- / +++ 头
			flushFile()
			cur = &DiffFile{}

		case strings.HasPrefix(line, "--- "):
			// 某些非 git 工具不会发 "diff --git" 行; 直接用 "--- " 作为文件边界兜底
			if cur == nil {
				cur = &DiffFile{}
			} else if len(cur.Hunks) > 0 || cur.OldPath != "" || cur.NewPath != "" {
				// 已经累积过内容, 说明上一文件结束; 落盘后开启新文件
				flushFile()
				cur = &DiffFile{}
			}
			// 进入新文件意味着先前的 hunk 必须落盘
			flushHunk()
			cur.OldPath = parseDiffPath(strings.TrimPrefix(line, "--- "))

		case strings.HasPrefix(line, "+++ "):
			if cur == nil {
				// "+++" 出现在没有 "---" 的情况下, 容错: 当成新文件起点
				cur = &DiffFile{}
			}
			flushHunk()
			cur.NewPath = parseDiffPath(strings.TrimPrefix(line, "+++ "))

		case strings.HasPrefix(line, "@@"):
			// 开新 hunk 前要把上一个落盘
			flushHunk()
			h, err := parseHunkHeader(line)
			if err != nil {
				errs = append(errs, fmt.Sprintf("line %d: %v", lineNo, err))
				continue
			}
			if cur == nil {
				// 没有任何文件头就先出现 @@, 兜底起一个空文件
				cur = &DiffFile{}
			}
			hunk = &h

		case strings.HasPrefix(line, "\\"):
			// "\ No newline at end of file" 之类的备注, 仅在 hunk 内有意义
			if hunk != nil {
				hunk.Lines = append(hunk.Lines, DiffLine{Kind: DiffLineNoNewline})
			}

		case hunk != nil && len(line) > 0 && (line[0] == '+' || line[0] == '-' || line[0] == ' '):
			// 注意: "+++" 和 "---" 已在上面分支匹配; 走到这里只剩单字符前缀
			kind := DiffLineContext
			switch line[0] {
			case '+':
				kind = DiffLineAdded
			case '-':
				kind = DiffLineRemoved
			}
			hunk.Lines = append(hunk.Lines, DiffLine{Kind: kind, Text: line[1:]})

		default:
			// "index abc..def 100644", "similarity index 100%", "Binary files ..." 等
			// 无需 IDE 渲染的元数据行, 全部跳过
		}
	}
	flushFile()

	if scanErr := scanner.Err(); scanErr != nil {
		// 扫描器自身报错(超长行等)归并到 err, 已解析的 files 仍然返回
		errs = append(errs, fmt.Sprintf("scan: %v", scanErr))
	}

	if len(errs) > 0 {
		return files, fmt.Errorf("unified diff parse: %s", strings.Join(errs, "; "))
	}
	return files, nil
}

// parseDiffPath 把 "--- a/foo.go" 中的 "a/foo.go" 规整成 "foo.go"
// "/dev/null" → ""; 其他不带 "a/"/"b/" 前缀的路径原样保留(供非 git 工具的 diff)
// 末尾的时间戳片段(unified diff 允许 "path<TAB>timestamp")会被剥掉
func parseDiffPath(s string) string {
	// 截掉 path 后可能跟着的 "\t2024-01-02 ..." 时间戳
	if tab := strings.IndexByte(s, '\t'); tab >= 0 {
		s = s[:tab]
	}
	s = strings.TrimSpace(s)
	if s == "/dev/null" {
		return ""
	}
	// git 的 a//b/ 前缀仅在路径首位剥一次, 不去递归
	switch {
	case strings.HasPrefix(s, "a/"):
		return s[2:]
	case strings.HasPrefix(s, "b/"):
		return s[2:]
	}
	return s
}

// parseHunkHeader 解析 "@@ -A[,B] +C[,D] @@ optional fn" 形式的 hunk 头
// B/D 省略时默认 1; 形如 "@@ -0,0 +1,5 @@" (新建文件) 也合法.
// 任何字段解析失败都返回 error, 由调用方决定是否丢弃这个 hunk.
func parseHunkHeader(line string) (DiffHunk, error) {
	var h DiffHunk
	// 截出两个 "@@" 之间的部分; 不要求第二个 @@ 一定存在,
	// 但要求开头是 "@@"
	rest := strings.TrimPrefix(line, "@@")
	if rest == line {
		return h, fmt.Errorf("hunk header missing '@@' prefix: %q", line)
	}
	// 找到第二个 "@@", 若没有就把剩余整段作为 range 部分
	if i := strings.Index(rest, "@@"); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimSpace(rest)
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return h, fmt.Errorf("hunk header missing ranges: %q", line)
	}
	oldRange, newRange := fields[0], fields[1]
	if !strings.HasPrefix(oldRange, "-") {
		return h, fmt.Errorf("hunk header old range missing '-': %q", line)
	}
	if !strings.HasPrefix(newRange, "+") {
		return h, fmt.Errorf("hunk header new range missing '+': %q", line)
	}
	oldStart, oldCount, err := parseRange(oldRange[1:])
	if err != nil {
		return h, fmt.Errorf("hunk old range: %w", err)
	}
	newStart, newCount, err := parseRange(newRange[1:])
	if err != nil {
		return h, fmt.Errorf("hunk new range: %w", err)
	}
	h.OldStart = oldStart
	h.OldCount = oldCount
	h.NewStart = newStart
	h.NewCount = newCount
	return h, nil
}

// parseRange 解析 "A" 或 "A,B" 形式. A 必须存在; B 缺省为 1.
func parseRange(s string) (start, count int, err error) {
	count = 1
	if s == "" {
		return 0, 0, fmt.Errorf("empty range")
	}
	comma := strings.IndexByte(s, ',')
	if comma < 0 {
		start, err = strconv.Atoi(s)
		if err != nil {
			return 0, 0, fmt.Errorf("start: %w", err)
		}
		return start, count, nil
	}
	start, err = strconv.Atoi(s[:comma])
	if err != nil {
		return 0, 0, fmt.Errorf("start: %w", err)
	}
	count, err = strconv.Atoi(s[comma+1:])
	if err != nil {
		return 0, 0, fmt.Errorf("count: %w", err)
	}
	return start, count, nil
}

// AddedLinesByFile 把已解析的 diff 折叠成 "新文件路径 → 新增行号列表"
// 行号以新文件坐标(1-based) 为准, 顺序与 hunk 内出现顺序一致.
// 用途: 编辑器侧边栏 "我刚加的行" 标记.
// 规则:
//   - 删除行(只在旧文件) 不计入;
//   - 上下文行(同时存在新旧两份) 不计入;
//   - 跳过 NewPath 为空的条目(纯删除文件), 它们没有 "新文件" 可挂.
func AddedLinesByFile(files []DiffFile) map[string][]int {
	out := make(map[string][]int)
	for _, f := range files {
		if f.NewPath == "" {
			continue
		}
		var added []int
		for _, h := range f.Hunks {
			// cursor 是当前行在新文件中的行号, 从 hunk 的 NewStart 起步
			cursor := h.NewStart
			for _, ln := range h.Lines {
				switch ln.Kind {
				case DiffLineAdded:
					added = append(added, cursor)
					cursor++
				case DiffLineContext:
					cursor++
				case DiffLineRemoved, DiffLineNoNewline:
					// 不前进 cursor: 删除行在新文件中不存在;
					// NoNewline 是上一条行的修饰, 也不占据新文件行号
				}
			}
		}
		if len(added) > 0 {
			out[f.NewPath] = added
		}
	}
	return out
}

// RemovedLineCount 返回某文件中所有 hunk 内被删除的行数总和
func RemovedLineCount(file DiffFile) int {
	n := 0
	for _, h := range file.Hunks {
		for _, ln := range h.Lines {
			if ln.Kind == DiffLineRemoved {
				n++
			}
		}
	}
	return n
}

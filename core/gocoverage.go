package core

import (
	"bufio"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Go coverage profile (`go test -coverprofile=cover.out`) 解析器
// 文件格式:
//   mode: set|count|atomic
//   <file>:<startLine>.<startCol>,<endLine>.<endCol> <numStmts> <count>
//   ...
// 例如:
//   silk/ged/foo.go:10.13,15.2 3 1
//   silk/ged/foo.go:17.2,17.10 1 0
// count>0 表示该代码块被执行过(覆盖); count==0 表示未覆盖.
// 这里只做数据层解析, IDE 编辑器侧的 gutter 渲染由后续 commit 接入.

// CoverageBlock 描述一个连续的可执行语句块
type CoverageBlock struct {
	File       string
	StartLine  int
	EndLine    int
	Covered    bool // count > 0
	Statements int  // numStmts
}

// FileCoverage 是单个源文件的覆盖率折叠结果
// Covered 仅包含被任意 block 提及的行;
// 未在任何 block 出现的行不会出现在 map 中(IDE 侧不应在这些行画 gutter)
type FileCoverage struct {
	File          string
	Covered       map[int]bool // line -> covered (covered wins: 一旦为 true 不会被任何 block 翻回 false)
	BlocksCovered int          // count of blocks with count > 0
	BlocksTotal   int
}

// ParseCoverage 解析 go cover 文本格式
// 返回:
//   mode   - "set"/"count"/"atomic", 缺省 header 时按 "set" 处理
//   blocks - 所有成功解析的块 (跳错收集策略, 即使有坏行也会返回到目前为止解析成功的块)
//   err    - 非 nil 表示有畸形行, 错误信息包含全部坏行的行号
// 解析器对空行宽容. 永远不 panic.
func ParseCoverage(profile string) (mode string, blocks []CoverageBlock, err error) {
	mode = "set"
	scanner := bufio.NewScanner(strings.NewReader(profile))
	// 覆盖率文件理论上不会有超长行, 但 IDE 可能传进来诡异内容, 这里把上限放宽
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var badLines []int
	lineNo := 0
	headerSeen := false
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !headerSeen && strings.HasPrefix(line, "mode:") {
			m := strings.TrimSpace(strings.TrimPrefix(line, "mode:"))
			if m != "" {
				mode = m
			}
			headerSeen = true
			continue
		}
		// 第一行不是 mode: 头, 也视作 block 行尝试解析
		headerSeen = true

		blk, perr := parseCoverageBlock(line)
		if perr != nil {
			badLines = append(badLines, lineNo)
			continue
		}
		blocks = append(blocks, blk)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		// 扫描器自身错误归并为 err 返回, 已解析的 blocks 仍然返回
		err = fmt.Errorf("coverage scan: %w", scanErr)
		return
	}
	if len(badLines) > 0 {
		err = fmt.Errorf("coverage parse: %d malformed line(s) at %v", len(badLines), badLines)
	}
	return
}

// parseCoverageBlock 解析单行 "file:sl.sc,el.ec stmts count"
func parseCoverageBlock(line string) (CoverageBlock, error) {
	var blk CoverageBlock
	// 末尾两个字段以空格分隔, 文件路径可能含空格(罕见但允许), 因此从末尾切
	// 不过 cover 输出从不在路径里塞空格, 用从右起的两个空格切就够了
	idx2 := strings.LastIndexByte(line, ' ')
	if idx2 <= 0 {
		return blk, errors.New("missing count field")
	}
	idx1 := strings.LastIndexByte(line[:idx2], ' ')
	if idx1 <= 0 {
		return blk, errors.New("missing stmts field")
	}
	countStr := line[idx2+1:]
	stmtsStr := line[idx1+1 : idx2]
	head := line[:idx1] // file:sl.sc,el.ec

	count, err := strconv.Atoi(countStr)
	if err != nil {
		return blk, fmt.Errorf("count: %w", err)
	}
	stmts, err := strconv.Atoi(stmtsStr)
	if err != nil {
		return blk, fmt.Errorf("stmts: %w", err)
	}

	// 拆 file 和 range. 用最右侧的 ':' 切, 兼容 Windows 风格 "C:\\foo\\bar.go:1.1,2.2"
	colon := strings.LastIndexByte(head, ':')
	if colon < 0 {
		return blk, errors.New("missing ':' between file and range")
	}
	file := head[:colon]
	rng := head[colon+1:]
	if file == "" {
		return blk, errors.New("empty file")
	}
	comma := strings.IndexByte(rng, ',')
	if comma < 0 {
		return blk, errors.New("missing ',' in range")
	}
	startPart := rng[:comma]
	endPart := rng[comma+1:]
	startLine, _, err := splitLineCol(startPart)
	if err != nil {
		return blk, fmt.Errorf("start: %w", err)
	}
	endLine, _, err := splitLineCol(endPart)
	if err != nil {
		return blk, fmt.Errorf("end: %w", err)
	}
	if startLine < 1 || endLine < startLine {
		return blk, fmt.Errorf("invalid line range %d..%d", startLine, endLine)
	}
	blk.File = file
	blk.StartLine = startLine
	blk.EndLine = endLine
	blk.Statements = stmts
	blk.Covered = count > 0
	return blk, nil
}

// splitLineCol 解析 "line.col" 形式, col 暂不被上层使用但仍校验合法性
func splitLineCol(s string) (line, col int, err error) {
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		err = errors.New("missing '.'")
		return
	}
	line, err = strconv.Atoi(s[:dot])
	if err != nil {
		return
	}
	col, err = strconv.Atoi(s[dot+1:])
	return
}

// BuildFileCoverage 把 block 列表折叠成每文件的逐行覆盖图
// 同一行被多个 block 覆盖时, "covered wins":
// 只要任何一个 block 报告 count>0, 该行就是 covered, 不会被 count==0 的 block 翻回去.
// 一行只有在没有任何 covered=true 的 block 提到它时, 才会被记为 Covered=false.
func BuildFileCoverage(blocks []CoverageBlock) map[string]*FileCoverage {
	out := make(map[string]*FileCoverage)
	for _, b := range blocks {
		fc, ok := out[b.File]
		if !ok {
			fc = &FileCoverage{File: b.File, Covered: make(map[int]bool)}
			out[b.File] = fc
		}
		fc.BlocksTotal++
		if b.Covered {
			fc.BlocksCovered++
		}
		for ln := b.StartLine; ln <= b.EndLine; ln++ {
			if b.Covered {
				fc.Covered[ln] = true
			} else {
				// 仅当尚未被标记 covered 时才记 false
				if _, exists := fc.Covered[ln]; !exists {
					fc.Covered[ln] = false
				}
			}
		}
	}
	return out
}

// CoveragePercent 返回 BlocksCovered / BlocksTotal * 100
// fc==nil 或 BlocksTotal==0 时返回 0
func CoveragePercent(fc *FileCoverage) float64 {
	if fc == nil || fc.BlocksTotal == 0 {
		return 0
	}
	return float64(fc.BlocksCovered) / float64(fc.BlocksTotal) * 100
}

package core

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// TODO/FIXME 注释扫描器
// 设计目标: 给 IDE 的 "TODO" 面板提供一份可操作的注释清单 —— 遍历工程目录,
// 从源码注释里抽出 TODO/FIXME/XXX/HACK/NOTE 这类标记及其后续文本.
// 全部走 stdlib(os, io/fs, path/filepath, bufio, regexp, sort, strings),
// 每个文件的读取错误都是"跳过并收集"(经 Warn 记录), 绝不因单个文件失败而中断整棵树,
// 也永远不 panic.
//
// v1 只扫描 Go 源码(.go). 其它语言(shell/python 的 `#` 注释、C/JS 等)留作后续,
// parseTodoLine 的启发式已为 Go 的 `//` 与 `/* ... */` 注释调好.

// TodoKind 是一个待办标记的类别
type TodoKind string

const (
	TodoTODO  TodoKind = "TODO"
	TodoFIXME TodoKind = "FIXME"
	TodoXXX   TodoKind = "XXX"
	TodoHACK  TodoKind = "HACK"
	TodoNOTE  TodoKind = "NOTE"
)

// TodoItem 是扫描到的一条待办标记
type TodoItem struct {
	File string   // 文件路径(随传入 dir 而定: dir 为绝对路径则此处也是绝对路径)
	Line int      // 1-based 行号
	Kind TodoKind // 标记类别
	Text string   // 关键字之后的文本, 已 TrimSpace
}

// maxTodoItems 是单次扫描收集的条目上限
// 防止在异常巨大的目录树上无限膨胀内存; 触顶时会 Warn 记录并提前结束遍历(非静默截断).
const maxTodoItems = 5000

// todoLineRe 是识别"注释里的待办标记"的行级启发式正则
// 结构: 先要求一个注释引导(`//` 或 `/*` 或续行的 `*`), 再匹配大写关键字(词边界 \b),
// 关键字后可选一个分隔符([:\s]?)与若干空白, 其余作为文本捕获.
//
// 已知边界(行级正则, 非完整 Go 词法分析, 取"简单且够用"):
//   - 假阳性: `*` 引导也会命中乘法/解引用行(如 `x * XXX`) —— 这是为了能匹配以 `*`
//     开头的块注释续行而付出的代价.
//   - 假阳性: 引导符出现在字符串字面量里也会命中(如 `s := "// TODO x"`),
//     因为我们只做单行正则. 但纯粹的 `"TODO ..."`(串内无引导符)不会命中.
//   - 假阳性: 单行块注释 `/* TODO: x */` 的尾部 `*/` 会留在 Text 里(仅做了 TrimSpace).
//   - 假阴性: 小写 `todo` 故意不匹配(大小写敏感, 避开标识符/字符串里的 todo).
//   - 假阴性: 关键字后粘连更多单词字符(如 `TODONE`)不匹配(\b 词边界所致).
//   - 假阴性: 完全没有注释引导的裸 `TODO` 不匹配 —— 必须有引导符.
var todoLineRe = regexp.MustCompile(`(?://|/\*|\*)\s*(TODO|FIXME|XXX|HACK|NOTE)\b[:\s]?\s*(.*)`)

// parseTodoLine 对单行文本做一次匹配, 返回标记类别 + 文本 + 是否命中
// 纯函数, 不碰磁盘, 可独立测试. 一行有多个标记时只取最左侧那一个.
func parseTodoLine(line string) (TodoKind, string, bool) {
	m := todoLineRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return TodoKind(m[1]), strings.TrimSpace(m[2]), true
}

// ScanTodos 递归扫描 dir 下的 .go 文件, 收集其中的待办标记
// 跳过 vendor/、node_modules/ 以及一切隐藏目录(.git/.idea/.vscode 等). 非 .go 文件忽略.
// 单个文件的读取错误经 Warn 记录后跳过, 继续扫描其余文件; 只有根目录本身不可访问才返回错误.
// 结果按 File 再按 Line 排序, 保证稳定输出. 条目数触及 maxTodoItems 时提前结束并 Warn.
func ScanTodos(dir string) ([]TodoItem, error) {
	var items []TodoItem
	truncated := false

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// 根目录本身不可访问 -> 视为致命, 返回错误
			if path == dir {
				return walkErr
			}
			// 深层某个条目出错(权限/竞态等) -> 记录并跳过, 继续遍历兄弟节点
			Warn("scan todos: walk ", path, ": ", walkErr)
			return nil
		}

		if d.IsDir() {
			// 根目录不参与跳过判断(其名字可能以 "." 开头, 例如 .claude/worktrees/...)
			if path != dir && skipTodoDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// v1 只看 Go 源码
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		fileItems, ferr := scanTodoFile(path)
		if ferr != nil {
			Warn("scan todos: read ", path, ": ", ferr)
			return nil
		}
		for _, it := range fileItems {
			items = append(items, it)
			if len(items) >= maxTodoItems {
				truncated = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return items, err
	}
	if truncated {
		Warn("scan todos: results truncated at ", maxTodoItems, " items under ", dir)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].File != items[j].File {
			return items[i].File < items[j].File
		}
		return items[i].Line < items[j].Line
	})
	return items, nil
}

// skipTodoDir 判定某个目录名在扫描时是否应整棵跳过
// vendor/node_modules 是依赖目录; 以 "." 开头的隐藏目录(.git/.idea/...)一律跳过.
func skipTodoDir(name string) bool {
	switch name {
	case "vendor", "node_modules":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// scanTodoFile 逐行扫描单个文件, 抽出其中的待办标记
// 打开或扫描失败时返回已收集到的条目连同错误, 由调用方决定记录并跳过.
func scanTodoFile(path string) ([]TodoItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var items []TodoItem
	sc := bufio.NewScanner(f)
	// 生成/压缩代码里可能出现超长行, 把 Scanner 缓冲上限调到 1MiB 防止 ErrTooLong
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNo := 0
	for sc.Scan() {
		lineNo++
		kind, text, ok := parseTodoLine(sc.Text())
		if !ok {
			continue
		}
		items = append(items, TodoItem{
			File: path,
			Line: lineNo,
			Kind: kind,
			Text: text,
		})
	}
	if err := sc.Err(); err != nil {
		return items, err
	}
	return items, nil
}

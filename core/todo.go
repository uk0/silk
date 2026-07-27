package core

import (
	"bufio"
	"go/scanner"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// TODO/FIXME 注释扫描器
// 设计目标: 给 IDE 的 "TODO" 面板提供一份可操作的注释清单 —— 遍历工程目录,
// 从源码注释里抽出 TODO/FIXME/HACK/XXX/NOTE/BUG 这类标记及其后续文本.
// 除 go/scanner 外全部走 stdlib(os, io/fs, path/filepath, bufio, regexp, sort, strings),
// 每个文件的读取错误都是"跳过并收集"(经 Warn 记录), 绝不因单个文件失败而中断整棵树,
// 也永远不 panic.
//
// 三条能力线:
//   - 标记集可配置: TodoScanner 接受任意标记(默认 DefaultTodoTags), ScanTodos 用默认集.
//   - 多语言注释识别: 后缀 -> 语言(todoLangForPath). Go 走 go/scanner 只看注释 token,
//     因此字符串字面量里的 `"// TODO x"` 不再误报; C 系(`//`, `/* */`)与
//     shell/python 系(`#`)走逐行启发式; 其它后缀不扫描.
//   - 增量索引: TodoIndex 用 UpdateFile/RemoveFile 跟着编辑器缓冲走, 免去每次全量重扫.

// TodoKind 是一个待办标记的类别
type TodoKind string

const (
	TodoTODO  TodoKind = "TODO"
	TodoFIXME TodoKind = "FIXME"
	TodoXXX   TodoKind = "XXX"
	TodoHACK  TodoKind = "HACK"
	TodoNOTE  TodoKind = "NOTE"
	TodoBUG   TodoKind = "BUG"
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

// DefaultTodoTags 返回默认标记集: TODO/FIXME/HACK/XXX/NOTE/BUG
// 返回的是副本, 调用方可以自由增删后交给 NewTodoScanner / NewTodoIndex.
func DefaultTodoTags() []TodoKind {
	return []TodoKind{TodoTODO, TodoFIXME, TodoHACK, TodoXXX, TodoNOTE, TodoBUG}
}

// todoTagAlt 把标记集拼成正则里的分支: "TODO|FIXME|..."
// 每个标记都过 QuoteMeta, 因此自定义标记里的正则元字符不会破坏(或注入)整个表达式.
func todoTagAlt(tags []TodoKind) string {
	alts := make([]string, 0, len(tags))
	for _, t := range tags {
		if t == "" {
			continue
		}
		alts = append(alts, regexp.QuoteMeta(string(t)))
	}
	return strings.Join(alts, "|")
}

// buildTodoLineRe 构造"注释里的待办标记"的行级启发式正则
// 结构: 引导符(`//` / `/*` / 续行的 `*` / `#`) -> 大写关键字(词边界 \b) ->
// 可选一个分隔符([:\s]?)与若干空白 -> 其余作为文本. 三个捕获组依次是
// 引导符、关键字、文本 —— 引导符要留着, 好按语言判定它是否算注释(todoLeadInOK).
//
// 已知边界(行级正则, 非完整词法分析, 取"简单且够用"; Go 文件不走这条路径,
// 它由 go/scanner 精确处理):
//   - 假阳性: `*` 引导也会命中乘法/解引用行(如 `x * XXX`) —— 这是为了能匹配以 `*`
//     开头的块注释续行而付出的代价.
//   - 假阳性: 引导符出现在字符串字面量里也会命中(如 `s := "// TODO x"`),
//     因为我们只做单行正则. 但纯粹的 `"TODO ..."`(串内无引导符)不会命中.
//   - 假阳性: 单行块注释 `/* TODO: x */` 的尾部 `*/` 会留在 Text 里(仅做了 TrimSpace).
//   - 假阴性: 小写 `todo` 故意不匹配(大小写敏感, 避开标识符/字符串里的 todo).
//   - 假阴性: 关键字后粘连更多单词字符(如 `TODONE`)不匹配(\b 词边界所致).
//   - 假阴性: 完全没有注释引导的裸 `TODO` 不匹配 —— 必须有引导符.
func buildTodoLineRe(tags []TodoKind) *regexp.Regexp {
	return regexp.MustCompile(`(//|/\*|\*|#)\s*(` + todoTagAlt(tags) + `)\b[:\s]?\s*(.*)`)
}

// buildTodoCommentRe 构造"文本已确定是注释"时用的正则
// 不要求引导符 —— 调用方(go/scanner 路径)已经知道这段文字整体就是注释, 所以块注释里
// 裸写的 `TODO: x` 也应识别. 两个捕获组: 关键字、文本.
func buildTodoCommentRe(tags []TodoKind) *regexp.Regexp {
	return regexp.MustCompile(`\b(` + todoTagAlt(tags) + `)\b[:\s]?\s*(.*)`)
}

// todoLineRe 是默认标记集的行级正则(parseTodoLine 用)
var todoLineRe = buildTodoLineRe(DefaultTodoTags())

// parseTodoLine 对单行文本做一次匹配, 返回标记类别 + 文本 + 是否命中
// 纯函数, 不碰磁盘, 可独立测试. 一行有多个标记时只取最左侧那一个.
// 语言无关: 任一引导符都接受(按语言收紧的版本是 TodoScanner.parseLine).
func parseTodoLine(line string) (TodoKind, string, bool) {
	m := todoLineRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return TodoKind(m[2]), strings.TrimSpace(m[3]), true
}

// todoLang 是一个文件的注释语法归类
type todoLang int

const (
	todoLangNone  todoLang = iota // 不识别 -> 不扫描(零值, 因此 map 未命中即不扫描)
	todoLangGo                    // Go: 走 go/scanner, 只看注释 token
	todoLangCLike                 // C/C++/Java/JS/TS/Rust/...: `//` 与 `/* */`
	todoLangHash                  // shell/python/ruby/yaml/...: `#`
)

// todoLangExt 是"后缀 -> 注释语法"的映射(后缀一律小写比较)
var todoLangExt = map[string]todoLang{
	".go": todoLangGo,

	".c": todoLangCLike, ".h": todoLangCLike, ".cc": todoLangCLike,
	".cpp": todoLangCLike, ".cxx": todoLangCLike, ".hpp": todoLangCLike,
	".hh": todoLangCLike, ".m": todoLangCLike, ".mm": todoLangCLike,
	".java": todoLangCLike, ".kt": todoLangCLike, ".cs": todoLangCLike,
	".js": todoLangCLike, ".jsx": todoLangCLike, ".mjs": todoLangCLike,
	".ts": todoLangCLike, ".tsx": todoLangCLike, ".swift": todoLangCLike,
	".rs": todoLangCLike, ".php": todoLangCLike, ".dart": todoLangCLike,
	".scala": todoLangCLike, ".css": todoLangCLike, ".scss": todoLangCLike,
	".less": todoLangCLike, ".glsl": todoLangCLike,

	".sh": todoLangHash, ".bash": todoLangHash, ".zsh": todoLangHash,
	".py": todoLangHash, ".rb": todoLangHash, ".pl": todoLangHash,
	".yaml": todoLangHash, ".yml": todoLangHash, ".toml": todoLangHash,
	".tf": todoLangHash, ".cmake": todoLangHash, ".mk": todoLangHash,
}

// todoLangBase 覆盖几个没有后缀的常见文件名
var todoLangBase = map[string]todoLang{
	"Makefile":   todoLangHash,
	"Dockerfile": todoLangHash,
}

// todoLangForPath 判定 path 的注释语法; 不识别的类型返回 todoLangNone(不扫描)
func todoLangForPath(path string) todoLang {
	if lang, ok := todoLangExt[strings.ToLower(filepath.Ext(path))]; ok {
		return lang
	}
	return todoLangBase[filepath.Base(path)]
}

// todoLeadInOK 判定引导符是否属于该语言的注释语法
// `#` 只算 shell/python 系的注释(C 系里的 `#` 是预处理指令); `//`、`/*`、`*`
// 反之只算 C 系(含 Go)的注释.
func todoLeadInOK(lead string, lang todoLang) bool {
	if lead == "#" {
		return lang == todoLangHash
	}
	return lang != todoLangHash
}

// TodoScanner 是一个标记集可配置的待办扫描器
// 无可变状态(仅持有编译好的正则), 因此可并发复用. 语言判定见 todoLangForPath.
type TodoScanner struct {
	tags      []TodoKind
	lineRe    *regexp.Regexp // 需要注释引导符: 逐行启发式(非 Go 语言)
	commentRe *regexp.Regexp // 不需要引导符: 文本已确定是注释(Go 走 go/scanner)
}

// NewTodoScanner 创建扫描器; 不传标记(或全是空串)则用 DefaultTodoTags
func NewTodoScanner(tags ...TodoKind) *TodoScanner {
	set := make([]TodoKind, 0, len(tags))
	for _, t := range tags {
		if t != "" {
			set = append(set, t)
		}
	}
	if len(set) == 0 {
		set = DefaultTodoTags()
	}
	return &TodoScanner{
		tags:      set,
		lineRe:    buildTodoLineRe(set),
		commentRe: buildTodoCommentRe(set),
	}
}

// defaultTodoScanner 是 ScanTodos 用的默认标记集扫描器
var defaultTodoScanner = NewTodoScanner()

// Tags 返回该扫描器识别的标记集副本
func (s *TodoScanner) Tags() []TodoKind {
	out := make([]TodoKind, len(s.tags))
	copy(out, s.tags)
	return out
}

// parseLine 是按语言收紧过的 parseTodoLine: 引导符必须属于该语言的注释语法
func (s *TodoScanner) parseLine(line string, lang todoLang) (TodoKind, string, bool) {
	m := s.lineRe.FindStringSubmatch(line)
	if m == nil || !todoLeadInOK(m[1], lang) {
		return "", "", false
	}
	return TodoKind(m[2]), strings.TrimSpace(m[3]), true
}

// parseComment 在"已知整体是注释"的一行文字里找标记
// 不要求引导符; 顺手把单行块注释的收尾 `*/` 从文本里去掉.
func (s *TodoScanner) parseComment(line string) (TodoKind, string, bool) {
	m := s.commentRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	text := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(m[2]), "*/"))
	return TodoKind(m[1]), text, true
}

// ScanContent 从内存里的文件内容抽取标记, 完全不碰磁盘
// 语言按 path 的后缀判定: Go 走 go/scanner(字符串字面量里的注释形状不会误报),
// C 系与 shell/python 系走逐行启发式, 其它后缀返回 nil.
// 这是 TodoIndex.UpdateFile 的底座 —— 编辑器里未保存的缓冲也能直接索引.
func (s *TodoScanner) ScanContent(path, content string) []TodoItem {
	switch lang := todoLangForPath(path); lang {
	case todoLangGo:
		return s.scanGoComments(path, content)
	case todoLangCLike, todoLangHash:
		items, _ := s.scanLines(path, strings.NewReader(content), lang)
		return items
	}
	return nil
}

// ScanFile 读取并扫描单个文件; 不识别的类型返回 (nil, nil)
func (s *TodoScanner) ScanFile(path string) ([]TodoItem, error) {
	lang := todoLangForPath(path)
	if lang == todoLangNone {
		return nil, nil
	}
	if lang == todoLangGo {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return s.scanGoComments(path, string(data)), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return s.scanLines(path, f, lang)
}

// ScanDir 递归扫描 dir 下所有可识别语言的源文件, 收集其中的待办标记
// 跳过 vendor/、node_modules/ 以及一切隐藏目录(.git/.idea/.vscode 等); 不识别的
// 文件类型忽略. 单个文件的读取错误经 Warn 记录后跳过, 继续扫描其余文件; 只有根目录
// 本身不可访问才返回错误. 结果按 File 再按 Line 排序, 保证稳定输出.
// 条目数触及 maxTodoItems 时提前结束并 Warn.
func (s *TodoScanner) ScanDir(dir string) ([]TodoItem, error) {
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

		// 不认识注释语法的文件类型直接跳过(省掉一次开文件)
		if todoLangForPath(path) == todoLangNone {
			return nil
		}

		fileItems, ferr := s.ScanFile(path)
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

	sortTodoItems(items)
	return items, nil
}

// scanGoComments 用 go/scanner 遍历 Go 源码的注释 token 抽取标记
// 只看 token.COMMENT, 所以字符串字面量(含反引号原始串)里长得像注释的 `"// TODO x"`
// 不会被误判成标记 —— 这正是行级正则的已知假阳性. 词法错误一律忽略(编辑中的半成品
// 文件是常态), scanner 仍会把能识别的注释吐出来.
func (s *TodoScanner) scanGoComments(path, content string) []TodoItem {
	var items []TodoItem
	fset := token.NewFileSet()
	f := fset.AddFile(path, fset.Base(), len(content))

	var sc scanner.Scanner
	sc.Init(f, []byte(content), nil, scanner.ScanComments)
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.COMMENT {
			continue
		}
		// 块注释可能跨行: 以注释起始行为基准, 加上标记所在的行内偏移.
		base := fset.Position(pos).Line
		for i, ln := range strings.Split(lit, "\n") {
			kind, text, ok := s.parseComment(ln)
			if !ok {
				continue
			}
			items = append(items, TodoItem{File: path, Line: base + i, Kind: kind, Text: text})
		}
	}
	return items
}

// scanLines 逐行扫描非 Go 文件, 抽出其中的待办标记
// 扫描失败时返回已收集到的条目连同错误, 由调用方决定记录并跳过.
func (s *TodoScanner) scanLines(path string, r io.Reader, lang todoLang) ([]TodoItem, error) {
	var items []TodoItem
	sc := bufio.NewScanner(r)
	// 生成/压缩代码里可能出现超长行, 把 Scanner 缓冲上限调到 1MiB 防止 ErrTooLong
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNo := 0
	for sc.Scan() {
		lineNo++
		kind, text, ok := s.parseLine(sc.Text(), lang)
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

// ScanTodos 递归扫描 dir 下的源文件, 收集其中的待办标记(默认标记集)
// 语言支持与跳过规则见 TodoScanner.ScanDir; 需要自定义标记集时改用
// NewTodoScanner(tags...).ScanDir(dir).
func ScanTodos(dir string) ([]TodoItem, error) {
	return defaultTodoScanner.ScanDir(dir)
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

// sortTodoItems 按 File 再按 Line 原地排序, 保证输出稳定
func sortTodoItems(items []TodoItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].File != items[j].File {
			return items[i].File < items[j].File
		}
		return items[i].Line < items[j].Line
	})
}

// TodoIndex 是一份可增量维护的待办索引
// 全量重扫一棵大树对每次按键来说太贵, 所以索引按文件分桶: 编辑器保存(或改动)一个
// 文件就 UpdateFile 一次, 删除/关闭就 RemoveFile, 查询侧照旧拿到全工程视图.
// UpdateFile 吃的是内容而不是路径, 因此未保存的缓冲也能索引.
// 带 RWMutex, 允许后台 goroutine 更新、UI 线程查询.
type TodoIndex struct {
	mu     sync.RWMutex
	sc     *TodoScanner
	byFile map[string][]TodoItem
}

// NewTodoIndex 创建索引; 不传标记则用 DefaultTodoTags
func NewTodoIndex(tags ...TodoKind) *TodoIndex {
	return &TodoIndex{
		sc:     NewTodoScanner(tags...),
		byFile: make(map[string][]TodoItem),
	}
}

// UpdateFile 用 content 重新索引 path, 返回该文件的标记(副本)
// 没有标记(或文件类型不识别)时把该文件从索引里摘掉, 免得留下空桶.
func (ix *TodoIndex) UpdateFile(path, content string) []TodoItem {
	items := ix.sc.ScanContent(path, content)

	ix.mu.Lock()
	defer ix.mu.Unlock()
	if len(items) == 0 {
		delete(ix.byFile, path)
		return nil
	}
	ix.byFile[path] = items
	return append([]TodoItem(nil), items...)
}

// RemoveFile 把 path 从索引里摘掉(文件被删除或关闭时调用)
func (ix *TodoIndex) RemoveFile(path string) {
	ix.mu.Lock()
	delete(ix.byFile, path)
	ix.mu.Unlock()
}

// ScanDir 用一次全量扫描给索引播种, 之后交给 UpdateFile/RemoveFile 增量维护
// 播种会清掉旧内容, 因此可以拿它做"重新打开工程".
func (ix *TodoIndex) ScanDir(dir string) error {
	items, err := ix.sc.ScanDir(dir)

	byFile := make(map[string][]TodoItem)
	for _, it := range items {
		byFile[it.File] = append(byFile[it.File], it)
	}

	ix.mu.Lock()
	ix.byFile = byFile
	ix.mu.Unlock()
	return err
}

// ByFile 返回 path 的标记副本(按行号)
func (ix *TodoIndex) ByFile(path string) []TodoItem {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return append([]TodoItem(nil), ix.byFile[path]...)
}

// ByTag 返回全索引里某一类标记, 按 File 再 Line 排序
func (ix *TodoIndex) ByTag(kind TodoKind) []TodoItem {
	return ix.collect(func(it TodoItem) bool { return it.Kind == kind })
}

// All 返回索引里的全部标记, 按 File 再 Line 排序
func (ix *TodoIndex) All() []TodoItem {
	return ix.collect(nil)
}

// Files 返回索引里仍有标记的文件路径, 按路径排序
func (ix *TodoIndex) Files() []string {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	out := make([]string, 0, len(ix.byFile))
	for path := range ix.byFile {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// Len 返回索引里的标记总数
func (ix *TodoIndex) Len() int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	n := 0
	for _, items := range ix.byFile {
		n += len(items)
	}
	return n
}

// collect 汇总所有桶里满足 keep 的条目并排序; keep 为 nil 表示全收
func (ix *TodoIndex) collect(keep func(TodoItem) bool) []TodoItem {
	ix.mu.RLock()
	var out []TodoItem
	for _, items := range ix.byFile {
		for _, it := range items {
			if keep == nil || keep(it) {
				out = append(out, it)
			}
		}
	}
	ix.mu.RUnlock()

	sortTodoItems(out)
	return out
}

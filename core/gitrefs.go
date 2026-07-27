package core

import (
	"strconv"
	"strings"
)

// 分支 / 远程 / 工作树状态的读写(与 core/gitops.go 共用 GitOps 与可注入的 GitRunner)
// 这里放三样东西:
//   - 分支: ListBranches(本地+远程, 当前分支标记, 上游, ahead/behind)、CreateBranch、
//     CheckoutBranch、DeleteBranch
//   - 远程: ListRemotes、AddRemote
//   - 状态: GitOps.Status 走 `git status --porcelain=v1 -z`(NUL 分隔), 以及
//     ParseGitPorcelainLines + DecodeGitQuotedPath 处理不带 -z 的带引号形态
//
// 关于路径引号: core/git.go 的 GitStatusPorcelain 只剥掉首尾引号, 不做 C 风格反转义,
// 于是 "\346\265\213" 这种非 ASCII 路径会原样带着反斜杠泄漏给上层. 这里两条路都补齐:
// 首选 -z(路径原样输出, 根本不需要引号/转义), 需要解析已有的非 -z 文本时用
// DecodeGitQuotedPath 做完整反转义. 所有 Parse* 都是纯函数, 不碰进程.

// GitBranch 是 `git branch --list --all --format=...` 的一条分支
// Name 是短名(本地 "main", 远程 "origin/main"), Ref 是完整引用名;
// Upstream 为空表示没有配上游; Ahead/Behind 是相对上游的领先/落后提交数;
// Gone 表示配了上游但那个引用已经不存在了(上游分支被删).
type GitBranch struct {
	Name     string
	Ref      string
	Remote   bool
	Current  bool
	Hash     string // 短 SHA
	Subject  string // 分支尖端提交的标题
	Upstream string
	Ahead    int
	Behind   int
	Gone     bool
}

// gitBranchFormat 是 ListBranches 用的 --format 串
// 七个字段用 0x1f(unit separator)分隔, 避免提交标题里的普通标点导致误切;
// %1f 是 ref-filter 的十六进制转义, 与 git log 的 %x1f 等价.
// %(HEAD) 对当前分支是 "*", 其余是空格.
const gitBranchFormat = "%(HEAD)%1f%(refname)%1f%(refname:short)%1f%(objectname:short)%1f%(upstream:short)%1f%(upstream:track)%1f%(contents:subject)"

// ListBranches 列出本地与远程跟踪分支(`git branch --list --all --format=...`)
func (g *GitOps) ListBranches() ([]GitBranch, error) {
	out, err := g.run("branch", "--list", "--all", "--format="+gitBranchFormat)
	if err != nil {
		return nil, err
	}
	return ParseGitBranches(out), nil
}

// ParseGitBranches 解析 gitBranchFormat 的输出(纯函数)
// 一行一个引用, 字段以 0x1f 分隔. 跳过两类不是分支的行:
//   - detached HEAD 的伪条目: git branch 会给它一行 "(HEAD detached at abc1234)",
//     refname 不以 "refs/" 开头 —— 它不是引用, 当前是否游离用 GitCurrentBranch 判定.
//   - refs/remotes/<remote>/HEAD: 指向远程默认分支的符号引用, 不是分支本身,
//     而且它的 refname:short 会缩成 "origin" 这种误导性的名字.
//
// 字段数不齐的行跳过并继续, 永远不 panic.
func ParseGitBranches(out string) []GitBranch {
	var branches []GitBranch
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) < 7 {
			continue
		}
		ref := parts[1]
		if !strings.HasPrefix(ref, "refs/") {
			continue
		}
		b := GitBranch{
			Current:  strings.HasPrefix(parts[0], "*"),
			Ref:      ref,
			Name:     parts[2],
			Hash:     parts[3],
			Upstream: parts[4],
			Subject:  parts[6],
			Remote:   strings.HasPrefix(ref, "refs/remotes/"),
		}
		if b.Remote && strings.HasSuffix(ref, "/HEAD") {
			continue
		}
		b.Ahead, b.Behind, b.Gone = parseGitTrack(parts[5])
		branches = append(branches, b)
	}
	return branches
}

// parseGitTrack 解析 %(upstream:track) 的取值(纯函数)
// 形态(C locale 下的英文形式, execGit 已用 LC_ALL=C 钉死):
//
//	""                    没有上游, 或与上游完全同步
//	"[ahead 2]"           领先 2 个提交
//	"[behind 1]"          落后 1 个提交
//	"[ahead 1, behind 1]" 已分叉
//	"[gone]"              上游引用已不存在
//
// 认不出的串返回 (0,0,false) —— 不猜, 宁可显示"同步"也不给出错的数字.
func parseGitTrack(s string) (ahead, behind int, gone bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return 0, 0, false
	}
	for _, part := range strings.Split(s[1:len(s)-1], ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "gone":
			gone = true
		case strings.HasPrefix(part, "ahead "):
			if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(part, "ahead "))); err == nil {
				ahead = n
			}
		case strings.HasPrefix(part, "behind "):
			if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(part, "behind "))); err == nil {
				behind = n
			}
		}
	}
	return ahead, behind, gone
}

// CreateBranch 新建分支但不切过去(`git branch <name> [<start-point>]`)
// startPoint 为空表示从当前 HEAD 起分支; 非空时可以是分支名、标签或 SHA.
func (g *GitOps) CreateBranch(name, startPoint string) error {
	if err := validateGitArg("branch name", name); err != nil {
		return err
	}
	args := []string{"branch", name}
	if startPoint != "" {
		if err := validateGitArg("start point", startPoint); err != nil {
			return err
		}
		args = append(args, startPoint)
	}
	_, err := g.run(args...)
	return err
}

// CheckoutBranch 切到已存在的分支(`git checkout <name>`)
// 工作树有会被覆盖的改动时 git 非零退出, 错误原样返回(该先 StashPush 或提交).
// 传远程跟踪分支名(如 "origin/main")时 git 会建一个同名本地分支并跟踪它.
func (g *GitOps) CheckoutBranch(name string) error {
	if err := validateGitArg("branch name", name); err != nil {
		return err
	}
	_, err := g.run("checkout", name)
	return err
}

// DeleteBranch 删除本地分支(`git branch -d|-D <name>`)
// force 为假时用 -d: 分支尚未合并进上游/当前分支时 git 会拒绝, 这是有意的保护;
// force 为真时用 -D, 无条件删除.
func (g *GitOps) DeleteBranch(name string, force bool) error {
	if err := validateGitArg("branch name", name); err != nil {
		return err
	}
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := g.run("branch", flag, name)
	return err
}

// GitRemote 是一个远程仓库及其 URL
// git 允许 fetch 与 push 指向不同 URL(remote.<name>.pushurl), 所以分成两个字段;
// 没有单独配 pushurl 时 PushURL 与 FetchURL 相同(git remote -v 会两行都打出来).
type GitRemote struct {
	Name     string
	FetchURL string
	PushURL  string
}

// ListRemotes 列出全部远程及其 URL(`git remote -v`)
func (g *GitOps) ListRemotes() ([]GitRemote, error) {
	out, err := g.run("remote", "-v")
	if err != nil {
		return nil, err
	}
	return ParseGitRemotes(out), nil
}

// ParseGitRemotes 解析 `git remote -v` 的输出(纯函数)
// 每行形如 "<name>\t<url> (fetch)" 或 "<name>\t<url> (push)", 同一个远程占两行,
// 这里按名字归并, 保持首次出现的顺序. 顺带兼容不带 -v 的纯名字列表(没有 TAB 的行),
// 那种情况下只有名字、URL 留空. 畸形行跳过并继续, 永远不 panic.
func ParseGitRemotes(out string) []GitRemote {
	var remotes []GitRemote
	at := make(map[string]int)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, rest, hasTab := strings.Cut(line, "\t")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// 行尾的 " (fetch)" / " (push)" 标记决定这个 URL 归哪一栏
		url := strings.TrimSpace(rest)
		kind := ""
		if i := strings.LastIndex(url, " ("); i >= 0 && strings.HasSuffix(url, ")") {
			kind = url[i+2 : len(url)-1]
			url = strings.TrimSpace(url[:i])
		}
		idx, seen := at[name]
		if !seen {
			remotes = append(remotes, GitRemote{Name: name})
			idx = len(remotes) - 1
			at[name] = idx
		}
		if !hasTab {
			continue
		}
		switch kind {
		case "fetch":
			remotes[idx].FetchURL = url
		case "push":
			remotes[idx].PushURL = url
		default:
			// 没有标记(比如 `git remote get-url` 风格的输出): 当 fetch URL 用
			if remotes[idx].FetchURL == "" {
				remotes[idx].FetchURL = url
			}
		}
	}
	return remotes
}

// AddRemote 添加一个远程(`git remote add <name> <url>`)
// 同名远程已存在时 git 非零退出, 错误原样返回.
func (g *GitOps) AddRemote(name, url string) error {
	if err := validateGitArg("remote", name); err != nil {
		return err
	}
	if err := validateGitArg("remote url", url); err != nil {
		return err
	}
	_, err := g.run("remote", "add", name, url)
	return err
}

// Status 返回工作树状态, 走 NUL 分隔的 `git status --porcelain=v1 -z`
// 与 core/git.go 的 GitStatusPorcelain 同一份 GitStatusEntry 结构, 区别在传输形态:
// -z 让 git 原样输出路径并以 NUL 收尾, 于是引号与 C 风格转义完全不参与, 含空格、TAB、
// 换行、非 ASCII 的路径都能原封不动拿到.
func (g *GitOps) Status() ([]GitStatusEntry, error) {
	out, err := g.run("status", "--porcelain=v1", "-z")
	if err != nil {
		return nil, err
	}
	return ParseGitPorcelainZ(out), nil
}

// ParseGitPorcelainZ 解析 `git status --porcelain=v1 -z` 的输出(纯函数)
// 每条记录是 "XY <path>" 后跟一个 NUL: X 是 index 列, Y 是工作树列, 第三字节是分隔空格.
// 重命名/复制(X 或 Y 为 'R'/'C')会多占一个字段 —— 紧跟其后的下一个 NUL 字段是旧路径,
// 注意顺序与不带 -z 的 "old -> new" 相反: -z 下先出现的是新路径.
// 路径原样输出, 因此不需要任何反转义.
// 长度不足或第三字节不是空格的畸形记录跳过并继续, 永远不 panic.
func ParseGitPorcelainZ(data string) []GitStatusEntry {
	fields := strings.Split(data, "\x00")
	var entries []GitStatusEntry
	for i := 0; i < len(fields); i++ {
		rec := fields[i]
		if len(rec) < 4 || rec[2] != ' ' {
			continue
		}
		e := GitStatusEntry{
			Staged:   rec[0],
			Unstaged: rec[1],
			Path:     rec[3:],
		}
		if isGitRenameStatus(e.Staged) || isGitRenameStatus(e.Unstaged) {
			if i+1 < len(fields) && fields[i+1] != "" {
				e.OrigPath = fields[i+1]
				i++
			}
		}
		entries = append(entries, e)
	}
	return entries
}

// ParseGitPorcelainLines 解析不带 -z 的 `git status --porcelain=v1` 输出(纯函数)
// 与 ParseGitPorcelainZ 同样产出 GitStatusEntry, 但要处理换行分隔形态下的两件麻烦事:
// 重命名行写作 "old -> new"(与 -z 的顺序相反), 且含特殊字符的路径被加引号并做了
// C 风格转义 —— 后者交给 DecodeGitQuotedPath 完整还原(core/git.go 的老解析只剥引号).
// 用途是解析已经拿到手的非 -z 文本; 自己发命令时优先用 Status(-z).
func ParseGitPorcelainLines(out string) []GitStatusEntry {
	var entries []GitStatusEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if len(line) < 4 || line[2] != ' ' {
			continue
		}
		e := GitStatusEntry{
			Staged:   line[0],
			Unstaged: line[1],
		}
		rest := line[3:]
		if orig, path, ok := splitGitRenameArrow(rest); ok {
			e.OrigPath = DecodeGitQuotedPath(orig)
			e.Path = DecodeGitQuotedPath(path)
		} else {
			e.Path = DecodeGitQuotedPath(rest)
		}
		entries = append(entries, e)
	}
	return entries
}

// isGitRenameStatus 报告状态字符是否表示重命名或复制(这两种才带旧路径)
func isGitRenameStatus(c byte) bool {
	return c == 'R' || c == 'C'
}

// splitGitRenameArrow 在引号之外找 " -> ", 把 "old -> new" 切成两半
// 直接 strings.Index(" -> ") 会被路径自身的箭头骗过去: git 只给含特殊字符的路径加引号,
// 于是 `R  "a -> b.txt" -> c.txt` 里第一个 " -> " 落在引号内部, naive 切法会得到
// orig=`"a` / path=`b.txt" -> c.txt`. 这里逐字节扫描, 只在引号外的位置切,
// 并且跳过引号内的 \x 转义对(否则 \" 会被当成收尾引号).
// 没找到分隔符时返回 ok=false.
func splitGitRenameArrow(rest string) (orig, path string, ok bool) {
	const arrow = " -> "
	inQuote := false
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '\\':
			if inQuote {
				i++
			}
		case '"':
			inQuote = !inQuote
		case ' ':
			if !inQuote && strings.HasPrefix(rest[i:], arrow) {
				return rest[:i], rest[i+len(arrow):], true
			}
		}
	}
	return "", "", false
}

// DecodeGitQuotedPath 还原 git 的 C 风格引号路径(纯函数)
// 路径含空格、控制字符或(core.quotePath 默认开启时)非 ASCII 字节时, git 会把整段用双引号
// 包起来并按 C 字符串规则转义: \a \b \f \n \r \t \v \" \\ 以及其余字节写成三位八进制 \ooo.
// 非 ASCII 就是逐字节 \ooo, 所以先还原成字节再拼回字符串, UTF-8 自然复原
// (例: "\346\265\213" → "测").
// 不以双引号包裹的输入原样返回 —— 普通路径不需要处理.
// 未知转义(如 \q)保留反斜杠与字符本身, 结尾孤立的反斜杠也保留: 宁可多一个反斜杠,
// 也不静默吞掉信息. 永远不 panic.
func DecodeGitQuotedPath(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	body := s[1 : len(s)-1]
	out := make([]byte, 0, len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '\\' {
			out = append(out, c)
			continue
		}
		i++
		if i >= len(body) {
			out = append(out, '\\')
			break
		}
		switch e := body[i]; e {
		case 'a':
			out = append(out, 0x07)
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'v':
			out = append(out, '\v')
		case '\\':
			out = append(out, '\\')
		case '"':
			out = append(out, '"')
		default:
			if isOctalDigit(e) && i+2 < len(body) && isOctalDigit(body[i+1]) && isOctalDigit(body[i+2]) {
				v := (int(e-'0') << 6) | (int(body[i+1]-'0') << 3) | int(body[i+2]-'0')
				out = append(out, byte(v))
				i += 2
			} else {
				out = append(out, '\\', e)
			}
		}
	}
	return string(out)
}

// isOctalDigit 报告 c 是否是八进制数字('0'..'7')
func isOctalDigit(c byte) bool {
	return c >= '0' && c <= '7'
}

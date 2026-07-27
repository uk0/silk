package ged

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/filesearch"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.GlobalSearchPanel", gui.TypeOf(GlobalSearchPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.GlobalSearchPanel",
		Name: "搜索",
		Icon: "search",
		Desc: "全局搜索",
	})
}

// SearchMatch represents a single search hit in a file.
type SearchMatch struct {
	FilePath string
	Line     int
	Column   int
	LineText string
	MatchLen int
}

// GlobalSearchPanel provides a search-across-files panel like VS Code's sidebar.
type GlobalSearchPanel struct {
	gui.Widget

	query       string
	queryRunes  []rune
	queryCursor int
	results     []SearchMatch
	grouped     map[string][]SearchMatch // grouped by file
	fileOrder   []string                 // ordered file keys
	collapsed   map[string]bool          // collapsed file groups

	replaceRunes  []rune // replace-with text
	replaceCursor int
	focusReplace  bool // true when the replace input holds keyboard focus

	// Engine options. filesearch already supports regex / case folding /
	// an extension filter; these hold the panel-side state that maps onto
	// filesearch.Options (see searchOptions). Whole-word has no engine flag
	// of its own — it is compiled into the pattern (see enginePattern).
	optRegex     bool
	optCaseSense bool
	optWholeWord bool
	include      []string // extensions to search, e.g. [".go", ".txt"]; empty = all
	exclude      []string // globs filtered out after the walk, e.g. ["*_test.go"]

	hoverIdx   int
	scrollY    float64
	rowHeight  float64
	searching  bool
	rootDir    string
	cbOpen     func(path string, line int)
	cbReplaced func(path, content string)

	// flattened visible rows for rendering
	flatRows []searchRow
}

type searchRowKind int

const (
	searchRowFile  searchRowKind = iota // file header row
	searchRowMatch                      // match result row
)

type searchRow struct {
	kind     searchRowKind
	filePath string
	match    *SearchMatch
}

func NewGlobalSearchPanel() *GlobalSearchPanel {
	p := new(GlobalSearchPanel)
	p.Init(p)
	return p
}

func (this *GlobalSearchPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
	this.collapsed = make(map[string]bool)
}

// SetRootDir sets the root directory for searching.
func (this *GlobalSearchPanel) SetRootDir(dir string) {
	abs, err := filepath.Abs(dir)
	if err == nil {
		dir = abs
	}
	this.rootDir = dir
}

// SigOpen registers a callback invoked when a search result is clicked.
func (this *GlobalSearchPanel) SigOpen(fn func(path string, line int)) {
	this.cbOpen = fn
}

// SigFileReplaced registers the callback fired once per file that a committed
// Replace All rewrote, with the file's new contents. The host uses it to reload
// the open buffer for that path so an editor never keeps stale text for a file
// that changed underneath it. It never fires for a transaction that rolled back.
func (this *GlobalSearchPanel) SigFileReplaced(fn func(path, content string)) {
	this.cbReplaced = fn
}

// SetRegex interprets the query as an RE2 regular expression.
func (this *GlobalSearchPanel) SetRegex(on bool) {
	this.optRegex = on
	this.Self().Update()
}

// SetCaseSensitive matches the query case-sensitively (the default is to fold
// case, as the panel always did).
func (this *GlobalSearchPanel) SetCaseSensitive(on bool) {
	this.optCaseSense = on
	this.Self().Update()
}

// SetWholeWord requires the match to sit on word boundaries. The engine has no
// flag for it, so it is compiled into the pattern as \b...\b — which forces
// regex mode on for that search even for a literal query.
func (this *GlobalSearchPanel) SetWholeWord(on bool) {
	this.optWholeWord = on
	this.Self().Update()
}

// SetInclude restricts the search to files with these extensions (a leading dot
// is optional, matching is case-insensitive — filesearch's own Include rules).
// An empty list searches every text file.
func (this *GlobalSearchPanel) SetInclude(exts []string) {
	this.include = append([]string(nil), exts...)
	this.Self().Update()
}

// SetExclude drops results whose path matches one of these globs. Each pattern
// is tested against the file's base name and against every path segment, so
// both "*_test.go" and "testdata" work. The engine has no exclude filter, so
// this is applied to its output (see excludedPath).
func (this *GlobalSearchPanel) SetExclude(patterns []string) {
	this.exclude = append([]string(nil), patterns...)
	this.Self().Update()
}

// searchOptions maps the panel's option state onto the engine's Options.
// Whole-word matching forces Regex on because it is expressed as \b...\b in the
// pattern; SkipBinary is always on (a binary blob has no useful text hits).
func (this *GlobalSearchPanel) searchOptions() filesearch.Options {
	return filesearch.Options{
		Regex:      this.optRegex || this.optWholeWord,
		IgnoreCase: !this.optCaseSense,
		Include:    this.include,
		SkipBinary: true,
	}
}

// enginePattern maps the user's query onto the pattern the engine matches.
// Without whole-word it is the query verbatim (literal or regex, as the engine's
// Regex flag says). With whole-word the query is wrapped in word boundaries —
// quoted first when it is a literal, and grouped when it is already a regex so
// an alternation like "a|b" becomes \b(?:a|b)\b rather than \ba|b\b.
func enginePattern(query string, regex, wholeWord bool) string {
	if !wholeWord || query == "" {
		return query
	}
	if regex {
		return `\b(?:` + query + `)\b`
	}
	return `\b` + regexp.QuoteMeta(query) + `\b`
}

// searchOptionLabel summarises the non-default options for the summary line,
// e.g. " · regex · Aa · .go". It returns "" when everything is at its default
// so the common case reads exactly as it did before.
func searchOptionLabel(opt filesearch.Options, wholeWord bool, exclude []string) string {
	var parts []string
	if opt.Regex && !wholeWord {
		parts = append(parts, "regex")
	}
	if wholeWord {
		parts = append(parts, "word")
	}
	if !opt.IgnoreCase {
		parts = append(parts, "Aa")
	}
	if len(opt.Include) > 0 {
		parts = append(parts, strings.Join(opt.Include, ","))
	}
	if len(exclude) > 0 {
		parts = append(parts, "!"+strings.Join(exclude, ","))
	}
	if len(parts) == 0 {
		return ""
	}
	return " · " + strings.Join(parts, " · ")
}

// Search runs a find-in-files search under rootDir for query. The filesystem
// walk is handed to the shared filesearch engine on a background goroutine and
// the results are marshalled back onto the main thread with gui.Post before any
// panel state is mutated, so a search triggered from the input never blocks the
// UI on disk I/O.
func (this *GlobalSearchPanel) Search(query string) {
	this.query = query
	this.queryRunes = []rune(query)
	this.queryCursor = len(this.queryRunes)
	this.results = nil
	this.grouped = make(map[string][]SearchMatch)
	this.fileOrder = nil
	this.scrollY = 0
	this.hoverIdx = -1

	if query == "" || this.rootDir == "" {
		this.rebuildFlatRows()
		this.Self().Update()
		return
	}

	this.searching = true
	this.Self().Update()

	root := this.rootDir
	pattern := enginePattern(query, this.optRegex, this.optWholeWord)
	opt := this.searchOptions()
	exclude := this.exclude
	go func() {
		matches := runSearchOpt(root, pattern, opt, exclude)
		gui.Post(func() {
			// A newer search may have superseded this one while it ran.
			if this.query != query {
				return
			}
			this.searching = false
			this.applyResults(matches)
			this.rebuildFlatRows()
			this.Self().Update()
		})
	}()
}

// runSearch runs the engine with the panel's original defaults: a
// case-insensitive literal scan of every text file. It is the no-options entry
// point (and what a freshly built panel does).
func runSearch(root, query string) []SearchMatch {
	return runSearchOpt(root, query, filesearch.Options{
		IgnoreCase: true,
		SkipBinary: true,
	}, nil)
}

// runSearchOpt runs the filesearch engine with explicit options and maps its
// matches onto SearchMatch. It is pure (touches no panel state and no GUI), so
// it is safe to call from a goroutine and directly from tests. filesearch
// reports a 1-based BYTE column; Column is stored 0-based in bytes — the same
// units Draw slices the line by — so search and highlight agree and multibyte
// text before a match no longer shifts the highlight. MatchLen comes from the
// matched text itself, not from len(pattern), because a regex or folded-case
// match can be a different length than the pattern. Binary files and
// .git/vendor/node_modules are skipped by the engine; exclude globs are applied
// here (the engine has no exclude filter).
func runSearchOpt(root, pattern string, opt filesearch.Options, exclude []string) []SearchMatch {
	matches, err := filesearch.Search(root, pattern, opt)
	if err != nil {
		return nil
	}
	re := searchMatcher(pattern, opt)
	out := make([]SearchMatch, 0, len(matches))
	for _, m := range matches {
		if excludedPath(m.Path, exclude) {
			continue
		}
		col := m.Col - 1
		out = append(out, SearchMatch{
			FilePath: m.Path,
			Line:     m.Line,
			Column:   col,
			LineText: m.Text,
			MatchLen: matchLenAt(re, pattern, m.Text, col),
		})
	}
	return out
}

// searchMatcher compiles the matcher filesearch uses for (pattern, opt) so a
// match's byte length can be recovered and occurrences counted. It returns nil
// for the literal case-sensitive path — there every match is len(pattern) — and
// for a pattern that does not compile (the engine already reported that error).
func searchMatcher(pattern string, opt filesearch.Options) *regexp.Regexp {
	expr := ""
	switch {
	case opt.Regex:
		expr = pattern
	case opt.IgnoreCase:
		expr = regexp.QuoteMeta(pattern)
	default:
		return nil
	}
	if opt.IgnoreCase {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil
	}
	return re
}

// matchLenAt returns the byte length of the match that starts at 0-based byte
// offset col in text. With no matcher (literal, case-sensitive) every match is
// the pattern's own length; that length is also the fallback when the matcher
// cannot re-anchor at col (e.g. a pattern anchored with ^).
func matchLenAt(re *regexp.Regexp, pattern, text string, col int) int {
	if re == nil || col < 0 || col > len(text) {
		return len(pattern)
	}
	if loc := re.FindStringIndex(text[col:]); loc != nil && loc[0] == 0 && loc[1] > 0 {
		return loc[1]
	}
	return len(pattern)
}

// countMatches counts the non-overlapping, non-empty matches of pattern in text.
func countMatches(re *regexp.Regexp, pattern, text string) int {
	if re == nil {
		return strings.Count(text, pattern)
	}
	n := 0
	for _, loc := range re.FindAllStringIndex(text, -1) {
		if loc[0] != loc[1] {
			n++
		}
	}
	return n
}

// excludedPath reports whether path is filtered out by any exclude glob. Each
// pattern is matched (filepath.Match) against every path segment, so both a
// file glob ("*_test.go") and a directory name ("testdata") work.
func excludedPath(path string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	segs := strings.Split(filepath.ToSlash(path), "/")
	for _, pat := range patterns {
		if pat == "" {
			continue
		}
		for _, seg := range segs {
			if ok, err := filepath.Match(pat, seg); err == nil && ok {
				return true
			}
		}
	}
	return false
}

// applyResults installs a fresh result set and rebuilds the per-file grouping.
// It must run on the main thread (Search marshals into it via gui.Post).
func (this *GlobalSearchPanel) applyResults(matches []SearchMatch) {
	this.results = matches
	this.grouped = make(map[string][]SearchMatch)
	this.fileOrder = nil
	for i := range matches {
		path := matches[i].FilePath
		if _, ok := this.grouped[path]; !ok {
			this.fileOrder = append(this.fileOrder, path)
		}
		this.grouped[path] = append(this.grouped[path], matches[i])
	}
}

// FileReplacement is one file's previewed rewrite: what is on disk now and what
// Replace All would put there. It is also the unit ApplyReplacements commits and
// the record it rolls back from, so Before is the exact text to restore.
type FileReplacement struct {
	Path   string // file to rewrite
	Before string // contents read from disk at preview time
	After  string // contents once the replacement is applied
	Count  int    // occurrences replaced in this file
}

// ComputeReplacements previews Replace All: for every file the last search
// matched it reads the file, applies the replacement with the same pattern and
// options the search used, and returns the before/after pair. It writes NOTHING
// — the caller shows the diff and then hands the slice to ApplyReplacements —
// so the destructive step is separated from the step that decides what to do.
// Files that come out byte-identical (no occurrence, or a replacement equal to
// the match) are left out entirely, so no file is touched for nothing.
//
// The replacement runs line by line — the same unit the search matches in — so a
// regex can never replace across a line boundary and anchors keep the meaning
// they had while searching.
func (this *GlobalSearchPanel) ComputeReplacements() []FileReplacement {
	if this.query == "" || len(this.fileOrder) == 0 {
		return nil
	}
	pattern := enginePattern(this.query, this.optRegex, this.optWholeWord)
	opt := this.searchOptions()
	re := searchMatcher(pattern, opt)
	replacement := string(this.replaceRunes)

	out := make([]FileReplacement, 0, len(this.fileOrder))
	for _, path := range this.fileOrder {
		data, err := os.ReadFile(path)
		if err != nil {
			core.Error(fmt.Sprintf("global search: read %s: %v", path, err))
			continue
		}
		before := string(data)
		after, n := replaceInText(before, pattern, replacement, opt, re)
		if n == 0 || after == before {
			continue
		}
		out = append(out, FileReplacement{Path: path, Before: before, After: after, Count: n})
	}
	return out
}

// ApplyReplacements commits a previewed set of rewrites as one transaction:
// either every file lands or none does. If a write fails, the files already
// written are restored from their Before text and the error is returned, so a
// failed Replace All never leaves the tree half-rewritten. Only on success does
// the SigFileReplaced callback fire — once per rewritten file — so a host that
// reloads open buffers from it never sees a state that was rolled back.
func (this *GlobalSearchPanel) ApplyReplacements(reps []FileReplacement) error {
	written := make([]FileReplacement, 0, len(reps))
	for _, r := range reps {
		if r.After == r.Before {
			continue // nothing to do; don't touch the file's mtime
		}
		if err := os.WriteFile(r.Path, []byte(r.After), 0644); err != nil {
			rollbackReplacements(written)
			return fmt.Errorf("global search: write %s: %w (rolled back %d file(s))",
				r.Path, err, len(written))
		}
		written = append(written, r)
	}
	if this.cbReplaced != nil {
		for _, r := range written {
			this.cbReplaced(r.Path, r.After)
		}
	}
	return nil
}

// rollbackReplacements restores each already-written file to its Before text,
// newest first. A rollback write that itself fails is logged, not swallowed:
// that file is the one a human has to look at.
func rollbackReplacements(written []FileReplacement) {
	for i := len(written) - 1; i >= 0; i-- {
		r := written[i]
		if err := os.WriteFile(r.Path, []byte(r.Before), 0644); err != nil {
			core.Error(fmt.Sprintf("global search: rollback %s: %v", r.Path, err))
		}
	}
}

// replaceInText rewrites every occurrence of pattern in text line by line,
// preserving each line's original terminator ("\n", "\r\n", or none at EOF).
// Working per line is what keeps regex anchors (^, $) and multiline flags
// behaving exactly as they did during the search, and guarantees a pattern can
// never be replaced across a line boundary. It returns the new text and the
// number of occurrences replaced.
func replaceInText(text, pattern, replacement string, opt filesearch.Options, re *regexp.Regexp) (string, int) {
	var b strings.Builder
	b.Grow(len(text))
	count := 0
	for rest := text; rest != ""; {
		line, term := rest, ""
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			line, term, rest = rest[:i], "\n", rest[i+1:]
			if strings.HasSuffix(line, "\r") {
				line, term = line[:len(line)-1], "\r\n"
			}
		} else {
			rest = ""
		}
		count += countMatches(re, pattern, line)
		b.WriteString(filesearch.Replace(filesearch.Match{Text: line}, pattern, replacement, opt))
		b.WriteString(term)
	}
	return b.String(), count
}

// ReplaceAll previews the rewrite and commits it in one transaction: nothing is
// written unless every file can be written (see ComputeReplacements /
// ApplyReplacements). A failed transaction is logged and leaves the tree exactly
// as it was. Afterwards the search is re-run synchronously so the results list
// reflects the state on disk either way.
func (this *GlobalSearchPanel) ReplaceAll() {
	reps := this.ComputeReplacements()
	if len(reps) == 0 {
		return
	}
	if err := this.ApplyReplacements(reps); err != nil {
		core.Error(err.Error())
	} else {
		n := 0
		for _, r := range reps {
			n += r.Count
		}
		core.Trace(fmt.Sprintf("global search: replaced %d occurrence(s) in %d file(s)", n, len(reps)))
	}

	// Refresh results against what is now on disk.
	pattern := enginePattern(this.query, this.optRegex, this.optWholeWord)
	this.applyResults(runSearchOpt(this.rootDir, pattern, this.searchOptions(), this.exclude))
	this.rebuildFlatRows()
	this.Self().Update()
}

// rebuildFlatRows flattens grouped results into a list of renderable rows.
func (this *GlobalSearchPanel) rebuildFlatRows() {
	this.flatRows = nil
	for _, filePath := range this.fileOrder {
		this.flatRows = append(this.flatRows, searchRow{
			kind:     searchRowFile,
			filePath: filePath,
		})
		if !this.collapsed[filePath] {
			matches := this.grouped[filePath]
			for i := range matches {
				this.flatRows = append(this.flatRows, searchRow{
					kind:     searchRowMatch,
					filePath: filePath,
					match:    &matches[i],
				})
			}
		}
	}
}

// totalMatchCount returns the total number of matches.
func (this *GlobalSearchPanel) totalMatchCount() int {
	return len(this.results)
}

// totalFileCount returns the number of files with matches.
func (this *GlobalSearchPanel) totalFileCount() int {
	return len(this.fileOrder)
}

// Draw renders the search panel.
func (this *GlobalSearchPanel) Draw(g paint.Painter) {
	w, h := this.Size()
	t := gui.Theme()

	// Background
	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Header
	headerH := 22.0
	g.SetBrush1(paint.Color{R: 235, G: 238, B: 245, A: 255})
	g.Rectangle(0, 0, w, headerH)
	g.Fill()
	g.SetPen1(paint.Color{R: 200, G: 200, B: 210, A: 255}, 1)
	g.MoveTo(0, headerH)
	g.LineTo(w, headerH)
	g.Stroke()

	headerFont := paint.NewFont(t.Font.Family(), 12, true, false)
	g.SetFont(headerFont)
	g.SetBrush1(t.TextColor)
	g.DrawText1(8, headerH-5, "搜索")

	normalFont := paint.NewFont(t.Font.Family(), 11, false, false)
	boldFont := paint.NewFont(t.Font.Family(), 11, true, false)
	smallFont := paint.NewFont(t.Font.Family(), 10, false, false)

	// Search input area
	inputY := headerH + 4
	inputH := 24.0
	inputX := 8.0
	inputW := w - 16

	g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
	g.Rectangle(inputX, inputY, inputW, inputH)
	g.Fill()
	// Highlight the border of the focused input.
	if this.focusReplace {
		g.SetPen1(paint.Color{R: 180, G: 185, B: 200, A: 255}, 1)
	} else {
		g.SetPen1(paint.Color{R: 100, G: 140, B: 200, A: 255}, 1)
	}
	g.Rectangle(inputX, inputY, inputW, inputH)
	g.Stroke()

	g.SetFont(normalFont)
	if len(this.queryRunes) > 0 {
		g.SetBrush1(t.TextColor)
		g.DrawText1(inputX+4, inputY+inputH-7, string(this.queryRunes))
	} else {
		g.SetBrush1(paint.Color{R: 160, G: 160, B: 170, A: 180})
		g.DrawText1(inputX+4, inputY+inputH-7, "Search... (Enter)")
	}

	// Replace input area (mirrors the search input, directly underneath).
	replaceY := inputY + inputH + 4
	replaceH := 24.0
	btnW := 64.0
	replaceW := inputW - btnW - 4

	g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
	g.Rectangle(inputX, replaceY, replaceW, replaceH)
	g.Fill()
	if this.focusReplace {
		g.SetPen1(paint.Color{R: 100, G: 140, B: 200, A: 255}, 1)
	} else {
		g.SetPen1(paint.Color{R: 180, G: 185, B: 200, A: 255}, 1)
	}
	g.Rectangle(inputX, replaceY, replaceW, replaceH)
	g.Stroke()

	g.SetFont(normalFont)
	if len(this.replaceRunes) > 0 {
		g.SetBrush1(t.TextColor)
		g.DrawText1(inputX+4, replaceY+replaceH-7, string(this.replaceRunes))
	} else {
		g.SetBrush1(paint.Color{R: 160, G: 160, B: 170, A: 180})
		g.DrawText1(inputX+4, replaceY+replaceH-7, "Replace...")
	}

	// Replace All button.
	btnX := inputX + replaceW + 4
	g.SetBrush1(paint.Color{R: 100, G: 140, B: 200, A: 255})
	g.Rectangle(btnX, replaceY, btnW, replaceH)
	g.Fill()
	g.SetFont(smallFont)
	g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
	btnLabel := "Replace All"
	btnExt := smallFont.TextExtents(btnLabel)
	g.DrawText1(btnX+(btnW-btnExt.XAdvance)/2, replaceY+replaceH-8, btnLabel)

	// Summary line
	summaryY := replaceY + replaceH + 4
	summaryH := 16.0

	if len(this.results) > 0 {
		g.SetFont(smallFont)
		g.SetBrush1(paint.Color{R: 120, G: 120, B: 140, A: 255})
		summary := fmt.Sprintf("%d matches in %d files%s", this.totalMatchCount(), this.totalFileCount(),
			searchOptionLabel(this.searchOptions(), this.optWholeWord, this.exclude))
		g.DrawText1(8, summaryY+summaryH-3, summary)
	} else if this.query != "" && !this.searching {
		g.SetFont(smallFont)
		g.SetBrush1(paint.Color{R: 160, G: 120, B: 120, A: 200})
		g.DrawText1(8, summaryY+summaryH-3, "No results found")
	}

	// Results list
	listY := summaryY + summaryH + 2
	rh := this.rowHeight
	startY := listY - this.scrollY

	for i, row := range this.flatRows {
		rowY := startY + float64(i)*rh
		if rowY+rh < listY || rowY > h {
			continue
		}

		// Hover highlight
		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 230, G: 235, B: 245, A: 255})
			g.Rectangle(0, rowY, w, rh)
			g.Fill()
		}

		switch row.kind {
		case searchRowFile:
			// File header
			relPath := row.filePath
			if this.rootDir != "" {
				if rel, err := filepath.Rel(this.rootDir, row.filePath); err == nil {
					relPath = rel
				}
			}

			// Expand/collapse triangle
			triX := 8.0
			triY := rowY + rh/2
			isCollapsed := this.collapsed[row.filePath]
			if isCollapsed {
				g.MoveTo(triX, triY-4)
				g.LineTo(triX+6, triY)
				g.LineTo(triX, triY+4)
			} else {
				g.MoveTo(triX, triY-3)
				g.LineTo(triX+6, triY-3)
				g.LineTo(triX+3, triY+3)
			}
			g.SetBrush1(paint.Color{R: 120, G: 120, B: 130, A: 255})
			g.Fill()

			// File name (bold)
			g.SetFont(boldFont)
			g.SetBrush1(t.TextColor)
			g.DrawText1(20, rowY+rh-5, filepath.Base(relPath))

			// Directory path (dimmed)
			dir := filepath.Dir(relPath)
			if dir != "." && dir != "" {
				nameExt := boldFont.TextExtents(filepath.Base(relPath))
				g.SetFont(smallFont)
				g.SetBrush1(paint.Color{R: 140, G: 140, B: 160, A: 200})
				g.DrawText1(20+nameExt.XAdvance+6, rowY+rh-5, dir)
			}

			// Match count badge
			matchCount := len(this.grouped[row.filePath])
			countStr := fmt.Sprintf("%d", matchCount)
			g.SetFont(smallFont)
			countExt := smallFont.TextExtents(countStr)
			badgeX := w - countExt.XAdvance - 16
			g.SetBrush1(paint.Color{R: 100, G: 140, B: 200, A: 200})
			g.DrawText1(badgeX, rowY+rh-5, countStr)

		case searchRowMatch:
			if row.match == nil {
				continue
			}
			m := row.match
			lineText := strings.TrimSpace(m.LineText)
			// Truncate long lines
			if len(lineText) > 120 {
				lineText = lineText[:120] + "..."
			}

			// Line number
			g.SetFont(smallFont)
			g.SetBrush1(paint.Color{R: 140, G: 140, B: 160, A: 200})
			lineStr := fmt.Sprintf("%d:", m.Line)
			g.DrawText1(24, rowY+rh-4, lineStr)

			// Line text with match highlight
			lineNumExt := smallFont.TextExtents(lineStr)
			textX := 24 + lineNumExt.XAdvance + 4

			// m.Column and m.MatchLen are BYTE offsets into m.LineText (the
			// filesearch engine reports byte columns). Leading indent is ASCII,
			// so the byte trim offset maps straight onto the trimmed line. Slice
			// lineText by bytes so multibyte text before the match does not
			// shift the highlight (search and highlight agree on byte offsets).
			trimOffset := len(m.LineText) - len(strings.TrimLeft(m.LineText, " \t"))
			matchCol := m.Column - trimOffset
			if matchCol < 0 {
				matchCol = 0
			}

			g.SetFont(normalFont)

			if matchCol >= 0 && matchCol < len(lineText) && matchCol+m.MatchLen <= len(lineText) {
				// Pre-match text
				if matchCol > 0 {
					pre := lineText[:matchCol]
					g.SetBrush1(t.TextColor)
					g.DrawText1(textX, rowY+rh-4, pre)
					preExt := normalFont.TextExtents(pre)
					textX += preExt.XAdvance
				}

				// Match highlight background
				matchText := lineText[matchCol : matchCol+m.MatchLen]
				matchExt := normalFont.TextExtents(matchText)
				g.SetBrush1(paint.Color{R: 255, G: 200, B: 60, A: 120})
				g.Rectangle(textX, rowY+2, matchExt.XAdvance, rh-4)
				g.Fill()

				// Match text
				g.SetBrush1(paint.Color{R: 180, G: 120, B: 20, A: 255})
				g.DrawText1(textX, rowY+rh-4, matchText)
				textX += matchExt.XAdvance

				// Post-match text
				postStart := matchCol + m.MatchLen
				if postStart < len(lineText) {
					post := lineText[postStart:]
					g.SetBrush1(t.TextColor)
					g.DrawText1(textX, rowY+rh-4, post)
				}
			} else {
				// Fallback: just draw the line
				g.SetBrush1(t.TextColor)
				g.DrawText1(textX, rowY+rh-4, lineText)
			}
		}
	}
}

// listTop returns the y coordinate where the results list begins. It accounts
// for the header, the search input, the replace input, and the summary line.
func (this *GlobalSearchPanel) listTop() float64 {
	headerH := 22.0
	inputH := 24.0
	replaceH := 24.0
	summaryH := 16.0
	return headerH + 4 + inputH + 4 + replaceH + 4 + summaryH + 2
}

// hitTest returns the row index for a given y coordinate, or -1.
func (this *GlobalSearchPanel) hitTest(y float64) int {
	listY := this.listTop()

	if y < listY {
		return -1
	}
	idx := int(math.Floor((y - listY + this.scrollY) / this.rowHeight))
	if idx < 0 || idx >= len(this.flatRows) {
		return -1
	}
	return idx
}

// isInSearchInput checks if a click is in the search input area.
func (this *GlobalSearchPanel) isInSearchInput(y float64) bool {
	headerH := 22.0
	inputY := headerH + 4
	inputH := 24.0
	return y >= inputY && y <= inputY+inputH
}

// isInReplaceInput checks if a click is in the replace input area (excluding
// the Replace All button to its right).
func (this *GlobalSearchPanel) isInReplaceInput(x, y float64) bool {
	headerH := 22.0
	inputH := 24.0
	replaceY := headerH + 4 + inputH + 4
	replaceH := 24.0
	w := this.Width()
	replaceW := (w - 16) - 64 - 4 // inputW - btnW - gap
	return y >= replaceY && y <= replaceY+replaceH && x >= 8 && x <= 8+replaceW
}

// isInReplaceButton checks if a click hits the Replace All button.
func (this *GlobalSearchPanel) isInReplaceButton(x, y float64) bool {
	headerH := 22.0
	inputH := 24.0
	replaceY := headerH + 4 + inputH + 4
	replaceH := 24.0
	w := this.Width()
	btnX := 8 + (w - 16) - 64 - 4 + 4 // inputX + replaceW + gap
	return y >= replaceY && y <= replaceY+replaceH && x >= btnX && x <= btnX+64
}

// OnLeftDown handles clicks.
func (this *GlobalSearchPanel) OnLeftDown(x, y float64) {
	this.SetFocus()

	if this.isInSearchInput(y) {
		this.focusReplace = false
		this.Self().Update()
		return
	}
	if this.isInReplaceInput(x, y) {
		this.focusReplace = true
		this.Self().Update()
		return
	}
	if this.isInReplaceButton(x, y) {
		this.ReplaceAll()
		return
	}

	idx := this.hitTest(y)
	if idx < 0 || idx >= len(this.flatRows) {
		return
	}

	row := this.flatRows[idx]
	switch row.kind {
	case searchRowFile:
		// Toggle collapse
		this.collapsed[row.filePath] = !this.collapsed[row.filePath]
		this.rebuildFlatRows()
	case searchRowMatch:
		if row.match != nil && this.cbOpen != nil {
			this.cbOpen(row.match.FilePath, row.match.Line)
		}
	}
	this.Self().Update()
}

// OnMouseMove updates hover.
func (this *GlobalSearchPanel) OnMouseMove(x, y float64) {
	idx := this.hitTest(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave resets hover.
func (this *GlobalSearchPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel handles scrolling.
func (this *GlobalSearchPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3
	listY := this.listTop()
	totalRows := float64(len(this.flatRows))
	maxScroll := totalRows*this.rowHeight - (this.Height() - listY)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// OnTextInput handles typing into the focused (search or replace) input.
func (this *GlobalSearchPanel) OnTextInput(s string) {
	if this.focusReplace {
		this.replaceRunes = append(this.replaceRunes, []rune(s)...)
		this.replaceCursor = len(this.replaceRunes)
	} else {
		this.queryRunes = append(this.queryRunes, []rune(s)...)
		this.queryCursor = len(this.queryRunes)
	}
	this.Self().Update()
}

// OnKeyDown handles keyboard input.
func (this *GlobalSearchPanel) OnKeyDown(key int, repeat bool) {
	switch key {
	case gui.KeyTab:
		// Toggle focus between the search and replace inputs.
		this.focusReplace = !this.focusReplace
		this.Self().Update()
	case gui.KeyBackSpace:
		if this.focusReplace {
			if len(this.replaceRunes) > 0 {
				this.replaceRunes = this.replaceRunes[:len(this.replaceRunes)-1]
				this.replaceCursor = len(this.replaceRunes)
				this.Self().Update()
			}
		} else if len(this.queryRunes) > 0 {
			this.queryRunes = this.queryRunes[:len(this.queryRunes)-1]
			this.queryCursor = len(this.queryRunes)
			this.Self().Update()
		}
	case gui.KeyEnter:
		// Enter in the replace field runs Replace All; otherwise it searches.
		if this.focusReplace {
			this.ReplaceAll()
		} else {
			this.Search(string(this.queryRunes))
		}
	case gui.KeyEsc:
		this.queryRunes = nil
		this.queryCursor = 0
		this.results = nil
		this.grouped = nil
		this.fileOrder = nil
		this.flatRows = nil
		this.focusReplace = false
		this.Self().Update()
	}
}

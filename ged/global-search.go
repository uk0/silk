package ged

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/filesearch"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/internal/filetxn"
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
	exclude      []string // basename/segment globs, e.g. ["*_test.go"]

	hoverIdx   int
	scrollY    float64
	rowHeight  float64
	searching  bool
	rootDir    string
	cbOpen     func(path string, line int)
	cbReplaced func(path, content string)

	// flattened visible rows for rendering
	flatRows                                  []searchRow
	controls                                  *globalSearchControls
	worker                                    *globalSearchWorker
	post                                      func(func())
	searchGeneration                          uint64
	searchReady, previewing, applying, closed bool
	searchErr                                 error
	report                                    filesearch.SearchResult
	notice                                    string
	preview                                   []FileReplacement
	selectedIdx                               int
	cbOpenMatch                               func(SearchMatch)
	cbPreview                                 func(FileReplacement)
	prepareReplace                            func([]FileReplacement) (func(error), error)
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
	this.selectedIdx = -1
	this.collapsed = make(map[string]bool)
}

// SetRootDir sets the root directory for searching.
func (this *GlobalSearchPanel) SetRootDir(dir string) {
	abs, err := filepath.Abs(dir)
	if dir != "" && err == nil {
		dir = abs
	}
	if this.applying || dir == this.rootDir {
		return
	}
	this.rootDir = dir
	this.invalidateSearch()
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
	if this.applying {
		return
	}
	this.optRegex = on
	this.invalidateSearch()
}

// SetCaseSensitive matches the query case-sensitively (the default is to fold
// case, as the panel always did).
func (this *GlobalSearchPanel) SetCaseSensitive(on bool) {
	if this.applying {
		return
	}
	this.optCaseSense = on
	this.invalidateSearch()
}

// SetWholeWord enables Unicode word boundaries for native asynchronous search.
func (this *GlobalSearchPanel) SetWholeWord(on bool) {
	if this.applying {
		return
	}
	this.optWholeWord = on
	this.invalidateSearch()
}

// SetInclude restricts the search to files with these extensions (a leading dot
// is optional, matching is case-insensitive — filesearch's own Include rules).
// An empty list searches every text file.
func (this *GlobalSearchPanel) SetInclude(exts []string) {
	if this.applying {
		return
	}
	this.include = append([]string(nil), exts...)
	if this.controls != nil {
		this.controls.include.SetText(strings.Join(exts, ", "))
	}
	this.invalidateSearch()
}

// SetExclude drops results whose path matches one of these globs. Each pattern
// is tested against the file's base name and against every path segment, so
// both "*_test.go" and "testdata" work. The native worker prunes before reading.
func (this *GlobalSearchPanel) SetExclude(patterns []string) {
	if this.applying {
		return
	}
	this.exclude = append([]string(nil), patterns...)
	if this.controls != nil {
		this.controls.exclude.SetText(strings.Join(patterns, ", "))
	}
	this.invalidateSearch()
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
	if this.applying || this.closed {
		return
	}
	this.query = query
	this.queryRunes = []rune(query)
	this.queryCursor = len(this.queryRunes)
	this.startSearch(false)
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

// ApplyReplacements synchronously commits an already computed SDK proposal.
// Before must still match disk. Ordinary failures roll back where safe; this
// is not crash-atomic and cannot lock out an unrelated writer. Failed rollback
// retains recovery copies. Native UI uses guarded asynchronous ApplyPreview.
func (this *GlobalSearchPanel) ApplyReplacements(reps []FileReplacement) error {
	if err := commitSearchReplacements(reps); err != nil {
		return err
	}
	if this.cbReplaced != nil {
		for _, r := range reps {
			if r.Before != r.After {
				this.cbReplaced(r.Path, r.After)
			}
		}
	}
	return nil
}

func commitSearchReplacements(reps []FileReplacement) error {
	changes := make([]filetxn.Change, 0, len(reps))
	for _, r := range reps {
		snap, err := filetxn.ReadLimit(r.Path, int64(len(r.Before))+1)
		if err != nil {
			return fmt.Errorf("global search preflight: %w", err)
		}
		if !snap.Exists() || string(snap.Bytes()) != r.Before {
			return fmt.Errorf("global search preflight: %w: %s", filetxn.ErrConflict, r.Path)
		}
		changes = append(changes, filetxn.Change{Before: snap, Data: []byte(r.After)})
	}
	return filetxn.Commit(changes...)
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

// ReplaceAll is the synchronous legacy SDK convenience method. Native UI uses
// cancellable PreviewReplacements followed by guarded asynchronous ApplyPreview.
// See ApplyReplacements for conflict and recovery limitations.
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

	normalFont := paint.NewFont(t.Font.Family(), 11, false, false)
	boldFont := paint.NewFont(t.Font.Family(), 11, true, false)
	smallFont := paint.NewFont(t.Font.Family(), 10, false, false)
	listY := this.listTop()
	rh := this.rowHeight
	g.Save()
	defer g.Restore()
	g.Rectangle(0, listY, w, max(0, h-listY))
	g.Clip()
	lo := max(0, int(this.scrollY/rh))
	hi := min(len(this.flatRows), int(math.Ceil((this.scrollY+h-listY)/rh))+1)
	for i := lo; i < hi; i++ {
		row := this.flatRows[i]
		rowY := listY - this.scrollY + float64(i)*rh
		// Hover highlight
		if i == this.hoverIdx || i == this.selectedIdx {
			g.SetBrush1(paint.Color{R: 100, G: 140, B: 200, A: 65})
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
			lineText, matchCol, matchEnd := searchSnippet(m.LineText, m.Column, m.MatchLen)

			// Line number
			g.SetFont(smallFont)
			g.SetBrush1(paint.Color{R: 140, G: 140, B: 160, A: 200})
			lineStr := fmt.Sprintf("%d:", m.Line)
			g.DrawText1(24, rowY+rh-4, lineStr)

			// Line text with match highlight
			lineNumExt := smallFont.TextExtents(lineStr)
			textX := 24 + lineNumExt.XAdvance + 4

			g.SetFont(normalFont)

			if matchCol >= 0 && matchCol < len(lineText) && matchEnd <= len(lineText) {
				// Pre-match text
				if matchCol > 0 {
					pre := lineText[:matchCol]
					g.SetBrush1(t.TextColor)
					g.DrawText1(textX, rowY+rh-4, pre)
					preExt := normalFont.TextExtents(pre)
					textX += preExt.XAdvance
				}

				// Match highlight background
				matchText := lineText[matchCol:matchEnd]
				matchExt := normalFont.TextExtents(matchText)
				g.SetBrush1(paint.Color{R: 255, G: 200, B: 60, A: 120})
				g.Rectangle(textX, rowY+2, matchExt.XAdvance, rh-4)
				g.Fill()

				// Match text
				g.SetBrush1(paint.Color{R: 180, G: 120, B: 20, A: 255})
				g.DrawText1(textX, rowY+rh-4, matchText)
				textX += matchExt.XAdvance

				// Post-match text
				postStart := matchEnd
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
	if this.controls != nil && this.controls.bottom > 0 {
		return this.controls.bottom
	}
	return 198
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

// OnLeftDown handles clicks.
func (this *GlobalSearchPanel) OnLeftDown(x, y float64) {
	this.Widget.SetFocus()
	this.activateRow(this.hitTest(y))
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
	if this.selectedIdx >= 0 {
		return
	}
	this.ensureControls()
	this.controls.query.OnTextInput(s)
}
func (this *GlobalSearchPanel) OnKeyDown(key int, repeat bool) {
	switch key {
	case gui.KeyUp:
		this.moveRow(-1)
	case gui.KeyDown:
		this.moveRow(1)
	case gui.KeyEnter:
		if this.selectedIdx >= 0 {
			this.activateRow(this.selectedIdx)
		} else {
			this.Search(this.query)
		}
	case gui.KeyEsc:
		this.CancelSearch()
	case gui.KeyTab:
		this.FocusSearch()
	}
}

// Build a bounded UTF-8-safe preview around the actual hit, not always column 0.
func searchSnippet(text string, column, length int) (string, int, int) {
	column = max(0, min(column, len(text)))
	endMatch := column + min(len(text)-column, max(0, length))
	start := column
	for n := 0; n < 30 && start > 0; n++ {
		_, w := utf8.DecodeLastRuneInString(text[:start])
		start -= w
	}
	end := start
	for n := 0; n < 120 && end < len(text); n++ {
		_, w := utf8.DecodeRuneInString(text[end:])
		end += w
	}
	prefix := ""
	if start > 0 {
		prefix = "…"
	}
	before := strings.ReplaceAll(text[start:column], "\t", "    ")
	hit := strings.ReplaceAll(text[column:min(end, endMatch)], "\t", "    ")
	after := strings.ReplaceAll(text[min(end, endMatch):end], "\t", "    ")
	if end < len(text) {
		after += "…"
	}
	return prefix + before + hit + after, len(prefix) + len(before), len(prefix) + len(before) + len(hit)
}

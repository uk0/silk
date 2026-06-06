package gui

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"os/exec"
	"silk/core"
	"silk/paint"
	"sort"
	"strings"
	"time"
	"unicode"
)

func init() {
	core.RegisterFactory("gui.CodeEditor", TypeOf((*CodeEditor)(nil)))
}

// tokenType classifies a lexical token for syntax coloring.
type tokenType int

const (
	tokNormal   tokenType = iota
	tokKeyword            // Go language keywords
	tokString             // string literals
	tokComment            // comments
	tokNumber             // numeric literals
	tokType               // built-in type names
	tokFunction           // function calls
)

// tokenColors maps each token type to a display color (dark-theme palette).
var tokenColors = map[tokenType]paint.Color{
	tokNormal:   {R: 200, G: 200, B: 210, A: 255},
	tokKeyword:  {R: 86, G: 156, B: 214, A: 255},  // blue
	tokString:   {R: 206, G: 145, B: 120, A: 255}, // orange/brown
	tokComment:  {R: 106, G: 153, B: 85, A: 255},  // green
	tokNumber:   {R: 181, G: 137, B: 214, A: 255}, // purple
	tokType:     {R: 78, G: 201, B: 176, A: 255},  // teal
	tokFunction: {R: 220, G: 220, B: 170, A: 255}, // light yellow
}

var goKeywords = map[string]bool{
	"func": true, "if": true, "else": true, "for": true, "range": true,
	"return": true, "switch": true, "case": true, "default": true,
	"break": true, "continue": true, "go": true, "defer": true,
	"var": true, "const": true, "type": true, "struct": true,
	"interface": true, "map": true, "chan": true, "select": true,
	"package": true, "import": true, "nil": true, "true": true, "false": true,
	"fallthrough": true, "goto": true,
}

var goTypes = map[string]bool{
	"string": true, "int": true, "int8": true, "int16": true, "int32": true,
	"int64": true, "uint": true, "uint8": true, "uint16": true, "uint32": true,
	"uint64": true, "uintptr": true, "float32": true, "float64": true,
	"complex64": true, "complex128": true,
	"bool": true, "byte": true, "rune": true, "error": true,
	"any": true,
}

// token represents a single colored fragment within a line.
type token struct {
	text string
	typ  tokenType
}

// cursorPos is a lightweight (line, col) pair used to track the location
// of an additional ("secondary") cursor in multi-cursor editing mode.
type cursorPos struct {
	line int
	col  int
}

// editAction records a single undoable edit operation.
type editAction struct {
	kind    int // 0=insert, 1=delete, 2=replace
	line    int
	col     int
	text    string // inserted text (kind 0,2) or deleted text (kind 1)
	oldText string // original text before replace (kind 2)
	stamp   time.Time
}

// findMatch records a single search hit location.
type findMatch struct {
	line int
	col  int
	end  int // end column (exclusive)
}

// CodeEditor is a syntax-highlighted code editing widget with line numbers,
// cursor, and basic editing support, designed for Go source code.
type CodeEditor struct {
	Widget
	text       string
	lines      []string
	scrollY    float64
	scrollX    float64
	cursorLine int
	cursorCol  int
	font       paint.Font
	lineHeight float64
	gutterW    float64

	// inBlockComment tracks whether we are inside a multi-line /* */ comment
	// at the start of each visible rendering pass.
	inBlockComment bool

	onChanged func(string)

	// cbWidgetClicked is called when the user clicks a line referencing a widget.
	cbWidgetClicked func(widgetName string)

	// --- Selection ---
	selStartLine int
	selStartCol  int
	selEndLine   int
	selEndCol    int
	hasSelection bool
	mouseDown    bool
	// double-click detection
	lastClickTime time.Time
	lastClickLine int
	lastClickCol  int

	// --- Undo/Redo ---
	undoStack []editAction
	redoStack []editAction

	// --- Find/Replace ---
	findActive     bool
	findText       string
	findCursor     int // cursor position within findText
	findMatches    []findMatch
	findCurrentIdx int
	findBarHeight  float64

	// --- Indentation Guides ---
	showIndentGuides bool

	// --- Code Completion ---
	completion *CompletionPopup

	// --- Symbol Navigation ---
	symbolPopup *SymbolPopup

	// --- Go To Line ---
	gotoLineActive bool
	gotoLineText   string
	gotoLineCursor int

	// --- Breadcrumb ---
	breadcrumbHeight float64

	// --- Minimap ---
	showMinimap bool

	// --- Word Wrap ---
	wordWrap bool

	// --- Status Bar ---
	statusBarHeight float64

	// --- Error Markers ---
	errorLines map[int]string // line number -> error message

	// --- Bookmarks ---
	bookmarks map[int]bool // bookmarked line numbers

	// --- Breakpoints ---
	// breakpoints holds lines (0-based, matching cursorLine/bookmarks) that carry
	// a debugger breakpoint. UI/state layer only: no debugger is wired up here, and
	// breakpoints are NOT re-mapped when lines are inserted/deleted (known limitation).
	breakpoints map[int]bool

	// --- Code Folding ---
	// foldedLines holds the start-line (0-based) of every region the user has
	// collapsed. The body of a folded region (start+1 .. end) is hidden from the
	// rendered view and from the visual-row math used for cursor/scroll. Like
	// breakpoints, this is a UI/state layer keyed 0-based and is NOT re-mapped on
	// insert/delete; foldRegions is recomputed from the line slice on demand.
	foldedLines map[int]bool

	// --- Hover error tooltip ---
	hoverErrorLine int // line currently hovered for error tooltip (-1 = none)

	// --- Rename Refactoring ---
	renameActive    bool
	renameText      string
	renameCursorPos int
	renameWord      string // original word being renamed

	// --- Code Navigation (Cmd+Click / Ctrl+Click) ---
	hoverLinkLine  int    // line of currently hovered link (-1 if none)
	hoverLinkStart int    // start column of hovered link word
	hoverLinkEnd   int    // end column of hovered link word
	hoverLinkWord  string // the identifier being hovered
	filePath       string // path of the file currently being edited
	navStack       NavigationStack
	cbNavigate     func(filePath string, line int) // callback for cross-file navigation

	// --- Git Gutter ---
	gitStatus map[int]GitLineStatus

	// --- Multi-Cursor Editing ---
	// Primary cursor is (cursorLine, cursorCol). additionalCursors stores
	// extra caret positions that receive the same text input / edits.
	additionalCursors []cursorPos
}

// NewCodeEditor creates a new code editor widget.
func NewCodeEditor() *CodeEditor {
	p := new(CodeEditor)
	p.Init(p)
	return p
}

func (this *CodeEditor) Init(iw IWidget) {
	this.Widget.Init(iw)
	this.font = paint.NewFont("Menlo", 13, false, false)
	this.gutterW = 50
	this.text = ""
	this.lines = []string{""}
	this.cursorLine = 0
	this.cursorCol = 0
	this.findBarHeight = 30
	this.showIndentGuides = true
	this.breadcrumbHeight = 22
	this.showMinimap = true
	this.wordWrap = false
	this.statusBarHeight = 20
	this.hoverErrorLine = -1
	this.hoverLinkLine = -1
	this.breakpoints = make(map[int]bool)
	this.foldedLines = make(map[int]bool)
}

// SetText replaces the entire editor content.
func (this *CodeEditor) SetText(s string) {
	this.text = s
	this.lines = strings.Split(s, "\n")
	if len(this.lines) == 0 {
		this.lines = []string{""}
	}
	this.cursorLine = 0
	this.cursorCol = 0
	this.scrollY = 0
	this.scrollX = 0
	this.clearSelection()
	this.additionalCursors = nil
	this.undoStack = nil
	this.redoStack = nil
	this.foldedLines = make(map[int]bool)
	this.Self().Update()
}

// Text returns the full editor content.
func (this *CodeEditor) Text() string {
	return strings.Join(this.lines, "\n")
}

// SetFont sets the editor's monospace font.
func (this *CodeEditor) SetFont(f paint.Font) {
	this.font = f
	this.Self().Update()
}

// SigChanged registers a callback invoked when text changes.
func (this *CodeEditor) SigChanged(fn func(string)) {
	this.onChanged = fn
}

// SigChangedFn returns the currently registered change callback, or nil.
func (this *CodeEditor) SigChangedFn() func(string) {
	return this.onChanged
}

// ScrollY returns the current vertical scroll position.
func (this *CodeEditor) ScrollY() float64 {
	return this.scrollY
}

// SetScrollY sets the vertical scroll position.
func (this *CodeEditor) SetScrollY(y float64) {
	this.scrollY = y
}

// CursorLine returns the current 0-based cursor line index.
func (this *CodeEditor) CursorLine() int {
	return this.cursorLine
}

// Lines returns the current editor lines.
func (this *CodeEditor) Lines() []string {
	return this.lines
}

// SigWidgetClicked registers a callback for widget-name clicks.
func (this *CodeEditor) SigWidgetClicked(fn func(string)) {
	this.cbWidgetClicked = fn
}

// SetFilePath stores the file path this editor is editing and refreshes git status.
func (this *CodeEditor) SetFilePath(path string) {
	this.filePath = path
	this.RefreshGitStatus()
}

// RefreshGitStatus re-runs git diff and updates the gutter markers.
func (this *CodeEditor) RefreshGitStatus() {
	if this.filePath != "" {
		this.gitStatus = GitDiff(this.filePath)
	} else {
		this.gitStatus = nil
	}
}

// GitStatus returns the git line status map (1-based line numbers).
func (this *CodeEditor) GitLineStatuses() map[int]GitLineStatus {
	return this.gitStatus
}

// FilePath returns the file path this editor is editing.
func (this *CodeEditor) FilePath() string {
	return this.filePath
}

// SetNavigateCallback sets the callback for cross-file navigation.
// The callback receives the target file path and line number.
func (this *CodeEditor) SetNavigateCallback(fn func(string, int)) {
	this.cbNavigate = fn
}

// pushNavPosition records the current position onto the navigation stack.
func (this *CodeEditor) pushNavPosition() {
	this.navStack.Push(NavPosition{
		FilePath: this.filePath,
		Line:     this.cursorLine,
		Column:   this.cursorCol,
		ScrollY:  this.scrollY,
	})
}

// NavGoBack navigates to the previous position in the navigation stack.
func (this *CodeEditor) NavGoBack() {
	pos, ok := this.navStack.GoBack()
	if !ok {
		return
	}
	if pos.FilePath != "" && pos.FilePath != this.filePath && this.cbNavigate != nil {
		this.cbNavigate(pos.FilePath, pos.Line)
		return
	}
	this.cursorLine = pos.Line
	this.cursorCol = pos.Column
	this.scrollY = pos.ScrollY
	this.clampCursor()
	this.ensureCursorVisible()
	this.Self().Update()
}

// NavGoForward navigates to the next position in the navigation stack.
func (this *CodeEditor) NavGoForward() {
	pos, ok := this.navStack.GoForward()
	if !ok {
		return
	}
	if pos.FilePath != "" && pos.FilePath != this.filePath && this.cbNavigate != nil {
		this.cbNavigate(pos.FilePath, pos.Line)
		return
	}
	this.cursorLine = pos.Line
	this.cursorCol = pos.Column
	this.scrollY = pos.ScrollY
	this.clampCursor()
	this.ensureCursorVisible()
	this.Self().Update()
}

// SetWordWrap toggles word wrap mode.
func (this *CodeEditor) SetWordWrap(on bool) {
	this.wordWrap = on
	this.Self().Update()
}

// IsWordWrap returns whether word wrap is enabled.
func (this *CodeEditor) IsWordWrap() bool {
	return this.wordWrap
}

// SetShowMinimap toggles the minimap display.
func (this *CodeEditor) SetShowMinimap(on bool) {
	this.showMinimap = on
	this.Self().Update()
}

// minimapWidth returns the minimap column width (0 if hidden).
func (this *CodeEditor) minimapWidth() float64 {
	if this.showMinimap {
		return 60
	}
	return 0
}

// ScrollToLine scrolls the editor so that the given line is visible.
func (this *CodeEditor) ScrollToLine(line int) {
	if line < 0 {
		line = 0
	}
	if line >= len(this.lines) {
		line = len(this.lines) - 1
	}
	fe := this.font.FontExtents()
	lh := fe.Height + 2
	_, h := this.Size()
	visibleLines := int((h - this.topOffset() - this.statusBarHeight) / lh)

	// Center the target line in the viewport if possible. Positions are in
	// visual-row space so folded bodies are skipped (no fold => row == line).
	targetRow := this.lineToVisualRow(line)
	targetScroll := float64(targetRow)*lh - float64(visibleLines/2)*lh
	if targetScroll < 0 {
		targetScroll = 0
	}
	maxScroll := float64(len(this.visibleLineIndices()))*lh - h
	if maxScroll < 0 {
		maxScroll = 0
	}
	if targetScroll > maxScroll {
		targetScroll = maxScroll
	}
	this.scrollY = targetScroll
	this.cursorLine = line
	this.cursorCol = 0
	this.Self().Update()
}

// FindLineContaining returns the first line number containing substr, or -1.
func (this *CodeEditor) FindLineContaining(substr string) int {
	for i, line := range this.lines {
		if strings.Contains(line, substr) {
			return i
		}
	}
	return -1
}

func (this *CodeEditor) fontExtents() *paint.FontExtents {
	return this.font.FontExtents()
}

// measureText returns the pixel width of text using the editor font.
func (this *CodeEditor) measureText(s string) float64 {
	if s == "" {
		return 0
	}
	ext := this.font.TextExtents(s)
	return ext.XAdvance
}

// clampCursor ensures cursor is within valid bounds.
func (this *CodeEditor) clampCursor() {
	if len(this.lines) == 0 {
		this.lines = []string{""}
	}
	if this.cursorLine < 0 {
		this.cursorLine = 0
	}
	if this.cursorLine >= len(this.lines) {
		this.cursorLine = len(this.lines) - 1
	}
	lineLen := len([]rune(this.lines[this.cursorLine]))
	if this.cursorCol < 0 {
		this.cursorCol = 0
	}
	if this.cursorCol > lineLen {
		this.cursorCol = lineLen
	}
}

// --- Multi-Cursor Helpers ---

// AddCursorAtLine adds a secondary caret at (line, col). The primary cursor
// remains at (cursorLine, cursorCol). Duplicate positions are ignored.
func (this *CodeEditor) AddCursorAtLine(line, col int) {
	if line < 0 || line >= len(this.lines) {
		return
	}
	lr := len([]rune(this.lines[line]))
	if col < 0 {
		col = 0
	}
	if col > lr {
		col = lr
	}
	// Skip if it coincides with the primary cursor.
	if line == this.cursorLine && col == this.cursorCol {
		return
	}
	for _, c := range this.additionalCursors {
		if c.line == line && c.col == col {
			return
		}
	}
	this.additionalCursors = append(this.additionalCursors, cursorPos{line: line, col: col})
}

// ClearAdditionalCursors removes all secondary carets, returning to single-cursor mode.
func (this *CodeEditor) ClearAdditionalCursors() {
	if len(this.additionalCursors) == 0 {
		return
	}
	this.additionalCursors = nil
	this.Self().Update()
}

// allCursors returns the combined list of cursor positions (primary first,
// then additional) sorted by (line, col).
func (this *CodeEditor) allCursors() []cursorPos {
	out := make([]cursorPos, 0, len(this.additionalCursors)+1)
	out = append(out, cursorPos{line: this.cursorLine, col: this.cursorCol})
	out = append(out, this.additionalCursors...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].line != out[j].line {
			return out[i].line < out[j].line
		}
		return out[i].col < out[j].col
	})
	return out
}

// clampCursorPos clamps a cursor position to valid bounds and returns the
// adjusted position.
func (this *CodeEditor) clampCursorPos(p cursorPos) cursorPos {
	if p.line < 0 {
		p.line = 0
	}
	if p.line >= len(this.lines) {
		p.line = len(this.lines) - 1
	}
	lr := len([]rune(this.lines[p.line]))
	if p.col < 0 {
		p.col = 0
	}
	if p.col > lr {
		p.col = lr
	}
	return p
}

// dedupCursors removes additional cursors that coincide with the primary
// cursor or with each other.
func (this *CodeEditor) dedupCursors() {
	if len(this.additionalCursors) == 0 {
		return
	}
	seen := map[cursorPos]bool{{line: this.cursorLine, col: this.cursorCol}: true}
	out := this.additionalCursors[:0]
	for _, c := range this.additionalCursors {
		c = this.clampCursorPos(c)
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	this.additionalCursors = out
}

// insertAtAllCursors inserts the given (single-line) text at every cursor
// position — primary and additional — processing them back-to-front so
// earlier cursor indices remain valid. After the insertion, each cursor's
// column is advanced by len(insertRunes). Other cursors on the same line to
// the right of an insertion point have their column shifted accordingly.
//
// Only single-line inserts are supported here; multi-line pastes fall back
// to the primary-cursor path.
func (this *CodeEditor) insertAtAllCursors(text string) {
	insertRunes := []rune(text)
	if len(insertRunes) == 0 {
		return
	}
	// Gather all cursor positions (primary + additional) sorted descending so
	// we can mutate lines without invalidating earlier positions.
	all := this.allCursors()
	sort.Slice(all, func(i, j int) bool {
		if all[i].line != all[j].line {
			return all[i].line > all[j].line
		}
		return all[i].col > all[j].col
	})
	// Insert at each cursor in reverse order.
	for _, c := range all {
		c = this.clampCursorPos(c)
		line := this.lines[c.line]
		runes := []rune(line)
		newRunes := make([]rune, 0, len(runes)+len(insertRunes))
		newRunes = append(newRunes, runes[:c.col]...)
		newRunes = append(newRunes, insertRunes...)
		newRunes = append(newRunes, runes[c.col:]...)
		this.lines[c.line] = string(newRunes)
	}
	// Advance every cursor past its insertion point. Because cursors on the
	// same line to the RIGHT of an earlier-inserted cursor also shifted, we
	// recompute each cursor's new column by counting how many insertions on
	// its line were at columns <= its own column.
	shiftCursor := func(p *cursorPos) {
		extra := 0
		for _, c := range all {
			if c.line == p.line && c.col < p.col {
				extra += len(insertRunes)
			}
		}
		// This cursor itself shifts by one insertion (its own).
		p.col += extra + len(insertRunes)
	}
	primary := cursorPos{line: this.cursorLine, col: this.cursorCol}
	shiftCursor(&primary)
	this.cursorLine = primary.line
	this.cursorCol = primary.col
	for i := range this.additionalCursors {
		shiftCursor(&this.additionalCursors[i])
	}
	this.dedupCursors()
}

// backspaceAtAllCursors deletes one rune to the left at every cursor. Cursors
// at column 0 and line 0 do nothing. Otherwise the cursor moves back one
// column (or joins the previous line if col == 0). Processed back-to-front.
func (this *CodeEditor) backspaceAtAllCursors() {
	all := this.allCursors()
	// Process descending so edits don't invalidate earlier positions.
	sort.Slice(all, func(i, j int) bool {
		if all[i].line != all[j].line {
			return all[i].line > all[j].line
		}
		return all[i].col > all[j].col
	})
	// For each deletion, remember (line, col) of the deletion point so we
	// can shift the surviving cursors afterwards.
	type delOp struct {
		line   int
		col    int // column of the deleted character
		joined bool
		prevLen int // length of previous line BEFORE join (only if joined)
	}
	var ops []delOp
	for _, c := range all {
		c = this.clampCursorPos(c)
		if c.col > 0 {
			runes := []rune(this.lines[c.line])
			newRunes := append(runes[:c.col-1], runes[c.col:]...)
			this.lines[c.line] = string(newRunes)
			ops = append(ops, delOp{line: c.line, col: c.col - 1})
		} else if c.line > 0 {
			prev := this.lines[c.line-1]
			prevLen := len([]rune(prev))
			this.lines[c.line-1] = prev + this.lines[c.line]
			this.lines = append(this.lines[:c.line], this.lines[c.line+1:]...)
			ops = append(ops, delOp{line: c.line, col: 0, joined: true, prevLen: prevLen})
		}
	}
	// Shift every cursor according to all ops.
	shift := func(p *cursorPos) {
		for _, op := range ops {
			if op.joined {
				// Line op.line was removed and merged into op.line-1.
				if p.line == op.line {
					p.line = op.line - 1
					p.col += op.prevLen
				} else if p.line > op.line {
					p.line--
				}
			} else {
				// Single-char delete on op.line at column op.col.
				if p.line == op.line && p.col > op.col {
					p.col--
				}
			}
		}
	}
	primary := cursorPos{line: this.cursorLine, col: this.cursorCol}
	shift(&primary)
	this.cursorLine = primary.line
	this.cursorCol = primary.col
	for i := range this.additionalCursors {
		shift(&this.additionalCursors[i])
	}
	// Re-clamp everything (lines may have shrunk) and dedup.
	this.clampCursor()
	for i := range this.additionalCursors {
		this.additionalCursors[i] = this.clampCursorPos(this.additionalCursors[i])
	}
	this.dedupCursors()
}

// deleteAtAllCursors applies forward-delete at every cursor position.
func (this *CodeEditor) deleteAtAllCursors() {
	all := this.allCursors()
	sort.Slice(all, func(i, j int) bool {
		if all[i].line != all[j].line {
			return all[i].line > all[j].line
		}
		return all[i].col > all[j].col
	})
	type delOp struct {
		line    int
		col     int
		joined  bool
		nextLen int // length of this line BEFORE join (only if joined)
	}
	var ops []delOp
	for _, c := range all {
		c = this.clampCursorPos(c)
		runes := []rune(this.lines[c.line])
		if c.col < len(runes) {
			newRunes := append(runes[:c.col], runes[c.col+1:]...)
			this.lines[c.line] = string(newRunes)
			ops = append(ops, delOp{line: c.line, col: c.col})
		} else if c.line < len(this.lines)-1 {
			curLen := len(runes)
			this.lines[c.line] = this.lines[c.line] + this.lines[c.line+1]
			this.lines = append(this.lines[:c.line+1], this.lines[c.line+2:]...)
			ops = append(ops, delOp{line: c.line, col: curLen, joined: true, nextLen: curLen})
		}
	}
	shift := func(p *cursorPos) {
		for _, op := range ops {
			if op.joined {
				// Line op.line+1 was merged into op.line.
				if p.line == op.line+1 {
					p.line = op.line
					p.col += op.nextLen
				} else if p.line > op.line+1 {
					p.line--
				}
			} else {
				if p.line == op.line && p.col > op.col {
					p.col--
				}
			}
		}
	}
	primary := cursorPos{line: this.cursorLine, col: this.cursorCol}
	shift(&primary)
	this.cursorLine = primary.line
	this.cursorCol = primary.col
	for i := range this.additionalCursors {
		shift(&this.additionalCursors[i])
	}
	this.clampCursor()
	for i := range this.additionalCursors {
		this.additionalCursors[i] = this.clampCursorPos(this.additionalCursors[i])
	}
	this.dedupCursors()
}

// selectNextOccurrence adds a cursor at the next occurrence of the current
// word (or currently selected text) after the primary cursor. Wraps around
// to the beginning if no match is found further down the file.
//
// This provides the VS Code "Cmd/Ctrl+D" multi-cursor behavior; only caret
// positions are tracked (no per-cursor selection).
func (this *CodeEditor) selectNextOccurrence() {
	needle := ""
	if this.hasSelection {
		needle = this.SelectedText()
	} else {
		this.clampCursor()
		runes := []rune(this.lines[this.cursorLine])
		if this.cursorCol < len(runes) {
			start, end := this.wordBoundsAt(this.cursorLine, this.cursorCol)
			if start != end {
				needle = string(runes[start:end])
				// Select the current word as the primary selection so the
				// user sees what they're matching.
				this.selStartLine = this.cursorLine
				this.selStartCol = start
				this.selEndLine = this.cursorLine
				this.selEndCol = end
				this.hasSelection = true
				this.cursorCol = end
			}
		}
	}
	if needle == "" || strings.ContainsRune(needle, '\n') {
		return
	}
	// Build a flat list of all occurrences.
	type hit struct {
		line int
		col  int
		end  int
	}
	var hits []hit
	for i, line := range this.lines {
		offset := 0
		for {
			idx := strings.Index(line[offset:], needle)
			if idx < 0 {
				break
			}
			sc := len([]rune(line[:offset+idx]))
			ec := sc + len([]rune(needle))
			hits = append(hits, hit{line: i, col: sc, end: ec})
			offset += idx + len(needle)
			if offset >= len(line) {
				break
			}
		}
	}
	if len(hits) == 0 {
		return
	}
	// Find the first hit strictly after the primary cursor that is not
	// already covered by an existing cursor.
	covered := map[cursorPos]bool{{line: this.cursorLine, col: this.cursorCol}: true}
	for _, c := range this.additionalCursors {
		covered[c] = true
	}
	pick := -1
	for i, h := range hits {
		pc := cursorPos{line: h.line, col: h.end}
		if covered[pc] {
			continue
		}
		if h.line > this.cursorLine || (h.line == this.cursorLine && h.col >= this.cursorCol) {
			pick = i
			break
		}
	}
	if pick < 0 {
		// Wrap around from the top.
		for i, h := range hits {
			pc := cursorPos{line: h.line, col: h.end}
			if covered[pc] {
				continue
			}
			pick = i
			break
		}
	}
	if pick < 0 {
		return
	}
	h := hits[pick]
	this.AddCursorAtLine(h.line, h.end)
	this.ensureCursorVisible()
	this.Self().Update()
}

// rebuildText syncs this.text from this.lines.
func (this *CodeEditor) rebuildText() {
	this.text = strings.Join(this.lines, "\n")
	if this.onChanged != nil {
		this.onChanged(this.text)
	}
}

// --- Selection Methods ---

// HasSelection returns true if text is selected.
func (this *CodeEditor) HasSelection() bool {
	return this.hasSelection
}

// selectionRange returns the normalized selection range (start before end).
func (this *CodeEditor) selectionRange() (startLine, startCol, endLine, endCol int) {
	sl, sc, el, ec := this.selStartLine, this.selStartCol, this.selEndLine, this.selEndCol
	if sl > el || (sl == el && sc > ec) {
		sl, sc, el, ec = el, ec, sl, sc
	}
	return sl, sc, el, ec
}

// SelectedText returns the currently selected text.
func (this *CodeEditor) SelectedText() string {
	if !this.hasSelection {
		return ""
	}
	sl, sc, el, ec := this.selectionRange()
	if sl == el {
		runes := []rune(this.lines[sl])
		if sc > len(runes) {
			sc = len(runes)
		}
		if ec > len(runes) {
			ec = len(runes)
		}
		return string(runes[sc:ec])
	}
	var sb strings.Builder
	// First line
	runes := []rune(this.lines[sl])
	if sc > len(runes) {
		sc = len(runes)
	}
	sb.WriteString(string(runes[sc:]))
	// Middle lines
	for i := sl + 1; i < el; i++ {
		sb.WriteByte('\n')
		sb.WriteString(this.lines[i])
	}
	// Last line
	sb.WriteByte('\n')
	runes = []rune(this.lines[el])
	if ec > len(runes) {
		ec = len(runes)
	}
	sb.WriteString(string(runes[:ec]))
	return sb.String()
}

// DeleteSelection removes the selected text and places cursor at start.
func (this *CodeEditor) DeleteSelection() {
	if !this.hasSelection {
		return
	}
	sl, sc, el, ec := this.selectionRange()
	if sl == el {
		runes := []rune(this.lines[sl])
		if sc > len(runes) {
			sc = len(runes)
		}
		if ec > len(runes) {
			ec = len(runes)
		}
		newRunes := append(runes[:sc], runes[ec:]...)
		this.lines[sl] = string(newRunes)
	} else {
		firstRunes := []rune(this.lines[sl])
		if sc > len(firstRunes) {
			sc = len(firstRunes)
		}
		lastRunes := []rune(this.lines[el])
		if ec > len(lastRunes) {
			ec = len(lastRunes)
		}
		merged := string(firstRunes[:sc]) + string(lastRunes[ec:])
		this.lines[sl] = merged
		this.lines = append(this.lines[:sl+1], this.lines[el+1:]...)
	}
	this.cursorLine = sl
	this.cursorCol = sc
	this.clearSelection()
	this.rebuildText()
}

// ReplaceSelection replaces selected text with new text.
func (this *CodeEditor) ReplaceSelection(text string) {
	if this.hasSelection {
		this.DeleteSelection()
	}
	if text != "" {
		pasteLines := strings.Split(text, "\n")
		if len(pasteLines) == 1 {
			this.insertTextAtCursor(text)
		} else {
			this.insertMultilineAtCursor(pasteLines)
		}
	}
}

// clearSelection removes the selection.
func (this *CodeEditor) clearSelection() {
	this.hasSelection = false
	this.selStartLine = 0
	this.selStartCol = 0
	this.selEndLine = 0
	this.selEndCol = 0
}

// setSelectionStart records the anchor point.
func (this *CodeEditor) setSelectionStart() {
	this.selStartLine = this.cursorLine
	this.selStartCol = this.cursorCol
}

// updateSelectionEnd updates the moving end of the selection.
func (this *CodeEditor) updateSelectionEnd() {
	this.selEndLine = this.cursorLine
	this.selEndCol = this.cursorCol
	this.hasSelection = !(this.selStartLine == this.selEndLine && this.selStartCol == this.selEndCol)
}

// --- Undo/Redo Methods ---

func (this *CodeEditor) pushUndo(a editAction) {
	a.stamp = time.Now()
	// Group rapid single-char inserts
	if a.kind == 0 && len(this.undoStack) > 0 {
		last := &this.undoStack[len(this.undoStack)-1]
		if last.kind == 0 && a.stamp.Sub(last.stamp) < 500*time.Millisecond &&
			a.line == last.line && a.col == last.col+len([]rune(last.text)) &&
			len([]rune(a.text)) == 1 && a.text != "\n" {
			last.text += a.text
			last.stamp = a.stamp
			this.redoStack = nil
			return
		}
	}
	this.undoStack = append(this.undoStack, a)
	if len(this.undoStack) > 500 {
		this.undoStack = this.undoStack[1:]
	}
	this.redoStack = nil
}

func (this *CodeEditor) undo() {
	if len(this.undoStack) == 0 {
		return
	}
	a := this.undoStack[len(this.undoStack)-1]
	this.undoStack = this.undoStack[:len(this.undoStack)-1]

	switch a.kind {
	case 0: // undo insert => delete the inserted text
		this.cursorLine = a.line
		this.cursorCol = a.col
		textRunes := []rune(a.text)
		// Delete len(textRunes) chars starting from (a.line, a.col)
		this.deleteRange(a.line, a.col, len(textRunes))
		this.redoStack = append(this.redoStack, a)
	case 1: // undo delete => re-insert the deleted text
		this.cursorLine = a.line
		this.cursorCol = a.col
		this.insertRawText(a.text)
		this.cursorLine = a.line
		this.cursorCol = a.col
		this.redoStack = append(this.redoStack, a)
	case 2: // undo replace => put oldText back
		this.cursorLine = a.line
		this.cursorCol = a.col
		newRunes := []rune(a.text)
		this.deleteRange(a.line, a.col, len(newRunes))
		this.insertRawText(a.oldText)
		this.cursorLine = a.line
		this.cursorCol = a.col
		this.redoStack = append(this.redoStack, a)
	case 3: // undo full text replace (rename refactoring)
		this.lines = strings.Split(a.oldText, "\n")
		if len(this.lines) == 0 {
			this.lines = []string{""}
		}
		this.cursorLine = a.line
		this.cursorCol = a.col
		this.redoStack = append(this.redoStack, a)
	}
	this.clearSelection()
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
}

func (this *CodeEditor) redo() {
	if len(this.redoStack) == 0 {
		return
	}
	a := this.redoStack[len(this.redoStack)-1]
	this.redoStack = this.redoStack[:len(this.redoStack)-1]

	switch a.kind {
	case 0: // redo insert => re-insert
		this.cursorLine = a.line
		this.cursorCol = a.col
		this.insertRawText(a.text)
		this.undoStack = append(this.undoStack, a)
	case 1: // redo delete => delete again
		this.cursorLine = a.line
		this.cursorCol = a.col
		this.deleteRange(a.line, a.col, len([]rune(a.text)))
		this.undoStack = append(this.undoStack, a)
	case 2: // redo replace => apply new text again
		this.cursorLine = a.line
		this.cursorCol = a.col
		oldRunes := []rune(a.oldText)
		this.deleteRange(a.line, a.col, len(oldRunes))
		this.insertRawText(a.text)
		this.undoStack = append(this.undoStack, a)
	case 3: // redo full text replace (rename refactoring)
		this.lines = strings.Split(a.text, "\n")
		if len(this.lines) == 0 {
			this.lines = []string{""}
		}
		this.cursorLine = a.line
		this.cursorCol = a.col
		this.undoStack = append(this.undoStack, a)
	}
	this.clearSelection()
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
}

// insertRawText inserts text at current cursor without recording undo.
func (this *CodeEditor) insertRawText(text string) {
	insertLines := strings.Split(text, "\n")
	if len(insertLines) == 1 {
		line := this.lines[this.cursorLine]
		runes := []rune(line)
		insertRunes := []rune(text)
		newRunes := make([]rune, 0, len(runes)+len(insertRunes))
		newRunes = append(newRunes, runes[:this.cursorCol]...)
		newRunes = append(newRunes, insertRunes...)
		newRunes = append(newRunes, runes[this.cursorCol:]...)
		this.lines[this.cursorLine] = string(newRunes)
		this.cursorCol += len(insertRunes)
	} else {
		this.insertMultilineAtCursor(insertLines)
	}
}

// deleteRange deletes count runes starting at (line, col), possibly across lines.
func (this *CodeEditor) deleteRange(line, col, count int) {
	for count > 0 && line < len(this.lines) {
		runes := []rune(this.lines[line])
		avail := len(runes) - col
		if avail >= count {
			newRunes := append(runes[:col], runes[col+count:]...)
			this.lines[line] = string(newRunes)
			count = 0
		} else {
			// Delete rest of this line + newline, merge with next
			this.lines[line] = string(runes[:col])
			count -= avail + 1 // +1 for the newline
			if line+1 < len(this.lines) {
				this.lines[line] += this.lines[line+1]
				this.lines = append(this.lines[:line+1], this.lines[line+2:]...)
			} else {
				count = 0
			}
		}
	}
}

// insertTextAtCursor inserts single-line text at cursor, no undo recording.
func (this *CodeEditor) insertTextAtCursor(text string) {
	line := this.lines[this.cursorLine]
	runes := []rune(line)
	insertRunes := []rune(text)
	newRunes := make([]rune, 0, len(runes)+len(insertRunes))
	newRunes = append(newRunes, runes[:this.cursorCol]...)
	newRunes = append(newRunes, insertRunes...)
	newRunes = append(newRunes, runes[this.cursorCol:]...)
	this.lines[this.cursorLine] = string(newRunes)
	this.cursorCol += len(insertRunes)
}

// insertMultilineAtCursor inserts multiple lines at cursor.
func (this *CodeEditor) insertMultilineAtCursor(pasteLines []string) {
	line := this.lines[this.cursorLine]
	runes := []rune(line)
	before := string(runes[:this.cursorCol])
	after := string(runes[this.cursorCol:])

	this.lines[this.cursorLine] = before + pasteLines[0]
	newLines := make([]string, 0, len(this.lines)+len(pasteLines))
	newLines = append(newLines, this.lines[:this.cursorLine+1]...)
	for _, pl := range pasteLines[1:] {
		newLines = append(newLines, pl)
	}
	lastIdx := len(newLines) - 1
	newLines[lastIdx] = newLines[lastIdx] + after
	newLines = append(newLines, this.lines[this.cursorLine+1:]...)
	this.lines = newLines
	this.cursorLine += len(pasteLines) - 1
	this.cursorCol = len([]rune(pasteLines[len(pasteLines)-1]))
}

// --- Find Methods ---

func (this *CodeEditor) findUpdateMatches() {
	this.findMatches = nil
	if this.findText == "" {
		return
	}
	needle := this.findText
	for i, line := range this.lines {
		offset := 0
		for {
			idx := strings.Index(line[offset:], needle)
			if idx < 0 {
				break
			}
			startCol := len([]rune(line[:offset+idx]))
			endCol := startCol + len([]rune(needle))
			this.findMatches = append(this.findMatches, findMatch{line: i, col: startCol, end: endCol})
			offset += idx + len(needle)
			if offset >= len(line) {
				break
			}
		}
	}
	if this.findCurrentIdx >= len(this.findMatches) {
		this.findCurrentIdx = 0
	}
}

func (this *CodeEditor) findNext() {
	if len(this.findMatches) == 0 {
		return
	}
	this.findCurrentIdx = (this.findCurrentIdx + 1) % len(this.findMatches)
	m := this.findMatches[this.findCurrentIdx]
	this.cursorLine = m.line
	this.cursorCol = m.col
	this.clearSelection()
	this.ensureCursorVisible()
	this.Self().Update()
}

func (this *CodeEditor) findPrev() {
	if len(this.findMatches) == 0 {
		return
	}
	this.findCurrentIdx--
	if this.findCurrentIdx < 0 {
		this.findCurrentIdx = len(this.findMatches) - 1
	}
	m := this.findMatches[this.findCurrentIdx]
	this.cursorLine = m.line
	this.cursorCol = m.col
	this.clearSelection()
	this.ensureCursorVisible()
	this.Self().Update()
}

// --- Bracket Matching ---

func (this *CodeEditor) findMatchingBracket() (int, int, bool) {
	if this.cursorLine >= len(this.lines) {
		return 0, 0, false
	}
	runes := []rune(this.lines[this.cursorLine])
	if this.cursorCol >= len(runes) {
		return 0, 0, false
	}
	ch := runes[this.cursorCol]

	openers := map[rune]rune{'{': '}', '(': ')', '[': ']'}
	closers := map[rune]rune{'}': '{', ')': '(', ']': '['}

	if match, ok := openers[ch]; ok {
		// Search forward
		depth := 1
		line := this.cursorLine
		col := this.cursorCol + 1
		for line < len(this.lines) {
			lr := []rune(this.lines[line])
			for col < len(lr) {
				if lr[col] == ch {
					depth++
				} else if lr[col] == match {
					depth--
					if depth == 0 {
						return line, col, true
					}
				}
				col++
			}
			line++
			col = 0
		}
	} else if match, ok := closers[ch]; ok {
		// Search backward
		depth := 1
		line := this.cursorLine
		col := this.cursorCol - 1
		for {
			if col < 0 {
				line--
				if line < 0 {
					break
				}
				col = len([]rune(this.lines[line])) - 1
				continue
			}
			lr := []rune(this.lines[line])
			if col < len(lr) {
				if lr[col] == ch {
					depth++
				} else if lr[col] == match {
					depth--
					if depth == 0 {
						return line, col, true
					}
				}
			}
			col--
		}
	}
	return 0, 0, false
}

// --- Line Operations ---

func (this *CodeEditor) duplicateLine() {
	this.clampCursor()
	dup := this.lines[this.cursorLine]
	newLines := make([]string, 0, len(this.lines)+1)
	newLines = append(newLines, this.lines[:this.cursorLine+1]...)
	newLines = append(newLines, dup)
	newLines = append(newLines, this.lines[this.cursorLine+1:]...)
	this.lines = newLines
	this.cursorLine++
	this.pushUndo(editAction{kind: 0, line: this.cursorLine, col: 0, text: dup + "\n"})
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
}

func (this *CodeEditor) deleteCurrentLine() {
	this.clampCursor()
	deleted := this.lines[this.cursorLine]
	if len(this.lines) > 1 {
		this.pushUndo(editAction{kind: 1, line: this.cursorLine, col: 0, text: deleted + "\n"})
		this.lines = append(this.lines[:this.cursorLine], this.lines[this.cursorLine+1:]...)
		if this.cursorLine >= len(this.lines) {
			this.cursorLine = len(this.lines) - 1
		}
	} else {
		this.pushUndo(editAction{kind: 1, line: 0, col: 0, text: deleted})
		this.lines[0] = ""
	}
	this.cursorCol = 0
	this.clearSelection()
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
}

func (this *CodeEditor) moveLineUp() {
	this.clampCursor()
	if this.cursorLine <= 0 {
		return
	}
	this.lines[this.cursorLine], this.lines[this.cursorLine-1] = this.lines[this.cursorLine-1], this.lines[this.cursorLine]
	this.cursorLine--
	this.clearSelection()
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
}

func (this *CodeEditor) moveLineDown() {
	this.clampCursor()
	if this.cursorLine >= len(this.lines)-1 {
		return
	}
	this.lines[this.cursorLine], this.lines[this.cursorLine+1] = this.lines[this.cursorLine+1], this.lines[this.cursorLine]
	this.cursorLine++
	this.clearSelection()
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
}

// --- Comment Toggling ---

// commentPrefix is the line-comment token inserted/removed by ToggleLineComment.
// Go source uses "// "; the trailing space is part of the inserted text but is
// treated as optional when detecting/removing an existing comment.
const commentPrefix = "// "

// toggleComment applies a line-comment toggle to the given lines and returns the
// transformed slice (the input is not mutated). It is a pure helper so the
// comment logic can be unit-tested without any GL/widget state.
//
// Semantics (Qt Creator / VS Code "Toggle Line Comment"):
//   - Blank / whitespace-only lines are ignored: they never count toward the
//     "already commented?" decision and are never commented.
//   - If EVERY non-blank line already starts (after leading whitespace) with the
//     comment token, the token is removed from each (also swallowing one trailing
//     space, so "//x" and "// x" both uncomment cleanly).
//   - Otherwise the token is inserted at each non-blank line's first
//     non-whitespace column, preserving indentation.
//   - A range containing only blank lines is returned unchanged.
func toggleComment(lines []string, prefix string) []string {
	out := make([]string, len(lines))
	copy(out, lines)

	// The bare comment token without a trailing space, used for detection and
	// removal (e.g. "//" from "// ").
	bare := strings.TrimRight(prefix, " ")

	// Decide direction: comment unless every non-blank line is already commented.
	allCommented := true
	anyNonBlank := false
	for _, ln := range out {
		trimmed := strings.TrimLeft(ln, " \t")
		if trimmed == "" {
			continue // blank line: ignored for the decision
		}
		anyNonBlank = true
		if !strings.HasPrefix(trimmed, bare) {
			allCommented = false
			break
		}
	}
	if !anyNonBlank {
		return out // nothing to do for an all-blank range
	}

	if allCommented {
		// Uncomment: strip the leading comment token (and one following space).
		for i, ln := range out {
			indent := len(ln) - len(strings.TrimLeft(ln, " \t"))
			rest := ln[indent:]
			if !strings.HasPrefix(rest, bare) {
				continue // blank line, untouched
			}
			rest = rest[len(bare):]
			rest = strings.TrimPrefix(rest, " ")
			out[i] = ln[:indent] + rest
		}
	} else {
		// Comment: insert the token at the first non-whitespace column.
		for i, ln := range out {
			if strings.TrimLeft(ln, " \t") == "" {
				continue // blank line, never commented
			}
			indent := len(ln) - len(strings.TrimLeft(ln, " \t"))
			out[i] = ln[:indent] + prefix + ln[indent:]
		}
	}
	return out
}

// ToggleLineComment toggles "// " line comments on the current line, or on every
// line spanned by the active selection (Cmd/Ctrl+/). It delegates the transform
// to the pure toggleComment helper, then fires the changed callback and repaints.
func (this *CodeEditor) ToggleLineComment() {
	this.clampCursor()
	startLine := this.cursorLine
	endLine := this.cursorLine
	if this.hasSelection {
		sl, _, el, _ := this.selectionRange()
		startLine = sl
		endLine = el
	}

	// Remember whether a selection was active so we can restore a sensible one
	// after the edit (the column offsets shift, but spanning the same lines is
	// what users expect).
	hadSelection := this.hasSelection

	transformed := toggleComment(this.lines[startLine:endLine+1], commentPrefix)
	copy(this.lines[startLine:endLine+1], transformed)

	if hadSelection {
		this.selStartLine = startLine
		this.selStartCol = 0
		this.selEndLine = endLine
		this.selEndCol = len([]rune(this.lines[endLine]))
		this.hasSelection = true
		this.cursorLine = endLine
		this.cursorCol = this.selEndCol
	} else {
		// Keep the caret on the same line; clamp the column to the new length.
		this.clearSelection()
		this.cursorLine = startLine
		lineLen := len([]rune(this.lines[startLine]))
		if this.cursorCol > lineLen {
			this.cursorCol = lineLen
		}
	}

	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
}

// --- Word detection for double-click ---

func (this *CodeEditor) wordBoundsAt(line, col int) (int, int) {
	if line >= len(this.lines) {
		return col, col
	}
	runes := []rune(this.lines[line])
	if col >= len(runes) {
		return len(runes), len(runes)
	}
	ch := runes[col]
	if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' {
		start := col
		end := col
		for start > 0 && (unicode.IsLetter(runes[start-1]) || unicode.IsDigit(runes[start-1]) || runes[start-1] == '_') {
			start--
		}
		for end < len(runes) && (unicode.IsLetter(runes[end]) || unicode.IsDigit(runes[end]) || runes[end] == '_') {
			end++
		}
		return start, end
	}
	return col, col + 1
}

// ensureCursorVisible scrolls so the cursor line is within the viewport.
func (this *CodeEditor) ensureCursorVisible() {
	fe := this.font.FontExtents()
	lh := fe.Height + 2
	_, h := this.Size()
	h -= this.topOffset() + this.statusBarHeight
	visibleLines := int(h / lh)
	if visibleLines < 1 {
		visibleLines = 1
	}

	// Vertical scroll is measured in visual rows so folded bodies don't count;
	// with no active fold cursorRow == cursorLine and this is unchanged.
	cursorRow := this.lineToVisualRow(this.cursorLine)
	startRow := int(this.scrollY / lh)
	if cursorRow < startRow {
		this.scrollY = float64(cursorRow) * lh
	} else if cursorRow >= startRow+visibleLines {
		this.scrollY = float64(cursorRow-visibleLines+1) * lh
	}
	if this.scrollY < 0 {
		this.scrollY = 0
	}

	// Horizontal: ensure cursor column is visible
	w, _ := this.Size()
	textAreaW := w - this.gutterW - 10 - this.minimapWidth()
	if textAreaW < 20 {
		textAreaW = 20
	}
	this.clampCursor()
	cursorPrefix := ""
	if this.cursorLine < len(this.lines) {
		runes := []rune(this.lines[this.cursorLine])
		if this.cursorCol <= len(runes) {
			cursorPrefix = string(runes[:this.cursorCol])
		}
	}
	cursorPxX := this.measureText(cursorPrefix)
	hMargin := 20.0
	if cursorPxX-this.scrollX < 0 {
		this.scrollX = cursorPxX - hMargin
	} else if cursorPxX-this.scrollX > textAreaW-hMargin {
		this.scrollX = cursorPxX - textAreaW + hMargin
	}
	if this.scrollX < 0 {
		this.scrollX = 0
	}
}

// ----- Drawing -----

func (this *CodeEditor) Draw(g paint.Painter) {
	g.Save()
	defer g.Restore()

	w, h := this.Size()
	mmW := this.minimapWidth()
	sbH := this.statusBarHeight

	// Dark editor background
	g.SetBrush1(paint.Color{R: 30, G: 30, B: 35, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	// Total top offset: find bar + goto-line bar + breadcrumb
	topOff := this.topOffset()
	editorRight := w - mmW // right edge of the main text area
	editorBottom := h - sbH

	// Gutter background
	g.SetBrush1(paint.Color{R: 40, G: 40, B: 48, A: 255})
	g.Rectangle(0, topOff, this.gutterW, editorBottom-topOff)
	g.Fill()

	// Gutter right border
	g.SetPen1(paint.Color{R: 55, G: 55, B: 65, A: 255}, 1)
	g.Line(this.gutterW, topOff, this.gutterW, editorBottom)
	g.Stroke()

	fe := this.font.FontExtents()
	lh := fe.Height + 2

	// Folding: vis is the ordered list of line indices currently drawn (folded
	// bodies removed). Vertical positions are computed in visual-row space, so a
	// line's y is its row among vis, not its raw index. With no active fold
	// vis[r] == r and this collapses to the original 1:1 mapping.
	vis := this.visibleLineIndices()
	// foldStartEnd maps each foldable region's start line to its end line, so the
	// render loop can draw a ▸/▾ marker and (when collapsed) a folded-line hint.
	foldRegions := this.FoldRegions()
	foldStartEnd := make(map[int]int, len(foldRegions))
	for _, reg := range foldRegions {
		foldStartEnd[reg.startLine] = reg.endLine
	}
	startRow := int(this.scrollY / lh)
	if startRow < 0 {
		startRow = 0
	}
	startLine := 0
	if startRow < len(vis) {
		startLine = vis[startRow]
	}
	visibleLines := int((editorBottom-topOff)/lh) + 2

	g.SetFont(this.font)

	// Determine block-comment state up to startLine.
	inBlock := false
	for i := 0; i < startLine && i < len(this.lines); i++ {
		inBlock = lineEndsInBlockComment(this.lines[i], inBlock)
	}

	// Bracket matching
	matchBLine, matchBCol, hasMatchB := this.findMatchingBracket()

	// Selection range (normalized)
	var sSelL, sSelC, eSelL, eSelC int
	if this.hasSelection {
		sSelL, sSelC, eSelL, eSelC = this.selectionRange()
	}

	// textOffX is the base x for text content, accounting for horizontal scroll
	textOffX := this.gutterW + 10 - this.scrollX

	// Clip the main editor area to avoid drawing into minimap/status bar
	g.Save()
	g.Rectangle(0, topOff, editorRight, editorBottom-topOff)
	g.Clip()

	for row := startRow; row < startRow+visibleLines && row < len(vis); row++ {
		i := vis[row]
		y := float64(row)*lh - this.scrollY + topOff

		// Error line background tint (draw before current line highlight)
		if _, hasErr := this.errorLines[i]; hasErr {
			g.SetBrush1(paint.Color{R: 255, G: 220, B: 220, A: 80})
			g.Rectangle(this.gutterW, y, editorRight-this.gutterW, lh)
			g.Fill()
		}

		// Current line highlight
		if i == this.cursorLine && !this.hasSelection {
			g.SetBrush1(paint.Color{R: 45, G: 45, B: 60, A: 255})
			g.Rectangle(this.gutterW, y, editorRight-this.gutterW, lh)
			g.Fill()
		}

		// Draw selection highlight
		if this.hasSelection && i >= sSelL && i <= eSelL {
			lineRunes := []rune(this.lines[i])
			var selStartX, selEndX float64
			if i == sSelL {
				sc := sSelC
				if sc > len(lineRunes) {
					sc = len(lineRunes)
				}
				selStartX = textOffX + this.measureText(string(lineRunes[:sc]))
			} else {
				selStartX = textOffX
			}
			if i == eSelL {
				ec := eSelC
				if ec > len(lineRunes) {
					ec = len(lineRunes)
				}
				selEndX = textOffX + this.measureText(string(lineRunes[:ec]))
			} else {
				selEndX = textOffX + this.measureText(string(lineRunes)) + 8
			}
			if selEndX > selStartX {
				g.SetBrush1(paint.Color{R: 60, G: 100, B: 180, A: 255})
				g.Rectangle(selStartX, y, selEndX-selStartX, lh)
				g.Fill()
			}
		}

		// Draw find match highlights
		if this.findActive && len(this.findMatches) > 0 {
			for idx, m := range this.findMatches {
				if m.line == i {
					lineRunes := []rune(this.lines[i])
					mc := m.col
					me := m.end
					if mc > len(lineRunes) {
						mc = len(lineRunes)
					}
					if me > len(lineRunes) {
						me = len(lineRunes)
					}
					mx := textOffX + this.measureText(string(lineRunes[:mc]))
					mw := this.measureText(string(lineRunes[mc:me]))
					if idx == this.findCurrentIdx {
						g.SetBrush1(paint.Color{R: 255, G: 152, B: 0, A: 120})
					} else {
						g.SetBrush1(paint.Color{R: 255, G: 235, B: 59, A: 80})
					}
					g.Rectangle(mx, y, mw, lh)
					g.Fill()
				}
			}
		}

		// Indentation guides
		if this.showIndentGuides {
			this.drawIndentGuides(g, i, y, lh)
		}

		// Gutter indicators: error circle and bookmark diamond
		gutterCenterY := y + lh/2

		// Error indicator: red filled circle in gutter
		if _, hasErr := this.errorLines[i]; hasErr {
			g.SetBrush1(paint.Color{R: 220, G: 50, B: 50, A: 230})
			g.Arc(8.0, gutterCenterY, 3.5, 0, 2*math.Pi)
			g.Fill()
		}

		// Bookmark indicator: blue filled circle in gutter
		if this.bookmarks[i] {
			g.SetBrush1(paint.Color{R: 60, G: 130, B: 230, A: 230})
			bx := 18.0
			br := 3.5
			g.Arc(bx, gutterCenterY, br, 0, 2*math.Pi)
			g.Fill()
		}

		// Breakpoint indicator: red filled circle at the gutter's left edge
		// (VS Code / Qt Creator style). Drawn at the same y as the line number.
		if this.breakpoints[i] {
			g.SetBrush1(paint.Color{R: 230, G: 60, B: 60, A: 255})
			g.Arc(10.0, gutterCenterY, 4.5, 0, 2*math.Pi)
			g.Fill()
		}

		// Fold marker: a small triangle at the right edge of the gutter for every
		// foldable region start. ▾ (down) when expanded, ▸ (right) when collapsed
		// (Qt Creator / VS Code style). Clicking it is handled in OnLeftDown.
		if _, foldable := foldStartEnd[i]; foldable {
			g.SetBrush1(paint.Color{R: 150, G: 150, B: 165, A: 230})
			fx := this.gutterW - 4 // right edge of the marker, near the text area
			s := 3.5               // half-size of the triangle
			if this.foldedLines[i] {
				// Collapsed: right-pointing triangle ▸
				g.MoveTo(fx-s, gutterCenterY-s)
				g.LineTo(fx, gutterCenterY)
				g.LineTo(fx-s, gutterCenterY+s)
			} else {
				// Expanded: down-pointing triangle ▾
				g.MoveTo(fx-s, gutterCenterY-s)
				g.LineTo(fx+s, gutterCenterY-s)
				g.LineTo(fx, gutterCenterY+s)
			}
			g.Fill()
		}

		// Git gutter indicator (colored bar on left edge of gutter)
		if gs, ok := this.gitStatus[i+1]; ok { // gitStatus uses 1-based line numbers
			switch gs {
			case GitAdded:
				g.SetBrush1(paint.Color{R: 80, G: 200, B: 120, A: 230})
				g.Rectangle(1, y, 3, lh)
				g.Fill()
			case GitModified:
				g.SetBrush1(paint.Color{R: 70, G: 140, B: 220, A: 230})
				g.Rectangle(1, y, 3, lh)
				g.Fill()
			case GitDeleted:
				// Red filled triangle marker for deleted lines
				g.SetBrush1(paint.Color{R: 220, G: 60, B: 60, A: 230})
				tx := 2.5
				ty := y + lh/2
				g.MoveTo(tx, ty-3)
				g.LineTo(tx+5, ty)
				g.LineTo(tx, ty+3)
				g.LineTo(tx, ty-3)
				g.Fill()
			}
		}

		// Line number
		g.SetFont(this.font)
		g.SetBrush1(paint.Color{R: 100, G: 100, B: 120, A: 255})
		numStr := fmt.Sprintf("%d", i+1)
		numExt := this.font.TextExtents(numStr)
		numX := this.gutterW - numExt.XAdvance - 8
		g.DrawText1(numX, y+fe.Ascent, numStr)

		// Syntax-highlighted line
		lineText := this.lines[i]
		// Word wrap indicator: if line exceeds visible area, show marker
		if this.wordWrap {
			textAreaW := editorRight - this.gutterW - 10
			if textAreaW < 20 {
				textAreaW = 20
			}
			lineW := this.measureText(lineText)
			if lineW > textAreaW {
				// Draw wrap indicator at right edge
				g.SetBrush1(paint.Color{R: 120, G: 120, B: 140, A: 180})
				g.DrawText1(editorRight-14, y+fe.Ascent, "\u00BB")
			}
		}
		inBlock = this.drawHighlightedLine(g, lineText, textOffX, y+fe.Ascent, inBlock)

		// Folded-region hint: on a collapsed start line, draw a subtle "⋯}" after
		// the text so the user sees the block is collapsed and where it ends.
		if end, foldable := foldStartEnd[i]; foldable && this.foldedLines[i] {
			hintX := textOffX + this.measureText(lineText) + 8
			hint := "⋯" // ⋯ (midline horizontal ellipsis)
			if end >= 0 && end < len(this.lines) {
				if last, ok := lastNonSpaceRune(this.lines[end]); ok && last == '}' {
					hint = "⋯ }"
				}
			}
			g.SetBrush1(paint.Color{R: 120, G: 120, B: 140, A: 200})
			g.SetFont(this.font)
			g.DrawText1(hintX, y+fe.Ascent, hint)
		}

		// Hover link: blue underline + blue text overlay for Cmd/Ctrl+hover word
		if i == this.hoverLinkLine && this.hoverLinkStart < this.hoverLinkEnd {
			lineRunes := []rune(this.lines[i])
			if this.hoverLinkEnd <= len(lineRunes) {
				linkX := textOffX + this.measureText(string(lineRunes[:this.hoverLinkStart]))
				linkW := this.measureText(string(lineRunes[this.hoverLinkStart:this.hoverLinkEnd]))
				linkWord := string(lineRunes[this.hoverLinkStart:this.hoverLinkEnd])
				// Draw the word in blue (override the syntax color)
				g.SetBrush1(paint.Color{R: 80, G: 160, B: 255, A: 255})
				g.SetFont(this.font)
				g.DrawText1(linkX, y+fe.Ascent, linkWord)
				// Draw underline
				underY := y + fe.Ascent + 2
				g.SetPen1(paint.Color{R: 80, G: 160, B: 255, A: 255}, 1.0)
				g.MoveTo(linkX, underY)
				g.LineTo(linkX+linkW, underY)
				g.Stroke()
			}
		}

		// Error underline: red squiggly line under the text
		if _, hasErr := this.errorLines[i]; hasErr {
			lineW := this.measureText(strings.TrimRight(lineText, " \t"))
			if lineW < 20 {
				lineW = 20
			}
			underY := y + fe.Ascent + 2
			g.SetPen1(paint.Color{R: 220, G: 50, B: 50, A: 200}, 1.2)
			// Draw squiggly underline using small wave segments
			sx := textOffX
			wave := 2.0
			for sx < textOffX+lineW {
				g.MoveTo(sx, underY)
				g.LineTo(sx+wave, underY-wave)
				g.LineTo(sx+wave*2, underY)
				g.Stroke()
				sx += wave * 2
			}
		}

		// Bracket matching highlight
		if hasMatchB && i == matchBLine {
			lineRunes := []rune(this.lines[i])
			if matchBCol < len(lineRunes) {
				bx := textOffX + this.measureText(string(lineRunes[:matchBCol]))
				bw := this.measureText(string(lineRunes[matchBCol : matchBCol+1]))
				g.SetPen1(paint.Color{R: 80, G: 140, B: 220, A: 200}, 1.5)
				g.Rectangle(bx, y, bw, lh)
				g.Stroke()
			}
		}
		// Also highlight the bracket under cursor
		if hasMatchB && i == this.cursorLine {
			lineRunes := []rune(this.lines[i])
			if this.cursorCol < len(lineRunes) {
				bx := textOffX + this.measureText(string(lineRunes[:this.cursorCol]))
				bw := this.measureText(string(lineRunes[this.cursorCol : this.cursorCol+1]))
				g.SetPen1(paint.Color{R: 80, G: 140, B: 220, A: 200}, 1.5)
				g.Rectangle(bx, y, bw, lh)
				g.Stroke()
			}
		}
	}

	// Draw cursor(s). Primary cursor is rendered at full brightness; any
	// additional multi-cursors are drawn slightly dimmer so the user can
	// tell them apart at a glance.
	if this.HasFocus() {
		this.clampCursor()
		// Primary cursor
		{
			line := this.lines[this.cursorLine]
			runes := []rune(line)
			col := this.cursorCol
			if col > len(runes) {
				col = len(runes)
			}
			prefix := string(runes[:col])
			cx := textOffX + this.measureText(prefix)
			cy := float64(this.lineToVisualRow(this.cursorLine))*lh - this.scrollY + topOff
			g.SetBrush1(paint.Color{R: 200, G: 200, B: 230, A: 255})
			g.Rectangle(cx, cy, 1.5, lh)
			g.Fill()
		}
		// Additional cursors
		for _, c := range this.additionalCursors {
			cc := this.clampCursorPos(c)
			runes := []rune(this.lines[cc.line])
			col := cc.col
			if col > len(runes) {
				col = len(runes)
			}
			prefix := string(runes[:col])
			cx := textOffX + this.measureText(prefix)
			cy := float64(this.lineToVisualRow(cc.line))*lh - this.scrollY + topOff
			// Dim color: same hue, half alpha.
			g.SetBrush1(paint.Color{R: 200, G: 200, B: 230, A: 160})
			g.Rectangle(cx, cy, 1.5, lh)
			g.Fill()
		}
	}

	g.Restore() // pop editor clip

	// --- Draw minimap ---
	if this.showMinimap && mmW > 0 {
		this.drawMinimap(g, editorRight, topOff, mmW, editorBottom-topOff, lh, startLine, visibleLines)
	}

	// --- Draw status bar ---
	this.drawStatusBar(g, w, h, sbH)

	// Draw completion popup
	if this.completion != nil && this.completion.visible {
		this.completion.drawPopup(g, this)
	}

	// Draw find bar at top
	if this.findActive {
		this.drawFindBar(g, w)
	}

	// Draw goto-line bar at top (below find bar if active)
	if this.gotoLineActive {
		g.Save()
		if this.findActive {
			g.Translate(0, this.findBarHeight)
		}
		this.drawGotoLine(g, w)
		g.Restore()
	}

	// Draw breadcrumb bar (below find bar and goto-line bar)
	bcY := 0.0
	if this.findActive {
		bcY += this.findBarHeight
	}
	if this.gotoLineActive {
		bcY += this.findBarHeight
	}
	this.drawBreadcrumb(g, bcY, w)

	// Draw rename input overlay
	if this.renameActive {
		this.drawRenameInput(g, topOff, editorRight)
	}

	// Draw symbol popup overlay (on top of everything)
	if this.symbolPopup != nil && this.symbolPopup.visible {
		this.symbolPopup.drawPopup(g, this)
	}
}

// drawFindBar draws the find/search bar at the top of the editor.
func (this *CodeEditor) drawFindBar(g paint.Painter, w float64) {
	fbH := this.findBarHeight
	// Background
	g.SetBrush1(paint.Color{R: 50, G: 50, B: 60, A: 255})
	g.Rectangle(0, 0, w, fbH)
	g.Fill()
	// Bottom border
	g.SetPen1(paint.Color{R: 70, G: 70, B: 85, A: 255}, 1)
	g.Line(0, fbH, w, fbH)
	g.Stroke()

	fe := this.font.FontExtents()
	g.SetFont(this.font)

	// "Find:" label
	g.SetBrush1(paint.Color{R: 180, G: 180, B: 190, A: 255})
	g.DrawText1(10, fbH/2+fe.Ascent/2-1, "Find:")

	// Input background
	inputX := 60.0
	inputW := 250.0
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(inputX, 4, inputW, fbH-8)
	g.Fill()
	g.SetPen1(paint.Color{R: 80, G: 80, B: 100, A: 255}, 1)
	g.Rectangle(inputX, 4, inputW, fbH-8)
	g.Stroke()

	// Find text
	g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
	if this.findText != "" {
		g.DrawText1(inputX+4, fbH/2+fe.Ascent/2-1, this.findText)
	}

	// Find cursor
	findPrefix := ""
	if this.findCursor <= len([]rune(this.findText)) {
		findPrefix = string([]rune(this.findText)[:this.findCursor])
	}
	fcx := inputX + 4 + this.measureText(findPrefix)
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 230, A: 255})
	g.Rectangle(fcx, 6, 1, fbH-12)
	g.Fill()

	// Match count
	countStr := fmt.Sprintf("%d of %d", 0, 0)
	if len(this.findMatches) > 0 {
		countStr = fmt.Sprintf("%d of %d", this.findCurrentIdx+1, len(this.findMatches))
	}
	g.SetBrush1(paint.Color{R: 140, G: 140, B: 155, A: 255})
	g.DrawText1(inputX+inputW+10, fbH/2+fe.Ascent/2-1, countStr)
}

// drawIndentGuides draws vertical indent guide lines for a given editor line.
func (this *CodeEditor) drawIndentGuides(g paint.Painter, lineIdx int, y, lh float64) {
	if lineIdx >= len(this.lines) {
		return
	}
	line := this.lines[lineIdx]
	tabWidth := this.measureText("    ") // 4-space equivalent

	// Count indent levels: each tab = 1 level, every 4 spaces = 1 level
	level := 0
	spaces := 0
	for _, r := range line {
		if r == '\t' {
			if spaces > 0 {
				level += spaces / 4
				spaces = 0
			}
			level++
		} else if r == ' ' {
			spaces++
		} else {
			break
		}
	}
	level += spaces / 4

	guideColor := paint.Color{R: 60, G: 60, B: 70, A: 100}
	for i := 1; i <= level; i++ {
		gx := this.gutterW + 10 - this.scrollX + float64(i)*tabWidth
		// Draw dotted vertical line (small segments every 3px)
		for dy := 0.0; dy < lh; dy += 3.0 {
			g.SetBrush1(guideColor)
			g.Rectangle(gx, y+dy, 1, 1.5)
			g.Fill()
		}
	}
}

// drawHighlightedLine renders a single line with syntax coloring.
// It returns the updated inBlockComment state after this line.
func (this *CodeEditor) drawHighlightedLine(g paint.Painter, line string, x, y float64, inBlock bool) bool {
	tokens, newInBlock := tokenizeLine(line, inBlock)
	for _, tok := range tokens {
		c, ok := tokenColors[tok.typ]
		if !ok {
			c = tokenColors[tokNormal]
		}
		g.SetBrush1(c)
		g.DrawText1(x, y, tok.text)
		ext := this.font.TextExtents(tok.text)
		x += ext.XAdvance
	}
	return newInBlock
}

// drawMinimap renders a scaled-down code overview on the right edge of the editor.
func (this *CodeEditor) drawMinimap(g paint.Painter, x, y, mmW, mmH, lh float64, startLine, visibleLines int) {
	// Background
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(x, y, mmW, mmH)
	g.Fill()

	// Left border
	g.SetPen1(paint.Color{R: 45, G: 45, B: 55, A: 255}, 1)
	g.Line(x, y, x, y+mmH)
	g.Stroke()

	totalLines := len(this.lines)
	if totalLines == 0 {
		return
	}

	// Calculate line height in minimap: scale to fit or use minimum 1px
	minimapLineH := mmH / float64(totalLines)
	if minimapLineH > 3 {
		minimapLineH = 3
	}
	if minimapLineH < 1 {
		minimapLineH = 1
	}

	// Determine block-comment state for minimap rendering
	inBlock := false
	maxVisibleMinimap := int(mmH / minimapLineH)
	if maxVisibleMinimap > totalLines {
		maxVisibleMinimap = totalLines
	}

	// Map token type to minimap color
	mmTokenColor := func(typ tokenType) paint.Color {
		switch typ {
		case tokKeyword:
			return paint.Color{R: 86, G: 156, B: 214, A: 200}
		case tokString:
			return paint.Color{R: 206, G: 145, B: 120, A: 200}
		case tokComment:
			return paint.Color{R: 106, G: 153, B: 85, A: 200}
		case tokNumber:
			return paint.Color{R: 181, G: 137, B: 214, A: 200}
		case tokType:
			return paint.Color{R: 78, G: 201, B: 176, A: 200}
		case tokFunction:
			return paint.Color{R: 220, G: 220, B: 170, A: 200}
		default:
			return paint.Color{R: 140, G: 140, B: 150, A: 100}
		}
	}

	mmScale := mmW - 6 // horizontal scale for line content
	for i := 0; i < maxVisibleMinimap; i++ {
		lineY := y + float64(i)*minimapLineH
		line := this.lines[i]

		tokens, newBlock := tokenizeLine(line, inBlock)
		inBlock = newBlock

		// Draw each token as a colored horizontal bar
		barX := x + 3
		for _, tok := range tokens {
			text := tok.text
			// Estimate proportional width based on character count
			charW := mmScale * float64(len([]rune(text))) / 80.0
			if charW < 1 {
				charW = 1
			}
			if charW > mmScale-barX+x+3 {
				charW = mmScale - barX + x + 3
			}
			if tok.typ != tokNormal || strings.TrimSpace(text) != "" {
				c := mmTokenColor(tok.typ)
				g.SetBrush1(c)
				g.Rectangle(barX, lineY, charW, minimapLineH)
				g.Fill()
			}
			barX += charW
			if barX > x+mmW-3 {
				break
			}
		}
	}

	// Draw viewport indicator (semi-transparent blue overlay showing visible range)
	vpStartY := y + float64(startLine)*minimapLineH
	vpH := float64(visibleLines) * minimapLineH
	if vpStartY+vpH > y+mmH {
		vpH = y + mmH - vpStartY
	}
	g.SetBrush1(paint.Color{R: 60, G: 100, B: 180, A: 50})
	g.Rectangle(x, vpStartY, mmW, vpH)
	g.Fill()
	// Viewport border
	g.SetPen1(paint.Color{R: 80, G: 130, B: 210, A: 100}, 1)
	g.Rectangle(x, vpStartY, mmW, vpH)
	g.Stroke()
}

// drawStatusBar renders the editor status bar at the bottom.
func (this *CodeEditor) drawStatusBar(g paint.Painter, w, h, sbH float64) {
	y := h - sbH

	// Background
	g.SetBrush1(paint.Color{R: 40, G: 40, B: 48, A: 255})
	g.Rectangle(0, y, w, sbH)
	g.Fill()

	// Top border
	g.SetPen1(paint.Color{R: 55, G: 55, B: 65, A: 255}, 1)
	g.Line(0, y, w, y)
	g.Stroke()

	smallFont := paint.NewFont("Menlo", 11, false, false)
	g.SetFont(smallFont)
	sfe := smallFont.FontExtents()
	textY := y + sfe.Ascent + (sbH-sfe.Height)/2

	// Line:Column
	g.SetBrush1(paint.Color{R: 170, G: 170, B: 185, A: 255})
	lnCol := fmt.Sprintf("Ln %d, Col %d", this.cursorLine+1, this.cursorCol+1)
	g.DrawText1(10, textY, lnCol)

	// Character count / Line count
	charCount := len([]rune(this.text))
	lineCount := len(this.lines)
	countStr := fmt.Sprintf("%d chars, %d lines", charCount, lineCount)
	lnColExt := smallFont.TextExtents(lnCol)
	g.SetBrush1(paint.Color{R: 130, G: 130, B: 145, A: 255})
	g.DrawText1(lnColExt.XAdvance+30, textY, countStr)

	// Right-aligned items
	rightX := w - 10.0

	// Word wrap status
	wrapStr := "Wrap: Off"
	if this.wordWrap {
		wrapStr = "Wrap: On"
	}
	wrapExt := smallFont.TextExtents(wrapStr)
	rightX -= wrapExt.XAdvance
	g.SetBrush1(paint.Color{R: 130, G: 130, B: 145, A: 255})
	g.DrawText1(rightX, textY, wrapStr)

	rightX -= 20

	// Language
	langStr := "Go"
	langExt := smallFont.TextExtents(langStr)
	rightX -= langExt.XAdvance
	g.SetBrush1(paint.Color{R: 130, G: 130, B: 145, A: 255})
	g.DrawText1(rightX, textY, langStr)

	rightX -= 20

	// Encoding
	encStr := "UTF-8"
	encExt := smallFont.TextExtents(encStr)
	rightX -= encExt.XAdvance
	g.SetBrush1(paint.Color{R: 130, G: 130, B: 145, A: 255})
	g.DrawText1(rightX, textY, encStr)
}

// ----- Tokenizer -----

// tokenizeLine performs simple lexical analysis of a single Go source line.
// inBlock indicates whether we are inside a block comment from a prior line.
// Returns the token list and the updated inBlock state.
func tokenizeLine(line string, inBlock bool) ([]token, bool) {
	var tokens []token
	runes := []rune(line)
	n := len(runes)
	i := 0

	addTok := func(text string, typ tokenType) {
		if text != "" {
			tokens = append(tokens, token{text, typ})
		}
	}

	if inBlock {
		// Continue scanning for end of block comment.
		start := i
		for i < n {
			if i+1 < n && runes[i] == '*' && runes[i+1] == '/' {
				i += 2
				addTok(string(runes[start:i]), tokComment)
				inBlock = false
				goto normalScan
			}
			i++
		}
		addTok(string(runes[start:]), tokComment)
		return tokens, true
	}

normalScan:
	for i < n {
		ch := runes[i]

		// Line comment
		if ch == '/' && i+1 < n && runes[i+1] == '/' {
			addTok(string(runes[i:]), tokComment)
			return tokens, inBlock
		}

		// Block comment start
		if ch == '/' && i+1 < n && runes[i+1] == '*' {
			start := i
			i += 2
			for i < n {
				if i+1 < n && runes[i] == '*' && runes[i+1] == '/' {
					i += 2
					addTok(string(runes[start:i]), tokComment)
					goto normalScan
				}
				i++
			}
			addTok(string(runes[start:]), tokComment)
			return tokens, true
		}

		// Double-quoted string
		if ch == '"' {
			start := i
			i++
			for i < n && runes[i] != '"' {
				if runes[i] == '\\' && i+1 < n {
					i++
				}
				i++
			}
			if i < n {
				i++ // skip closing quote
			}
			addTok(string(runes[start:i]), tokString)
			continue
		}

		// Raw string (backtick)
		if ch == '`' {
			start := i
			i++
			for i < n && runes[i] != '`' {
				i++
			}
			if i < n {
				i++
			}
			addTok(string(runes[start:i]), tokString)
			continue
		}

		// Single-quoted rune literal
		if ch == '\'' {
			start := i
			i++
			for i < n && runes[i] != '\'' {
				if runes[i] == '\\' && i+1 < n {
					i++
				}
				i++
			}
			if i < n {
				i++
			}
			addTok(string(runes[start:i]), tokString)
			continue
		}

		// Number
		if isDigit(ch) || (ch == '.' && i+1 < n && isDigit(runes[i+1])) {
			start := i
			if ch == '0' && i+1 < n && (runes[i+1] == 'x' || runes[i+1] == 'X') {
				i += 2
				for i < n && isHexDigit(runes[i]) {
					i++
				}
			} else if ch == '0' && i+1 < n && (runes[i+1] == 'b' || runes[i+1] == 'B') {
				i += 2
				for i < n && (runes[i] == '0' || runes[i] == '1') {
					i++
				}
			} else if ch == '0' && i+1 < n && (runes[i+1] == 'o' || runes[i+1] == 'O') {
				i += 2
				for i < n && runes[i] >= '0' && runes[i] <= '7' {
					i++
				}
			} else {
				for i < n && (isDigit(runes[i]) || runes[i] == '.') {
					i++
				}
				// Exponent
				if i < n && (runes[i] == 'e' || runes[i] == 'E') {
					i++
					if i < n && (runes[i] == '+' || runes[i] == '-') {
						i++
					}
					for i < n && isDigit(runes[i]) {
						i++
					}
				}
			}
			// Type suffix (i for complex)
			if i < n && runes[i] == 'i' {
				i++
			}
			addTok(string(runes[start:i]), tokNumber)
			continue
		}

		// Identifier or keyword
		if isIdentStart(ch) {
			start := i
			for i < n && isIdentPart(runes[i]) {
				i++
			}
			word := string(runes[start:i])

			// Check if followed by '(' => function call
			j := i
			for j < n && runes[j] == ' ' {
				j++
			}
			isFunc := j < n && runes[j] == '('

			if goKeywords[word] {
				addTok(word, tokKeyword)
			} else if goTypes[word] {
				addTok(word, tokType)
			} else if isFunc {
				addTok(word, tokFunction)
			} else {
				addTok(word, tokNormal)
			}
			continue
		}

		// Whitespace and operators -- emit one character at a time.
		addTok(string(ch), tokNormal)
		i++
	}

	return tokens, inBlock
}

// lineEndsInBlockComment determines whether we are inside a block comment
// after processing this line, starting from the given state.
func lineEndsInBlockComment(line string, inBlock bool) bool {
	_, newState := tokenizeLine(line, inBlock)
	return newState
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentPart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// ----- Input Handling -----

func (this *CodeEditor) posFromXY(x, y float64) (int, int) {
	fe := this.font.FontExtents()
	lh := fe.Height + 2
	topOff := this.topOffset()

	// The vertical hit lands on a visual row; folding maps that row back to the
	// underlying line index. With no active fold vis[row] == row.
	row := int((y - topOff + this.scrollY) / lh)
	if row < 0 {
		row = 0
	}
	vis := this.visibleLineIndices()
	var line int
	if len(vis) == 0 {
		line = 0
	} else if row >= len(vis) {
		line = vis[len(vis)-1]
	} else {
		line = vis[row]
	}
	if line >= len(this.lines) {
		line = len(this.lines) - 1
	}

	xOff := x - this.gutterW - 10 + this.scrollX
	col := 0
	if xOff >= 0 {
		runes := []rune(this.lines[line])
		col = len(runes)
		for c := 0; c <= len(runes); c++ {
			cw := this.measureText(string(runes[:c]))
			if cw > xOff {
				col = c
				if c > 0 {
					prevW := this.measureText(string(runes[:c-1]))
					if xOff-prevW < cw-xOff {
						col = c - 1
					}
				}
				break
			}
		}
	}
	return line, col
}

func (this *CodeEditor) OnLeftDown(x, y float64) {
	this.SetFocus()

	// If clicking in status bar area, ignore
	_, h := this.Size()
	if y > h-this.statusBarHeight {
		return
	}

	// If clicking in minimap area, scroll to that position
	w, _ := this.Size()
	mmW := this.minimapWidth()
	if this.showMinimap && mmW > 0 && x > w-mmW {
		topOff := this.topOffset()
		editorBottom := h - this.statusBarHeight
		mmH := editorBottom - topOff
		totalLines := len(this.lines)
		if totalLines > 0 && mmH > 0 {
			relY := y - topOff
			if relY < 0 {
				relY = 0
			}
			clickedLine := int(relY / mmH * float64(totalLines))
			if clickedLine >= totalLines {
				clickedLine = totalLines - 1
			}
			if clickedLine < 0 {
				clickedLine = 0
			}
			this.ScrollToLine(clickedLine)
		}
		return
	}

	// If clicking in find bar area, ignore normal click
	if this.findActive && y < this.findBarHeight {
		return
	}

	// If clicking in goto-line bar area, ignore normal click
	gotoBarTop := 0.0
	if this.findActive {
		gotoBarTop = this.findBarHeight
	}
	if this.gotoLineActive && y >= gotoBarTop && y < gotoBarTop+this.findBarHeight {
		return
	}

	// Gutter clicks (left of the text area, below the breadcrumb): the right strip
	// of the gutter carries the fold ▸/▾ marker, so a click there on a foldable
	// start line toggles the fold; anywhere else in the gutter toggles the
	// breakpoint. Normal text-area clicks fall through to the logic below.
	if x < this.gutterW && y >= this.topOffset() {
		line, _ := this.posFromXY(x, y)
		if line >= 0 && line < len(this.lines) {
			// Fold-marker hit zone: the ~12px strip at the gutter's right edge.
			if x >= this.gutterW-12 {
				if _, foldable := this.foldRegionAt(line); foldable {
					this.ToggleFold(line)
					return
				}
			}
			this.ToggleBreakpoint(line)
		}
		return
	}

	// Cmd+Click (macOS) / Ctrl+Click: go-to-definition with cross-file navigation
	if isActionModifier() && !IsKeyDown(KeyShift) {
		line, col := this.posFromXY(x, y)
		this.cursorLine = line
		this.cursorCol = col
		this.clampCursor()

		// Use hover link word if available, otherwise detect from cursor
		word := this.hoverLinkWord
		if word == "" {
			word = this.wordAtCursor()
		}

		if word != "" {
			// Push current position for back navigation
			this.pushNavPosition()

			// Try cross-file navigation first (searches current file + siblings)
			target := FindDefinition(word, this.filePath, this.Text())
			if target != nil {
				if target.FilePath == this.filePath || this.filePath == "" {
					// Same file: jump directly
					this.goToLine(target.Line)
				} else if this.cbNavigate != nil {
					// Different file: delegate to editor tabs
					this.cbNavigate(target.FilePath, target.Line)
				}
				// Clear hover link
				this.hoverLinkLine = -1
				this.hoverLinkWord = ""
				return
			}

			// Fallback: search in-file symbols only
			symbols := this.ParseSymbols()
			for _, s := range symbols {
				if s.Name == word {
					this.goToLine(s.Line)
					this.hoverLinkLine = -1
					this.hoverLinkWord = ""
					return
				}
			}
		}
		// Fall through to normal click if no definition found
	}

	line, col := this.posFromXY(x, y)

	now := time.Now()
	isShift := IsKeyDown(KeyShift)

	// Double-click detection
	if !isShift && now.Sub(this.lastClickTime) < 400*time.Millisecond &&
		line == this.lastClickLine && col == this.lastClickCol {
		// Double click: select word
		this.cursorLine = line
		this.cursorCol = col
		start, end := this.wordBoundsAt(line, col)
		this.selStartLine = line
		this.selStartCol = start
		this.selEndLine = line
		this.selEndCol = end
		this.hasSelection = start != end
		this.cursorCol = end
		this.lastClickTime = time.Time{} // prevent triple-click
		this.mouseDown = false
		this.Self().Update()
		return
	}

	this.lastClickTime = now
	this.lastClickLine = line
	this.lastClickCol = col

	if isShift {
		// Extend selection from current cursor
		if !this.hasSelection {
			this.selStartLine = this.cursorLine
			this.selStartCol = this.cursorCol
		}
		this.cursorLine = line
		this.cursorCol = col
		this.selEndLine = line
		this.selEndCol = col
		this.hasSelection = !(this.selStartLine == this.selEndLine && this.selStartCol == this.selEndCol)
	} else {
		this.clearSelection()
		// A plain click collapses multi-cursor back to a single caret.
		this.additionalCursors = nil
		this.cursorLine = line
		this.cursorCol = col
		this.setSelectionStart() // anchor for drag
	}

	this.mouseDown = true
	this.clampCursor()
	this.Self().Update()

	// Check for widget-name click (lines like "// widgetName ...")
	if this.cbWidgetClicked != nil && !isShift {
		trimmed := strings.TrimSpace(this.lines[this.cursorLine])
		if strings.HasPrefix(trimmed, "// ") {
			parts := strings.Fields(trimmed[3:])
			if len(parts) > 0 {
				this.cbWidgetClicked(parts[0])
			}
		}
	}
}

func (this *CodeEditor) OnMouseMove(x, y float64) {
	if this.mouseDown {
		line, col := this.posFromXY(x, y)
		this.cursorLine = line
		this.cursorCol = col
		this.clampCursor()
		this.updateSelectionEnd()
		this.ensureCursorVisible()
		this.Self().Update()
		return
	}

	// Hover link detection: Cmd+hover (macOS) / Ctrl+hover (other) shows underlined link
	if isActionModifier() {
		line, col := this.posFromXY(x, y)
		if line >= 0 && line < len(this.lines) {
			start, end := this.wordBoundsAt(line, col)
			if start != end {
				runes := []rune(this.lines[line])
				word := string(runes[start:end])
				if word != this.hoverLinkWord || line != this.hoverLinkLine {
					this.hoverLinkLine = line
					this.hoverLinkStart = start
					this.hoverLinkEnd = end
					this.hoverLinkWord = word
					this.Self().Update()
				}
				// Skip error tooltip while showing link hover
				return
			}
		}
	}

	// Clear link hover if modifier not held or not over a word
	if this.hoverLinkLine >= 0 {
		this.hoverLinkLine = -1
		this.hoverLinkStart = 0
		this.hoverLinkEnd = 0
		this.hoverLinkWord = ""
		this.Self().Update()
	}

	// Error tooltip on hover: show error message when hovering over error lines
	if len(this.errorLines) > 0 {
		line, _ := this.posFromXY(x, y)
		if msg, ok := this.errorLines[line]; ok {
			if this.hoverErrorLine != line {
				this.hoverErrorLine = line
				gx, gy := this.MapToGlobal(x, y)
				ShowToolTip(gx, gy, msg)
			}
		} else {
			if this.hoverErrorLine >= 0 {
				this.hoverErrorLine = -1
				HideToolTip()
			}
		}
	} else if this.hoverErrorLine >= 0 {
		this.hoverErrorLine = -1
		HideToolTip()
	}
}

func (this *CodeEditor) OnLeftUp(x, y float64) {
	this.mouseDown = false
}

// OnMouseLeave clears hover link state when the cursor leaves the editor.
func (this *CodeEditor) OnMouseLeave() {
	if this.hoverLinkLine >= 0 {
		this.hoverLinkLine = -1
		this.hoverLinkWord = ""
		this.Self().Update()
	}
	if this.hoverErrorLine >= 0 {
		this.hoverErrorLine = -1
		HideToolTip()
	}
}

func (this *CodeEditor) OnTextInput(s string) {
	// Route to rename input if active
	if this.renameActive {
		runes := []rune(this.renameText)
		insertRunes := []rune(s)
		newRunes := make([]rune, 0, len(runes)+len(insertRunes))
		newRunes = append(newRunes, runes[:this.renameCursorPos]...)
		newRunes = append(newRunes, insertRunes...)
		newRunes = append(newRunes, runes[this.renameCursorPos:]...)
		this.renameText = string(newRunes)
		this.renameCursorPos += len(insertRunes)
		this.Self().Update()
		return
	}

	// Route to symbol popup if active
	if this.symbolPopup != nil && this.symbolPopup.visible {
		this.symbolPopup.OnTextInput(s)
		this.Self().Update()
		return
	}

	// Route to goto-line bar if active
	if this.gotoLineActive {
		// Only accept digits
		for _, r := range s {
			if r >= '0' && r <= '9' {
				this.gotoLineText += string(r)
				this.gotoLineCursor++
			}
		}
		this.Self().Update()
		return
	}

	// Route to find bar if active
	if this.findActive {
		findRunes := []rune(this.findText)
		insertRunes := []rune(s)
		newRunes := make([]rune, 0, len(findRunes)+len(insertRunes))
		newRunes = append(newRunes, findRunes[:this.findCursor]...)
		newRunes = append(newRunes, insertRunes...)
		newRunes = append(newRunes, findRunes[this.findCursor:]...)
		this.findText = string(newRunes)
		this.findCursor += len(insertRunes)
		this.findUpdateMatches()
		this.findCurrentIdx = 0
		// Jump to first match
		if len(this.findMatches) > 0 {
			m := this.findMatches[0]
			this.cursorLine = m.line
			this.cursorCol = m.col
			this.ensureCursorVisible()
		}
		this.Self().Update()
		return
	}

	this.clampCursor()

	// Multi-cursor: if additional cursors are active and the input is
	// single-line, insert at every caret. Undo is recorded as a single
	// insert at the primary cursor — a pragmatic simplification that keeps
	// the undo stack simple; users can always Escape to exit multi-cursor.
	if len(this.additionalCursors) > 0 && !strings.Contains(s, "\n") {
		if this.hasSelection {
			this.clearSelection()
		}
		this.pushUndo(editAction{kind: 0, line: this.cursorLine, col: this.cursorCol, text: s})
		this.insertAtAllCursors(s)
		this.rebuildText()
		this.ensureCursorVisible()
		this.Self().Update()
		return
	}

	// Delete selection first if present
	if this.hasSelection {
		selText := this.SelectedText()
		sl, sc, _, _ := this.selectionRange()
		this.DeleteSelection()
		this.pushUndo(editAction{kind: 2, line: sl, col: sc, text: s, oldText: selText})
		this.insertTextAtCursor(s)
		this.rebuildText()
		this.ensureCursorVisible()
		this.Self().Update()
		return
	}

	this.pushUndo(editAction{kind: 0, line: this.cursorLine, col: this.cursorCol, text: s})

	line := this.lines[this.cursorLine]
	runes := []rune(line)
	insertRunes := []rune(s)

	newRunes := make([]rune, 0, len(runes)+len(insertRunes))
	newRunes = append(newRunes, runes[:this.cursorCol]...)
	newRunes = append(newRunes, insertRunes...)
	newRunes = append(newRunes, runes[this.cursorCol:]...)
	this.lines[this.cursorLine] = string(newRunes)
	this.cursorCol += len(insertRunes)

	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()

	// Trigger completion after typing
	this.tryTriggerCompletion(s)
}

// tryTriggerCompletion checks whether to show the completion popup after input.
func (this *CodeEditor) tryTriggerCompletion(typed string) {
	if this.completion == nil {
		this.completion = NewCompletionPopup(this)
	}
	if len(typed) == 0 {
		return
	}
	lastRune := []rune(typed)[len([]rune(typed))-1]
	if lastRune == '.' {
		// Trigger after dot
		this.completion.Show("", this)
		return
	}
	if unicode.IsLetter(lastRune) || lastRune == '_' {
		// Build prefix from current word
		prefix := this.currentWordPrefix()
		if len(prefix) >= 1 {
			this.completion.Show(prefix, this)
		}
	} else {
		this.completion.Dismiss()
	}
}

// currentWordPrefix returns the word being typed at the cursor position.
func (this *CodeEditor) currentWordPrefix() string {
	if this.cursorLine >= len(this.lines) {
		return ""
	}
	runes := []rune(this.lines[this.cursorLine])
	end := this.cursorCol
	if end > len(runes) {
		end = len(runes)
	}
	start := end
	for start > 0 && (unicode.IsLetter(runes[start-1]) || unicode.IsDigit(runes[start-1]) || runes[start-1] == '_') {
		start--
	}
	if start == end {
		return ""
	}
	return string(runes[start:end])
}

func (this *CodeEditor) OnKeyDown(key int, repeat bool) {
	// --- Rename refactoring key routing ---
	if this.renameActive {
		switch key {
		case KeyEnter:
			this.renameAccept()
			return
		case KeyEsc:
			this.renameCancel()
			return
		case KeyBackSpace:
			if this.renameCursorPos > 0 {
				runes := []rune(this.renameText)
				runes = append(runes[:this.renameCursorPos-1], runes[this.renameCursorPos:]...)
				this.renameText = string(runes)
				this.renameCursorPos--
				this.Self().Update()
			}
			return
		case KeyLeft:
			if this.renameCursorPos > 0 {
				this.renameCursorPos--
				this.Self().Update()
			}
			return
		case KeyRight:
			if this.renameCursorPos < len([]rune(this.renameText)) {
				this.renameCursorPos++
				this.Self().Update()
			}
			return
		case KeyHome:
			this.renameCursorPos = 0
			this.Self().Update()
			return
		case KeyEnd:
			this.renameCursorPos = len([]rune(this.renameText))
			this.Self().Update()
			return
		}
		return // consume all other keys while rename is active
	}

	// --- Symbol popup key routing ---
	if this.symbolPopup != nil && this.symbolPopup.visible {
		switch key {
		case KeyUp:
			this.symbolPopup.SelectPrev()
			this.Self().Update()
			return
		case KeyDown:
			this.symbolPopup.SelectNext()
			this.Self().Update()
			return
		case KeyEnter:
			this.symbolPopup.Accept(this)
			this.Self().Update()
			return
		case KeyEsc:
			this.symbolPopup.Dismiss()
			this.Self().Update()
			return
		case KeyBackSpace:
			this.symbolPopup.OnBackspace()
			this.Self().Update()
			return
		}
		return // consume all other keys while symbol popup is open
	}

	// --- Go-to-line bar key routing ---
	if this.gotoLineActive {
		switch key {
		case KeyEsc:
			this.gotoLineActive = false
			this.gotoLineText = ""
			this.gotoLineCursor = 0
			this.Self().Update()
			return
		case KeyEnter:
			this.gotoLineAccept()
			this.Self().Update()
			return
		case KeyBackSpace:
			if len(this.gotoLineText) > 0 {
				runes := []rune(this.gotoLineText)
				this.gotoLineText = string(runes[:len(runes)-1])
				this.gotoLineCursor--
			}
			this.Self().Update()
			return
		}
		return // consume all other keys while goto-line is active
	}

	// --- Completion popup key routing ---
	if this.completion != nil && this.completion.visible {
		switch key {
		case KeyUp:
			this.completion.SelectPrev()
			this.Self().Update()
			return
		case KeyDown:
			this.completion.SelectNext()
			this.Self().Update()
			return
		case KeyEnter, KeyTab:
			this.completion.Accept(this)
			this.Self().Update()
			return
		case KeyEsc:
			this.completion.Dismiss()
			this.Self().Update()
			return
		}
	}
	// --- Find bar key routing ---
	if this.findActive {
		switch key {
		case KeyEsc:
			this.findActive = false
			this.findMatches = nil
			this.Self().Update()
			return
		case KeyEnter, KeyF3:
			if IsKeyDown(KeyShift) {
				this.findPrev()
			} else {
				this.findNext()
			}
			return
		case KeyBackSpace:
			if this.findCursor > 0 {
				fr := []rune(this.findText)
				fr = append(fr[:this.findCursor-1], fr[this.findCursor:]...)
				this.findText = string(fr)
				this.findCursor--
				this.findUpdateMatches()
				this.Self().Update()
			}
			return
		case KeyLeft:
			if this.findCursor > 0 {
				this.findCursor--
				this.Self().Update()
			}
			return
		case KeyRight:
			if this.findCursor < len([]rune(this.findText)) {
				this.findCursor++
				this.Self().Update()
			}
			return
		case KeyHome:
			this.findCursor = 0
			this.Self().Update()
			return
		case KeyEnd:
			this.findCursor = len([]rune(this.findText))
			this.Self().Update()
			return
		}
		// Let Ctrl+F pass through to toggle find off below
		if !(key == 'F' && IsKeyDown(KeyCtrl)) {
			return
		}
	}

	shift := IsKeyDown(KeyShift)
	ctrl := IsKeyDown(KeyCtrl)
	alt := IsKeyDown(KeyMenu)

	// Helper: begin or extend selection on shift-modified movement
	beginSelIfShift := func() {
		if shift && !this.hasSelection {
			this.setSelectionStart()
		}
	}
	endSelIfShift := func() {
		if shift {
			this.updateSelectionEnd()
		} else {
			this.clearSelection()
		}
	}

	// Ctrl+Space triggers completion
	if ctrl && key == KeySpace {
		if this.completion == nil {
			this.completion = NewCompletionPopup(this)
		}
		prefix := this.currentWordPrefix()
		this.completion.Show(prefix, this)
		this.Self().Update()
		return
	}

	// Dismiss completion on other keys
	if this.completion != nil && this.completion.visible {
		switch key {
		case KeyLeft, KeyRight, KeyHome, KeyEnd:
			this.completion.Dismiss()
		case KeyBackSpace:
			// Will handle normally, then re-trigger
		}
	}

	switch key {
	case KeyBackSpace:
		// Multi-cursor: delete at every caret in reverse order.
		if len(this.additionalCursors) > 0 && !this.hasSelection {
			this.backspaceAtAllCursors()
			this.rebuildText()
			this.ensureCursorVisible()
			this.Self().Update()
			return
		}
		if this.hasSelection {
			selText := this.SelectedText()
			sl, sc, _, _ := this.selectionRange()
			this.pushUndo(editAction{kind: 1, line: sl, col: sc, text: selText})
			this.DeleteSelection()
			this.ensureCursorVisible()
			this.Self().Update()
			return
		}
		this.clampCursor()
		if this.cursorCol > 0 {
			line := this.lines[this.cursorLine]
			runes := []rune(line)
			deleted := string(runes[this.cursorCol-1 : this.cursorCol])
			this.pushUndo(editAction{kind: 1, line: this.cursorLine, col: this.cursorCol - 1, text: deleted})
			newRunes := append(runes[:this.cursorCol-1], runes[this.cursorCol:]...)
			this.lines[this.cursorLine] = string(newRunes)
			this.cursorCol--
			this.rebuildText()
		} else if this.cursorLine > 0 {
			prevLine := this.lines[this.cursorLine-1]
			curLine := this.lines[this.cursorLine]
			newCol := len([]rune(prevLine))
			this.pushUndo(editAction{kind: 1, line: this.cursorLine - 1, col: newCol, text: "\n"})
			this.lines[this.cursorLine-1] = prevLine + curLine
			this.lines = append(this.lines[:this.cursorLine], this.lines[this.cursorLine+1:]...)
			this.cursorLine--
			this.cursorCol = newCol
			this.rebuildText()
		}
		this.ensureCursorVisible()
		this.Self().Update()
		// Update completion after backspace
		if this.completion != nil && this.completion.visible {
			prefix := this.currentWordPrefix()
			if prefix == "" {
				this.completion.Dismiss()
			} else {
				this.completion.Show(prefix, this)
			}
		}

	case KeyDelete:
		// Multi-cursor: forward-delete at every caret.
		if len(this.additionalCursors) > 0 && !this.hasSelection {
			this.deleteAtAllCursors()
			this.rebuildText()
			this.ensureCursorVisible()
			this.Self().Update()
			return
		}
		if this.hasSelection {
			selText := this.SelectedText()
			sl, sc, _, _ := this.selectionRange()
			this.pushUndo(editAction{kind: 1, line: sl, col: sc, text: selText})
			this.DeleteSelection()
			this.Self().Update()
			return
		}
		this.clampCursor()
		line := this.lines[this.cursorLine]
		runes := []rune(line)
		if this.cursorCol < len(runes) {
			deleted := string(runes[this.cursorCol : this.cursorCol+1])
			this.pushUndo(editAction{kind: 1, line: this.cursorLine, col: this.cursorCol, text: deleted})
			newRunes := append(runes[:this.cursorCol], runes[this.cursorCol+1:]...)
			this.lines[this.cursorLine] = string(newRunes)
			this.rebuildText()
		} else if this.cursorLine < len(this.lines)-1 {
			this.pushUndo(editAction{kind: 1, line: this.cursorLine, col: this.cursorCol, text: "\n"})
			nextLine := this.lines[this.cursorLine+1]
			this.lines[this.cursorLine] = line + nextLine
			this.lines = append(this.lines[:this.cursorLine+1], this.lines[this.cursorLine+2:]...)
			this.rebuildText()
		}
		this.Self().Update()

	case KeyEnter:
		this.clampCursor()
		line := this.lines[this.cursorLine]
		runes := []rune(line)
		before := string(runes[:this.cursorCol])
		after := string(runes[this.cursorCol:])

		// Auto-indent: copy leading whitespace from current line.
		indent := ""
		for _, r := range []rune(line) {
			if r == ' ' || r == '\t' {
				indent += string(r)
			} else {
				break
			}
		}

		// Smart indent: extra tab after '{'
		trimmedBefore := strings.TrimRight(before, " \t")
		if len(trimmedBefore) > 0 && trimmedBefore[len(trimmedBefore)-1] == '{' {
			indent += "\t"
		}
		// Smart indent: reduce indent after '}'
		trimmedAfter := strings.TrimLeft(after, " \t")
		if len(trimmedAfter) > 0 && trimmedAfter[0] == '}' && len(indent) > 0 {
			// Remove one level
			if indent[len(indent)-1] == '\t' {
				indent = indent[:len(indent)-1]
			} else {
				// Remove up to 4 spaces
				spaces := 0
				for spaces < 4 && len(indent) > 0 && indent[len(indent)-1] == ' ' {
					indent = indent[:len(indent)-1]
					spaces++
				}
			}
		}

		insertedText := "\n" + indent
		if this.hasSelection {
			selText := this.SelectedText()
			sl, sc, _, _ := this.selectionRange()
			this.DeleteSelection()
			this.pushUndo(editAction{kind: 2, line: sl, col: sc, text: insertedText, oldText: selText})
			// Recompute after deletion
			this.clampCursor()
			line = this.lines[this.cursorLine]
			runes = []rune(line)
			before = string(runes[:this.cursorCol])
			after = string(runes[this.cursorCol:])
		} else {
			this.pushUndo(editAction{kind: 0, line: this.cursorLine, col: this.cursorCol, text: insertedText})
		}

		this.lines[this.cursorLine] = before
		newLines := make([]string, 0, len(this.lines)+1)
		newLines = append(newLines, this.lines[:this.cursorLine+1]...)
		newLines = append(newLines, indent+after)
		newLines = append(newLines, this.lines[this.cursorLine+1:]...)
		this.lines = newLines
		this.cursorLine++
		this.cursorCol = len([]rune(indent))
		this.clearSelection()
		this.rebuildText()
		this.ensureCursorVisible()
		this.Self().Update()

	case KeyTab:
		// Try snippet expansion before inserting tab
		if !shift && !ctrl && !this.hasSelection {
			if this.tryExpandSnippet() {
				return
			}
		}
		this.OnTextInput("\t")

	case KeyLeft:
		if alt && !ctrl && !shift {
			// Alt+Left: navigate back
			this.NavGoBack()
			return
		}
		beginSelIfShift()
		this.clampCursor()
		if this.cursorCol > 0 {
			this.cursorCol--
		} else if this.cursorLine > 0 {
			this.cursorLine--
			this.cursorCol = len([]rune(this.lines[this.cursorLine]))
		}
		endSelIfShift()
		this.ensureCursorVisible()
		this.Self().Update()

	case KeyRight:
		if alt && !ctrl && !shift {
			// Alt+Right: navigate forward
			this.NavGoForward()
			return
		}
		beginSelIfShift()
		this.clampCursor()
		lineLen := len([]rune(this.lines[this.cursorLine]))
		if this.cursorCol < lineLen {
			this.cursorCol++
		} else if this.cursorLine < len(this.lines)-1 {
			this.cursorLine++
			this.cursorCol = 0
		}
		endSelIfShift()
		this.ensureCursorVisible()
		this.Self().Update()

	case KeyUp:
		// Multi-cursor: Cmd/Ctrl+Alt+Up adds a cursor on the line above.
		if alt && (ctrl || isActionModifier()) {
			// Use the topmost existing cursor as the anchor.
			top := cursorPos{line: this.cursorLine, col: this.cursorCol}
			for _, c := range this.additionalCursors {
				if c.line < top.line {
					top = c
				}
			}
			if top.line > 0 {
				this.AddCursorAtLine(top.line-1, top.col)
				this.ensureCursorVisible()
				this.Self().Update()
			}
			return
		}
		if alt {
			this.moveLineUp()
			return
		}
		beginSelIfShift()
		if this.cursorLine > 0 {
			this.cursorLine--
			this.clampCursor()
		}
		endSelIfShift()
		this.ensureCursorVisible()
		this.Self().Update()

	case KeyDown:
		// Multi-cursor: Cmd/Ctrl+Alt+Down adds a cursor on the line below.
		if alt && (ctrl || isActionModifier()) {
			bot := cursorPos{line: this.cursorLine, col: this.cursorCol}
			for _, c := range this.additionalCursors {
				if c.line > bot.line {
					bot = c
				}
			}
			if bot.line < len(this.lines)-1 {
				this.AddCursorAtLine(bot.line+1, bot.col)
				this.ensureCursorVisible()
				this.Self().Update()
			}
			return
		}
		if alt {
			this.moveLineDown()
			return
		}
		beginSelIfShift()
		if this.cursorLine < len(this.lines)-1 {
			this.cursorLine++
			this.clampCursor()
		}
		endSelIfShift()
		this.ensureCursorVisible()
		this.Self().Update()

	case KeyHome:
		beginSelIfShift()
		this.cursorCol = 0
		endSelIfShift()
		this.Self().Update()

	case KeyEnd:
		beginSelIfShift()
		this.clampCursor()
		this.cursorCol = len([]rune(this.lines[this.cursorLine]))
		endSelIfShift()
		this.Self().Update()

	case KeyPageUp:
		beginSelIfShift()
		fe := this.font.FontExtents()
		lh := fe.Height + 2
		_, h := this.Size()
		pageLines := int((h - this.topOffset() - this.statusBarHeight) / lh)
		if pageLines < 1 {
			pageLines = 1
		}
		this.cursorLine -= pageLines
		if this.cursorLine < 0 {
			this.cursorLine = 0
		}
		this.clampCursor()
		this.scrollY -= float64(pageLines) * lh
		if this.scrollY < 0 {
			this.scrollY = 0
		}
		endSelIfShift()
		this.Self().Update()

	case KeyPageDown:
		beginSelIfShift()
		fe := this.font.FontExtents()
		lh := fe.Height + 2
		_, h := this.Size()
		pageLines := int((h - this.topOffset() - this.statusBarHeight) / lh)
		if pageLines < 1 {
			pageLines = 1
		}
		this.cursorLine += pageLines
		if this.cursorLine >= len(this.lines) {
			this.cursorLine = len(this.lines) - 1
		}
		this.clampCursor()
		this.scrollY += float64(pageLines) * lh
		maxScroll := float64(len(this.visibleLineIndices()))*lh - h + this.topOffset() + this.statusBarHeight
		if maxScroll < 0 {
			maxScroll = 0
		}
		if this.scrollY > maxScroll {
			this.scrollY = maxScroll
		}
		endSelIfShift()
		this.Self().Update()

	case KeyEsc:
		// Multi-cursor: Escape first collapses extra cursors, then the selection.
		if len(this.additionalCursors) > 0 {
			this.ClearAdditionalCursors()
			return
		}
		this.clearSelection()
		this.Self().Update()

	case 'A':
		if ctrl {
			this.editorSelectAll()
		}

	case 'C':
		if ctrl {
			this.editorCopy()
		}

	case 'V':
		if ctrl {
			this.pasteClipboard()
		}

	case 'X':
		if ctrl {
			this.editorCut()
		}

	case 'Z':
		if ctrl {
			if shift {
				this.redo()
			} else {
				this.undo()
			}
		}

	case 'Y':
		if ctrl {
			this.redo()
		}

	case 'F':
		if ctrl && shift {
			// Ctrl+Shift+F: format code with gofmt
			this.FormatCode()
		} else if ctrl {
			this.findActive = !this.findActive
			if this.findActive {
				// If text is selected, use it as find text
				if this.hasSelection {
					this.findText = this.SelectedText()
					this.findCursor = len([]rune(this.findText))
					this.findUpdateMatches()
				}
			} else {
				this.findMatches = nil
			}
			this.Self().Update()
		}

	case 'D':
		if ctrl {
			// Multi-cursor Ctrl+D (VS Code style): select current word and
			// add a cursor at the next occurrence. When invoked on an empty
			// or whitespace-only line with no existing selection or
			// secondary cursors, fall back to the classic duplicate-line
			// behavior so the shortcut still does something useful.
			if this.hasSelection || len(this.additionalCursors) > 0 {
				this.selectNextOccurrence()
			} else {
				this.clampCursor()
				word := this.wordAtCursor()
				if word != "" {
					this.selectNextOccurrence()
				} else {
					this.duplicateLine()
				}
			}
		}

	case 'K':
		if ctrl && shift {
			this.deleteCurrentLine()
		}

	case 'O':
		if ctrl && shift {
			// Ctrl+Shift+O: open symbol popup
			if this.symbolPopup == nil {
				this.symbolPopup = NewSymbolPopup()
			}
			this.symbolPopup.Show(this)
			this.Self().Update()
		}

	case 'G':
		if ctrl {
			// Ctrl+G: go to line
			this.gotoLineActive = true
			this.gotoLineText = ""
			this.gotoLineCursor = 0
			this.Self().Update()
		}

	case 'W':
		if ctrl && shift {
			// Ctrl+Shift+W: toggle word wrap
			this.wordWrap = !this.wordWrap
			this.Self().Update()
		}

	case 'B':
		if ctrl {
			this.ToggleBookmark()
		}

	case KeyF2:
		if shift {
			this.PrevBookmark()
		} else if ctrl {
			// Ctrl+F2: rename refactoring (Qt Creator style)
			this.activateRename()
		} else {
			this.NextBookmark()
		}

	case KeyF9:
		// F9: toggle a breakpoint on the cursor line (Qt Creator / VS Code style).
		if !ctrl && !shift {
			this.clampCursor()
			this.ToggleBreakpoint(this.cursorLine)
		}

	case KeyF12:
		// F12 / Shift+F12: AST-driven navigation for the identifier at the cursor.
		// F12 jumps to its definition; Shift+F12 highlights every reference in the
		// current file by reusing the find-bar's findMatches overlay.
		if shift {
			this.HighlightReferencesAtCursor()
		} else {
			this.GoToDefinitionAtCursor()
		}

	case '/':
		if ctrl || isActionModifier() {
			// Cmd+/ (macOS) / Ctrl+/: toggle line comment
			this.ToggleLineComment()
		}

	case 'R':
		if ctrl && shift {
			// Ctrl+Shift+R: rename refactoring
			this.activateRename()
		}

	case '[':
		if (ctrl || isActionModifier()) && alt {
			// Cmd+Option+[ (macOS) / Ctrl+Alt+[: fold the region at the cursor.
			if reg, ok := this.foldRegionEnclosing(this.cursorLine); ok && !this.IsFolded(reg.startLine) {
				this.ToggleFold(reg.startLine)
			}
		} else if (ctrl || isActionModifier()) && shift {
			// Cmd+Shift+[ (macOS) / Ctrl+Shift+[: fold every region.
			this.FoldAll()
		} else if ctrl || isActionModifier() {
			// Cmd+[ (macOS) / Ctrl+[: navigate back
			this.NavGoBack()
		}

	case ']':
		if (ctrl || isActionModifier()) && alt {
			// Cmd+Option+] (macOS) / Ctrl+Alt+]: unfold the region at the cursor.
			if reg, ok := this.foldRegionEnclosing(this.cursorLine); ok && this.IsFolded(reg.startLine) {
				this.ToggleFold(reg.startLine)
			}
		} else if (ctrl || isActionModifier()) && shift {
			// Cmd+Shift+] (macOS) / Ctrl+Shift+]: unfold every region.
			this.UnfoldAll()
		} else if ctrl || isActionModifier() {
			// Cmd+] (macOS) / Ctrl+]: navigate forward
			this.NavGoForward()
		}

	}
}

// editorSelectAll selects all text in the editor.
func (this *CodeEditor) editorSelectAll() {
	this.selStartLine = 0
	this.selStartCol = 0
	this.selEndLine = len(this.lines) - 1
	this.selEndCol = len([]rune(this.lines[len(this.lines)-1]))
	this.hasSelection = true
	this.cursorLine = this.selEndLine
	this.cursorCol = this.selEndCol
	this.Self().Update()
}

// editorCopy copies the selection (or current line) to clipboard.
func (this *CodeEditor) editorCopy() {
	this.clampCursor()
	var text string
	if this.hasSelection {
		text = this.SelectedText()
	} else {
		text = this.lines[this.cursorLine]
	}
	if text != "" {
		Clipboard.Clear()
		Clipboard.SetData(text)
	}
}

// editorCut copies and removes the selection (or current line).
func (this *CodeEditor) editorCut() {
	this.clampCursor()
	if this.hasSelection {
		text := this.SelectedText()
		if text != "" {
			Clipboard.Clear()
			Clipboard.SetData(text)
		}
		sl, sc, _, _ := this.selectionRange()
		this.pushUndo(editAction{kind: 1, line: sl, col: sc, text: text})
		this.DeleteSelection()
		this.ensureCursorVisible()
		this.Self().Update()
		return
	}
	text := this.lines[this.cursorLine]
	if text != "" {
		Clipboard.Clear()
		Clipboard.SetData(text)
	}
	if len(this.lines) > 1 {
		this.pushUndo(editAction{kind: 1, line: this.cursorLine, col: 0, text: text + "\n"})
		this.lines = append(this.lines[:this.cursorLine], this.lines[this.cursorLine+1:]...)
		if this.cursorLine >= len(this.lines) {
			this.cursorLine = len(this.lines) - 1
		}
	} else {
		this.pushUndo(editAction{kind: 1, line: 0, col: 0, text: text})
		this.lines[0] = ""
	}
	this.cursorCol = 0
	this.rebuildText()
	this.Self().Update()
}

// pasteClipboard inserts clipboard text at cursor, replacing selection.
func (this *CodeEditor) pasteClipboard() {
	data, err := Clipboard.Data("text/plain")
	if err != nil {
		return
	}
	text, ok := data.(string)
	if !ok || text == "" {
		return
	}

	// Delete selection first
	if this.hasSelection {
		selText := this.SelectedText()
		sl, sc, _, _ := this.selectionRange()
		this.DeleteSelection()
		this.pushUndo(editAction{kind: 2, line: sl, col: sc, text: text, oldText: selText})
	} else {
		this.pushUndo(editAction{kind: 0, line: this.cursorLine, col: this.cursorCol, text: text})
	}

	// Handle multi-line paste
	pasteLines := strings.Split(text, "\n")
	if len(pasteLines) == 1 {
		this.insertTextAtCursor(text)
		this.rebuildText()
		this.ensureCursorVisible()
		this.Self().Update()
		return
	}
	this.clampCursor()
	this.insertMultilineAtCursor(pasteLines)
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
}

func (this *CodeEditor) OnMouseWheel(x, y, z float64) {
	fe := this.font.FontExtents()
	lh := fe.Height + 2

	// Shift+Scroll = horizontal scrolling
	if IsKeyDown(KeyShift) {
		scrollStep := this.measureText("    ") // 4 chars
		this.scrollX -= z * 3 * scrollStep
		if this.scrollX < 0 {
			this.scrollX = 0
		}
		this.Self().Update()
		return
	}

	this.scrollY -= z * 3 * lh
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.lines))*lh - h + this.topOffset() + this.statusBarHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

func (this *CodeEditor) Cursor() *Cursor {
	return cursorIBeam
}

func (this *CodeEditor) SizeHints() SizeHints {
	fe := this.font.FontExtents()
	h := fe.Height*10 + 4
	return SizeHints{
		Width:  math.Max(this.w, 200),
		Height: h,
		Policy: GrowHorizontal | GrowVertical,
	}
}

func (this *CodeEditor) Layout() {
	this.Self().Update()
}

func (this *CodeEditor) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
}

// FormatCode runs gofmt on the current editor text with a timeout to prevent
// UI freezes. If formatting succeeds, the text is replaced with the formatted
// output. If it fails or times out, the text remains unchanged.
func (this *CodeEditor) FormatCode() {
	text := this.Text()
	if text == "" {
		return
	}

	// Write current text to a temp file
	tmpFile, err := os.CreateTemp("", "silk_fmt_*.go")
	if err != nil {
		core.Error(err)
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(text); err != nil {
		tmpFile.Close()
		core.Error(err)
		return
	}
	tmpFile.Close()

	// Run gofmt with a timeout so a hung process cannot freeze the UI
	cmd := exec.Command("gofmt", tmpPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		core.Error(fmt.Errorf("gofmt start: %v", err))
		return
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			core.Error(fmt.Errorf("gofmt: %v", err))
			return
		}
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
		core.Error(fmt.Errorf("gofmt: timed out after 3s"))
		return
	}

	// Replace text with formatted output
	formatted := out.String()
	if formatted != "" && formatted != text {
		// Save cursor position
		saveLine := this.cursorLine
		saveCol := this.cursorCol
		this.SetText(formatted)
		// Restore cursor approximately
		this.cursorLine = saveLine
		if this.cursorLine >= len(this.lines) {
			this.cursorLine = len(this.lines) - 1
		}
		this.cursorCol = saveCol
		this.clampCursor()
		this.ensureCursorVisible()
		this.Self().Update()
	}
}

// --- Error Markers ---

// SetErrors sets compile error markers on specific lines.
// The map keys are 0-based line numbers, values are error messages.
func (this *CodeEditor) SetErrors(errors map[int]string) {
	this.errorLines = errors
	this.Self().Update()
}

// ClearErrors removes all error markers.
func (this *CodeEditor) ClearErrors() {
	this.errorLines = nil
	this.Self().Update()
}

// ErrorAtLine returns the error message for a line, or empty string if none.
func (this *CodeEditor) ErrorAtLine(line int) string {
	if this.errorLines == nil {
		return ""
	}
	return this.errorLines[line]
}

// --- Bookmarks ---

// ToggleBookmark toggles a bookmark on the current cursor line.
func (this *CodeEditor) ToggleBookmark() {
	this.clampCursor()
	if this.bookmarks == nil {
		this.bookmarks = make(map[int]bool)
	}
	if this.bookmarks[this.cursorLine] {
		delete(this.bookmarks, this.cursorLine)
	} else {
		this.bookmarks[this.cursorLine] = true
	}
	this.Self().Update()
}

// NextBookmark moves the cursor to the next bookmark after the current line.
func (this *CodeEditor) NextBookmark() {
	if len(this.bookmarks) == 0 {
		return
	}
	// Collect and sort bookmark lines
	bms := make([]int, 0, len(this.bookmarks))
	for ln := range this.bookmarks {
		bms = append(bms, ln)
	}
	sort.Ints(bms)
	// Find next bookmark after current line
	for _, ln := range bms {
		if ln > this.cursorLine {
			this.cursorLine = ln
			this.cursorCol = 0
			this.clearSelection()
			this.ensureCursorVisible()
			this.Self().Update()
			return
		}
	}
	// Wrap around to first bookmark
	this.cursorLine = bms[0]
	this.cursorCol = 0
	this.clearSelection()
	this.ensureCursorVisible()
	this.Self().Update()
}

// PrevBookmark moves the cursor to the previous bookmark before the current line.
func (this *CodeEditor) PrevBookmark() {
	if len(this.bookmarks) == 0 {
		return
	}
	bms := make([]int, 0, len(this.bookmarks))
	for ln := range this.bookmarks {
		bms = append(bms, ln)
	}
	sort.Ints(bms)
	// Find previous bookmark before current line
	for i := len(bms) - 1; i >= 0; i-- {
		if bms[i] < this.cursorLine {
			this.cursorLine = bms[i]
			this.cursorCol = 0
			this.clearSelection()
			this.ensureCursorVisible()
			this.Self().Update()
			return
		}
	}
	// Wrap around to last bookmark
	this.cursorLine = bms[len(bms)-1]
	this.cursorCol = 0
	this.clearSelection()
	this.ensureCursorVisible()
	this.Self().Update()
}

// --- Breakpoints ---
//
// Breakpoints are part of the editor's UI/state layer only; toggling one renders
// a red dot in the gutter but does NOT start, stop, or talk to any debugger.
// Lines are keyed 0-based (same convention as cursorLine and bookmarks). The set
// is NOT re-mapped when lines are inserted or deleted (known limitation).

// ToggleBreakpoint flips the breakpoint state of a line.
func (this *CodeEditor) ToggleBreakpoint(line int) {
	if this.breakpoints == nil {
		this.breakpoints = make(map[int]bool)
	}
	if this.breakpoints[line] {
		delete(this.breakpoints, line)
	} else {
		this.breakpoints[line] = true
	}
	this.Self().Update()
}

// SetBreakpoint enables or disables the breakpoint on a line.
func (this *CodeEditor) SetBreakpoint(line int, on bool) {
	if this.breakpoints == nil {
		this.breakpoints = make(map[int]bool)
	}
	if on {
		this.breakpoints[line] = true
	} else {
		delete(this.breakpoints, line)
	}
	this.Self().Update()
}

// ClearBreakpoints removes all breakpoints.
func (this *CodeEditor) ClearBreakpoints() {
	this.breakpoints = make(map[int]bool)
	this.Self().Update()
}

// Breakpoints returns the lines (0-based) that currently have a breakpoint,
// sorted ascending.
func (this *CodeEditor) Breakpoints() []int {
	lines := make([]int, 0, len(this.breakpoints))
	for ln := range this.breakpoints {
		lines = append(lines, ln)
	}
	sort.Ints(lines)
	return lines
}

// --- Code Folding ---
//
// Folding collapses a brace block so its body lines are hidden from the view.
// A foldRegion is the start line that ends in '{' and the line carrying the
// matching '}'. The user collapses a region by clicking the ▸/▾ marker in the
// gutter (or via the public API); collapsed regions hide lines start+1 .. end.
//
// Folding heuristic: a straightforward brace-depth scan. A line whose last
// non-space rune is '{' opens a region; it closes on the line whose '}' brings
// the depth back to where the opener sat. This is a Qt Creator / VS Code style
// brace fold. Limitation: braces inside string literals, runes, or comments are
// counted like any other brace, so pathological lines (e.g. a '{' inside a
// string with no real block) can mis-pair. Regions spanning a single line
// (open and close on the same line) are not foldable.

// foldRegion is a foldable brace block: lines startLine .. endLine inclusive,
// where startLine ends in '{' and endLine carries the matching '}'.
type foldRegion struct {
	startLine int
	endLine   int
}

// lastNonSpaceRune returns the final non-whitespace rune of s and true, or
// (0, false) when s is blank.
func lastNonSpaceRune(s string) (rune, bool) {
	trimmed := strings.TrimRight(s, " \t\r")
	if trimmed == "" {
		return 0, false
	}
	r := []rune(trimmed)
	return r[len(r)-1], true
}

// computeFoldRegions scans lines and returns every foldable brace block using a
// brace-depth match. Only multi-line regions (endLine > startLine) are returned;
// an opener with no matching closer (unbalanced '{') is dropped. The result is
// ordered by startLine. See the heuristic note above for the string/comment
// limitation.
func computeFoldRegions(lines []string) []foldRegion {
	type opener struct {
		line  int
		depth int
	}
	var stack []opener
	var regions []foldRegion
	depth := 0
	for i, line := range lines {
		// Count brace deltas on this line so a "} else {" both closes and opens.
		for _, r := range line {
			switch r {
			case '{':
				depth++
			case '}':
				if depth > 0 {
					depth--
				}
				// Close the most recent opener sitting at this depth.
				if n := len(stack); n > 0 && stack[n-1].depth == depth {
					op := stack[n-1]
					stack = stack[:n-1]
					if i > op.line {
						regions = append(regions, foldRegion{startLine: op.line, endLine: i})
					}
				}
			}
		}
		// A line whose last visible rune is '{' opens a region at this line.
		if last, ok := lastNonSpaceRune(line); ok && last == '{' {
			stack = append(stack, opener{line: i, depth: depth - 1})
		}
	}
	sort.Slice(regions, func(a, b int) bool {
		return regions[a].startLine < regions[b].startLine
	})
	return regions
}

// visibleLines returns the ordered indices of lines that should be drawn given
// the set of folded start-lines and the foldable regions. A folded region hides
// its body (start+1 .. end); the start line itself stays visible. Nested folds
// compose: the outermost folded region wins, so a region whose start is hidden
// by an enclosing fold contributes nothing extra. Pure; no GL, no receiver.
func visibleLines(total int, folded map[int]bool, regions []foldRegion) []int {
	// hidden[i] marks a line concealed by some folded region.
	hidden := make([]bool, total)
	for _, reg := range regions {
		if reg.startLine < 0 || reg.startLine >= total {
			continue
		}
		if !folded[reg.startLine] {
			continue
		}
		end := reg.endLine
		if end >= total {
			end = total - 1
		}
		for i := reg.startLine + 1; i <= end; i++ {
			hidden[i] = true
		}
	}
	out := make([]int, 0, total)
	for i := 0; i < total; i++ {
		if !hidden[i] {
			out = append(out, i)
		}
	}
	return out
}

// foldRegionAt returns the region that opens on the given start line, or false.
func (this *CodeEditor) foldRegionAt(startLine int) (foldRegion, bool) {
	for _, reg := range this.FoldRegions() {
		if reg.startLine == startLine {
			return reg, true
		}
	}
	return foldRegion{}, false
}

// foldRegionEnclosing returns the innermost foldable region whose span contains
// the given line (start <= line <= end), or false. Used by the keyboard
// fold/unfold-at-cursor shortcuts.
func (this *CodeEditor) foldRegionEnclosing(line int) (foldRegion, bool) {
	var best foldRegion
	found := false
	for _, reg := range this.FoldRegions() {
		if line >= reg.startLine && line <= reg.endLine {
			// Prefer the tightest (largest start) enclosing region.
			if !found || reg.startLine > best.startLine {
				best = reg
				found = true
			}
		}
	}
	return best, found
}

// FoldRegions returns the foldable brace regions for the current text, ordered
// by start line. Recomputed each call from the line slice (cheap brace scan).
func (this *CodeEditor) FoldRegions() []foldRegion {
	return computeFoldRegions(this.lines)
}

// IsFolded reports whether the region starting at the given line is collapsed.
func (this *CodeEditor) IsFolded(startLine int) bool {
	return this.foldedLines[startLine]
}

// ToggleFold collapses or expands the foldable region that starts at the given
// line. A line that is not the start of a foldable region is ignored.
func (this *CodeEditor) ToggleFold(startLine int) {
	if this.foldedLines == nil {
		this.foldedLines = make(map[int]bool)
	}
	if _, ok := this.foldRegionAt(startLine); !ok {
		return
	}
	if this.foldedLines[startLine] {
		delete(this.foldedLines, startLine)
	} else {
		this.foldedLines[startLine] = true
	}
	this.clampCursorVisible()
	this.Self().Update()
}

// FoldAll collapses every foldable region.
func (this *CodeEditor) FoldAll() {
	if this.foldedLines == nil {
		this.foldedLines = make(map[int]bool)
	}
	for _, reg := range this.FoldRegions() {
		this.foldedLines[reg.startLine] = true
	}
	this.clampCursorVisible()
	this.Self().Update()
}

// UnfoldAll expands every collapsed region.
func (this *CodeEditor) UnfoldAll() {
	this.foldedLines = make(map[int]bool)
	this.Self().Update()
}

// GoToDefinitionAtCursor jumps to the definition of the identifier at the
// caret using the AST-based FindDefinition resolver. When the target lives in
// the current file (or no file path is set) the editor scrolls to it directly;
// when it lives in a sibling .go file it is delegated to the cross-file
// navigation callback the host editor wires up. No-op when no identifier is
// under the cursor or no definition can be resolved.
func (this *CodeEditor) GoToDefinitionAtCursor() {
	this.clampCursor()
	word := this.wordAtCursor()
	if word == "" {
		return
	}
	target := FindDefinition(word, this.filePath, this.Text())
	if target == nil {
		return
	}
	this.pushNavPosition()
	if target.FilePath == this.filePath || this.filePath == "" {
		this.goToLine(target.Line)
		this.ScrollToLine(target.Line)
		return
	}
	if this.cbNavigate != nil {
		this.cbNavigate(target.FilePath, target.Line)
	}
}

// HighlightReferencesAtCursor finds every occurrence of the identifier at the
// caret in the current buffer (AST-based FindReferences) and routes them
// through the find bar's findMatches overlay so they are highlighted in place.
// The user dismisses the highlight with Esc the same way they dismiss a normal
// find (the find bar's Esc handler already clears findMatches).
func (this *CodeEditor) HighlightReferencesAtCursor() {
	this.clampCursor()
	word := this.wordAtCursor()
	if word == "" {
		return
	}
	refs := FindReferences(word, this.Text())
	this.findText = word
	this.findCursor = len([]rune(word))
	this.findMatches = nil
	wordLen := len([]rune(word))
	for _, r := range refs {
		this.findMatches = append(this.findMatches, findMatch{
			line: r.Line,
			col:  r.Column,
			end:  r.Column + wordLen,
		})
	}
	this.findCurrentIdx = 0
	this.findActive = true
	this.Self().Update()
}

// visibleLineIndices returns the ordered line indices currently drawn, honoring
// the active folds. It is the bridge between the pure visibleLines helper and
// the editor's live state.
func (this *CodeEditor) visibleLineIndices() []int {
	return visibleLines(len(this.lines), this.foldedLines, this.FoldRegions())
}

// lineToVisualRow maps a line index to its visual row (its position among the
// visible lines). If the line is hidden inside a fold, the row of the nearest
// preceding visible line is returned so cursor math lands on the fold header.
func (this *CodeEditor) lineToVisualRow(line int) int {
	vis := this.visibleLineIndices()
	row := 0
	for r, ln := range vis {
		if ln == line {
			return r
		}
		if ln > line {
			break
		}
		row = r
	}
	return row
}

// clampCursorVisible nudges the cursor onto a visible line when the line it sat
// on has just been hidden by a fold, so the caret never disappears into a
// collapsed block.
func (this *CodeEditor) clampCursorVisible() {
	vis := this.visibleLineIndices()
	if len(vis) == 0 {
		return
	}
	visset := make(map[int]bool, len(vis))
	for _, ln := range vis {
		visset[ln] = true
	}
	if visset[this.cursorLine] {
		return
	}
	// Walk back to the nearest visible line at or above the cursor (the fold
	// header that now stands in for the hidden body).
	target := vis[0]
	for _, ln := range vis {
		if ln <= this.cursorLine {
			target = ln
		} else {
			break
		}
	}
	this.cursorLine = target
	this.clampCursor()
}

package gui

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"os/exec"
	"silk/core"
	"silk/paint"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

func init() {
	core.RegisterFactory("gui.CodeEditor", TypeOf((*CodeEditor)(nil)))
}

// Pathological-input guards. Real source files can occasionally arrive at the
// editor with shapes the syntax pipeline was never tuned for: a minified bundle
// on a single line, a generated file with hundreds of thousands of lines, or an
// obfuscated source full of nested braces. The constants below let the editor
// degrade gracefully instead of stalling the UI:
//
//   - maxHighlightLineLength: lines at or beyond this byte length skip
//     tokenization and render as a single plain-text run. Syntax coloring is
//     lost on that line; everything else (editing, find, scroll) still works.
//   - maxFoldComputeLines: files at or beyond this line count skip fold-region
//     computation. computeFoldRegions returns nil and folding is silently
//     unavailable until the file shrinks; all other editor features stay live.
//
// Thresholds are deliberately generous — typical source stays well under both.
const (
	maxHighlightLineLength = 4000
	maxFoldComputeLines    = 50000
)

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
	tokNormal:   {R: 212, G: 212, B: 222, A: 255},
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

// codeEditorFindRowHeight is the height of a single row of the find/replace bar.
// The bar is one row tall for find-only and two rows tall when the replace input
// is shown; findBarHeight always holds the current total so topOffset() reserves
// the right amount of vertical space above the text area.
const codeEditorFindRowHeight = 30

// Find/replace bar layout, shared by drawFindBar and replaceButtonHit so the
// drawn geometry and the click hit-test never drift.
const (
	findBarInputX = 60.0  // left edge of both input boxes
	findBarInputW = 250.0 // width of both input boxes
	findBarBtnW   = 66.0  // width of the Replace / All buttons
)

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
	// replaceVisible toggles the second (replace) input row. While it is true the
	// bar is two rows tall (findBarHeight == codeEditorFindRowHeight*2) so the
	// text area is pushed down accordingly. replaceFocused routes text input and
	// caret editing to the replace input instead of the find input.
	replaceVisible bool
	replaceFocused bool
	replaceText    string
	replaceCursor  int // cursor position within replaceText

	// --- Indentation Guides ---
	showIndentGuides bool

	// --- Code Completion ---
	completion *CompletionPopup
	// externalCompletions holds candidates injected by an external provider
	// (e.g. silkide's gopls LSP client). They persist until replaced or
	// cleared and are merged into the popup on every (re)build.
	externalCompletions []ExternalCompletion

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

	// --- LSP host hooks (hover + signature help) ---
	// These are host-driven signals: the editor only reports WHERE the user is
	// (line/col + global anchor) and lets the host (silkide) run the async gopls
	// RPC and display the result via ShowToolTip. The editor never blocks on the
	// fetch. Both are nil-safe: with no callback registered they are a no-op.
	cbHoverRequested     func(line, col int, gx, gy float64) // mouse settled over an identifier
	cbSignatureRequested func(line, col int)                 // typed a "(" or "," signature trigger
	// hoverReqLine/hoverReqCol track the identifier (line, word-start col) last
	// reported via cbHoverRequested so we fire exactly once per identifier as the
	// mouse moves, instead of on every pixel. -1 means "nothing reported yet".
	hoverReqLine int
	hoverReqCol  int

	// --- Run-Test Gutter Marker ---
	// cbTestRun is fired when the user clicks the run-test ▶ gutter marker next
	// to a top-level Go test function in a *_test.go file. The host (silkide) runs
	// `go test -run ^Name$`. Nil-safe: with no callback the marker still draws,
	// and clicking it is consumed (no breakpoint toggle) but does nothing.
	cbTestRun func(name string)

	// --- Git Gutter ---
	gitStatus map[int]GitLineStatus

	// --- Coverage Gutter ---
	// coverage maps line (0-based, matching breakpoints/bookmarks) -> covered.
	// Nil means "no coverage data" and the stripe is invisible. A line missing
	// from a non-nil map draws no stripe either, so neutral / no-data lines stay
	// clean. The host pushes this map via SetCoverage; the editor only renders
	// it (the gocoverage parser lives outside the editor).
	coverage map[int]bool

	// --- Diff Gutter Markers ---
	// diffMarkers maps line (0-based, matching breakpoints/bookmarks/coverage)
	// -> the VCS diff state the host wants rendered. The editor does NOT compute
	// the diff; the host pushes a precomputed set via SetDiffMarkers and the
	// editor only renders a coloured bar (added/modified) or a small triangle
	// (removed) per entry. Nil / absent lines draw nothing, so unchanged lines
	// stay clean. Like breakpoints, this is a UI/state layer and is NOT re-mapped
	// when lines are inserted/deleted.
	diffMarkers map[int]DiffMarkerKind

	// --- Blame (Annotate) Layer ---
	// blame maps line (0-based, matching breakpoints/diffMarkers) -> a host-fed
	// annotation string, conventionally "shorthash author". The editor does NOT
	// run git; the host computes blame and pushes the set via SetBlameAnnotations.
	// When blameVisible is true the annotation is drawn dim and right-aligned in a
	// fixed-width column pinned to the text area's right edge. It does NOT reserve
	// space between the gutter and text, so gutterW / textOffX are byte-identical
	// whether blame is on or off. Like breakpoints, this is a UI/state layer keyed
	// 0-based and is NOT re-mapped when lines are inserted/deleted.
	blame        map[int]string
	blameVisible bool

	// --- Multi-Cursor Editing ---
	// Primary cursor is (cursorLine, cursorCol). additionalCursors stores
	// extra caret positions that receive the same text input / edits.
	additionalCursors []cursorPos

	// --- Snippet Expansion (Qt Creator / VS Code "tab triggers") ---
	// snippets is the active SnippetSet consulted on Tab. NewGoSnippetSet()
	// is installed by Init; the host may override via SetSnippets. The legacy
	// goSnippets table (used by tryExpandSnippet, ${N:text} placeholders) is
	// independent and still consulted afterwards.
	snippets *SnippetSet

	// --- Tokenization cache (Draw hot path) ---
	// tokenCache memoizes tokenizeLine output per line index, keyed by a
	// fnv64a hash over the line bytes plus the incoming inBlock flag. A
	// stable line short-circuits the per-rune lexer scan; lines that
	// changed yield a hash mismatch and recompute transparently.
	//
	// Invalidation rule (kept deliberately coarse, see rebuildText):
	//   - SetText / undo / redo / any line-count change clears the whole
	//     map, because line-index assignments shift after insert/delete
	//     and a stale entry at the new index would silently mis-color.
	//   - In-line edits (typing a char, line replacement at constant
	//     line count) rely on the hash mismatch to self-invalidate the
	//     affected entry on its next read.
	// Single-threaded by construction: Draw and every mutation handler
	// run on the GLFW main thread, so the map needs no mutex.
	tokenCache       map[int]tokenCacheEntry
	tokenCacheLineCt int // line count at the last cache fill, for shift detection
}

// tokenCacheEntry holds the tokens emitted for a (line index, line bytes,
// inBlock) triple, plus the resulting inBlock state. nextBlock is cached
// alongside tokens because tokenizeLine returns both and downstream code
// (drawHighlightedLine, minimap) consumes both.
type tokenCacheEntry struct {
	hash      uint64
	tokens    []token
	nextBlock bool
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
	this.findBarHeight = codeEditorFindRowHeight
	this.showIndentGuides = true
	this.breadcrumbHeight = 22
	this.showMinimap = true
	this.wordWrap = false
	this.statusBarHeight = 20
	this.hoverErrorLine = -1
	this.hoverLinkLine = -1
	this.hoverReqLine = -1
	this.hoverReqCol = -1
	this.breakpoints = make(map[int]bool)
	this.foldedLines = make(map[int]bool)
	this.snippets = NewGoSnippetSet()
}

// SetSnippets installs a SnippetSet used by Tab to expand triggers. Passing nil
// disables the new-style expansion path; the legacy goSnippets table still
// fires from tryExpandSnippet.
func (this *CodeEditor) SetSnippets(s *SnippetSet) {
	this.snippets = s
}

// Snippets returns the active SnippetSet (may be nil if cleared via SetSnippets).
func (this *CodeEditor) Snippets() *SnippetSet {
	return this.snippets
}

// tryExpandSnippetAtCursor expands a SnippetSet trigger when the identifier
// ending flush at the cursor matches one. Returns true when expansion happened
// (buffer + cursor updated, onChanged fired, redraw scheduled).
//
// The check is conservative: the cursor must sit immediately after an
// identifier rune AND not in the middle of an identifier (no identifier rune
// to the right). When the snippets field is nil or no trigger matches, returns
// false so the caller can fall through to the legacy ${N:} path / Tab insert.
func (this *CodeEditor) tryExpandSnippetAtCursor() bool {
	if this.snippets == nil {
		return false
	}
	this.clampCursor()
	if this.cursorLine >= len(this.lines) {
		return false
	}
	runes := []rune(this.lines[this.cursorLine])
	end := this.cursorCol
	if end > len(runes) {
		end = len(runes)
	}
	// Must be at end of an identifier: nothing identifier-ish to the right.
	if end < len(runes) && isIdentPart(runes[end]) {
		return false
	}
	// Walk back to find the identifier start.
	start := end
	for start > 0 && isIdentPart(runes[start-1]) {
		start--
	}
	if start == end {
		return false
	}
	word := string(runes[start:end])

	// Compute the global rune offset of the cursor in Text(): sum of rune
	// lengths of all preceding lines (each plus a newline) plus cursorCol.
	cursorOffset := 0
	for i := 0; i < this.cursorLine; i++ {
		cursorOffset += len([]rune(this.lines[i])) + 1 // +1 for '\n'
	}
	cursorOffset += end

	newBuf, newCur, ok := this.snippets.Expand(this.Text(), cursorOffset, word)
	if !ok {
		return false
	}

	// Record undo before mutating: store the abbreviation we're replacing.
	this.pushUndo(editAction{kind: 2, line: this.cursorLine, col: start, text: "", oldText: word})

	// Replace buffer and re-derive line/col from the returned rune offset.
	this.lines = strings.Split(newBuf, "\n")
	if len(this.lines) == 0 {
		this.lines = []string{""}
	}
	// Map newCur (rune offset) back to (line, col).
	remaining := newCur
	newLine, newCol := 0, 0
	for i, ln := range this.lines {
		ll := len([]rune(ln))
		if remaining <= ll {
			newLine, newCol = i, remaining
			break
		}
		remaining -= ll + 1 // consume the line and its newline
	}
	this.cursorLine = newLine
	this.cursorCol = newCol
	this.clearSelection()
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
	return true
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
	this.clearTokenCache()
	this.tokenCacheLineCt = len(this.lines)
	this.Self().Update()
}

// Text returns the full editor content.
func (this *CodeEditor) Text() string {
	return strings.Join(this.lines, "\n")
}

// ReplaceAllText swaps the whole buffer for s while PRESERVING undo history,
// the caret, and scroll — unlike SetText, which resets all three. Used by
// LSP format / rename / code-action application so a single Cmd+Z reverts the
// change. Records one kind-3 (full-text-replace) undo entry, fires SigChanged
// so the host re-syncs gopls, and no-ops when s equals the current text.
func (this *CodeEditor) ReplaceAllText(s string) {
	oldFull := strings.Join(this.lines, "\n")
	if s == oldFull {
		return
	}
	saveLine, saveCol := this.cursorLine, this.cursorCol
	this.pushUndo(editAction{kind: 3, line: saveLine, col: saveCol, text: s, oldText: oldFull})
	this.lines = strings.Split(s, "\n")
	if len(this.lines) == 0 {
		this.lines = []string{""}
	}
	this.clampCursor()
	this.clearSelection()
	this.additionalCursors = nil
	this.clearTokenCache()
	this.tokenCacheLineCt = len(this.lines)
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
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

// CursorCol returns the current 0-based cursor column (rune index within
// the line). LSP callers need it to build a textDocument position for
// completion / go-to-definition requests.
func (this *CodeEditor) CursorCol() int {
	return this.cursorCol
}

// CursorUTF16Col returns the caret column as a UTF-16 code-unit offset within
// its line — the encoding LSP mandates for a Position's `character` field.
// It parallels CursorCol, which stays a rune index for internal editing: the
// two agree for ASCII / BMP text but diverge once a non-BMP rune (emoji, a
// CJK-extension glyph) precedes the caret, where each such rune counts as two
// UTF-16 units. LSP callers MUST send this, not CursorCol, as the character
// offset or completion / hover / definition / rename resolve at the wrong column.
func (this *CodeEditor) CursorUTF16Col() int {
	if this.cursorLine < 0 || this.cursorLine >= len(this.lines) {
		return 0
	}
	return utf16ColumnOf(this.lines[this.cursorLine], this.cursorCol)
}

// utf16ColumnOf counts the UTF-16 code units in the first runeCol runes of
// line. runeCol is clamped to the line's rune length, so a caret past the end
// maps to the full line width. utf16.RuneLen reports 2 for a non-BMP rune and
// 1 for a BMP one; an invalid rune (RuneLen == -1) is counted as 1.
func utf16ColumnOf(line string, runeCol int) int {
	runes := []rune(line)
	if runeCol > len(runes) {
		runeCol = len(runes)
	}
	col := 0
	for _, r := range runes[:runeCol] {
		if n := utf16.RuneLen(r); n > 0 {
			col += n
		} else {
			col++
		}
	}
	return col
}

// caretLocalXY returns the primary caret's position in LOCAL widget
// coordinates: x is the caret's left edge, y is the BOTTOM of the caret's
// line, so a popup anchored there sits just under the line instead of
// covering it. The math mirrors the primary-cursor draw in Draw()
// (cx = textOffX + measureText(prefix), cy = visualRow*lh - scrollY +
// topOffset) — keep the two in sync. If the caret is scrolled out of view,
// the result is clamped into the visible text viewport (right of the
// gutter, left of the minimap, between the top bars and the status bar) so
// a popup anchored to it never lands off-widget.
func (this *CodeEditor) caretLocalXY() (float64, float64) {
	fe := this.font.FontExtents()
	lh := fe.Height + 2
	topOff := this.topOffset()

	// Clamp line/col locally — same bounds as the draw path's clampCursor,
	// but without mutating cursor state from a read-only accessor.
	line := this.cursorLine
	if line < 0 {
		line = 0
	}
	if line >= len(this.lines) {
		line = len(this.lines) - 1
	}
	prefix := ""
	if line >= 0 && line < len(this.lines) {
		runes := []rune(this.lines[line])
		col := this.cursorCol
		if col < 0 {
			col = 0
		}
		if col > len(runes) {
			col = len(runes)
		}
		prefix = string(runes[:col])
	}
	x := this.gutterW + 10 - this.scrollX + this.measureText(prefix)
	y := float64(this.lineToVisualRow(line))*lh - this.scrollY + topOff + lh // line bottom

	// Viewport clamp: upper bound first so the lower bound wins on
	// degenerate (tiny) editor sizes.
	w, h := this.Size()
	if right := w - this.minimapWidth(); x > right {
		x = right
	}
	if left := this.gutterW + 10; x < left {
		x = left
	}
	if bottom := h - this.statusBarHeight; y > bottom {
		y = bottom
	}
	if top := topOff + lh; y < top {
		y = top
	}
	return x, y
}

// CaretGlobalXY returns the primary caret's position in GLOBAL screen
// coordinates: the caret's left x and the bottom of its line, ready to
// anchor an LSP hover / signature-help popup just below the caret. The
// local position is viewport-clamped (see caretLocalXY), so the anchor
// stays inside the editor even when the caret is scrolled out of view.
func (this *CodeEditor) CaretGlobalXY() (float64, float64) {
	return this.MapToGlobal(this.caretLocalXY())
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

// SigHoverRequested registers the host hook fired when the mouse settles over
// an identifier in the text area. The editor reports (line, col) plus a global
// anchor (gx, gy); the host runs the async LSP Hover RPC and shows the result
// (e.g. via ShowToolTip). The editor does NOT fetch or display hover text — it
// only signals where the user is hovering. The callback fires once per
// identifier (see hoverReqLine/hoverReqCol) and never while the completion
// popup is visible. Passing nil disables the signal (no-op).
func (this *CodeEditor) SigHoverRequested(fn func(line, col int, gx, gy float64)) {
	this.cbHoverRequested = fn
}

// SigSignatureRequested registers the host hook fired when the user types a
// signature trigger ("(" or ","). The editor passes the cursor (line, col)
// AFTER the insert; the host runs the async LSP SignatureHelp RPC and shows the
// result. Typing ")" dismisses any shown help (the editor calls HideToolTip on
// ")"); Esc dismissal is left to the host. Passing nil disables the signal
// (no-op).
func (this *CodeEditor) SigSignatureRequested(fn func(line, col int)) {
	this.cbSignatureRequested = fn
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

// ScrollToLineCol is ScrollToLine that also lands the caret on col (clamped to
// the line). Used by LSP go-to-definition so F12 puts the cursor on the symbol,
// not just its line. col < 0 behaves like ScrollToLine (col 0).
func (this *CodeEditor) ScrollToLineCol(line, col int) {
	this.ScrollToLine(line)
	if col > 0 && this.cursorLine >= 0 && this.cursorLine < len(this.lines) {
		if n := len([]rune(this.lines[this.cursorLine])); col > n {
			col = n
		}
		this.cursorCol = col
		this.Self().Update()
	}
}

// CompletionVisible reports whether the completion popup is currently shown.
// The LSP host uses it to decide whether to REFRESH an open popup with server
// items vs. force-open one (which would hijack Enter/arrows after a newline,
// paste, or programmatic edit).
func (this *CodeEditor) CompletionVisible() bool {
	return this.completion != nil && this.completion.visible
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

// dedupCursorsList is the pure-helper twin of dedupCursors. Removes duplicate
// positions while preserving the order of first occurrence. Used by the
// arrow-key multi-cursor mover (and the multi-cursor unit tests).
func dedupCursorsList(in []cursorPos) []cursorPos {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[cursorPos]bool, len(in))
	out := make([]cursorPos, 0, len(in))
	for _, c := range in {
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// moveAllCursorsBy moves the primary cursor and every additional cursor by
// the same (dLine, dCol) delta, clamping each to its line bounds and
// collapsing duplicates. Used by Left/Right/Up/Down when secondary cursors
// are active. Per-cursor selections are not tracked, so this drops selection
// state.
func (this *CodeEditor) moveAllCursorsBy(dLine, dCol int) {
	move := func(p cursorPos) cursorPos {
		// Horizontal: cross line boundaries when stepping off either end.
		// This matches the single-cursor Left/Right behavior.
		p.col += dCol
		for p.col < 0 && p.line > 0 {
			p.line--
			p.col += len([]rune(this.lines[p.line])) + 1
		}
		for p.line < len(this.lines)-1 && p.col > len([]rune(this.lines[p.line])) {
			p.col -= len([]rune(this.lines[p.line])) + 1
			p.line++
		}
		// Vertical: clamp the column to the new line's length.
		p.line += dLine
		return this.clampCursorPos(p)
	}
	primary := move(cursorPos{line: this.cursorLine, col: this.cursorCol})
	this.cursorLine = primary.line
	this.cursorCol = primary.col
	for i := range this.additionalCursors {
		this.additionalCursors[i] = move(this.additionalCursors[i])
	}
	this.dedupCursors()
}

// moveAllCursorsToLineBound moves every cursor to col 0 (when toEnd is false)
// or to the end of its own line (when toEnd is true). Used by Home/End in
// multi-cursor mode.
func (this *CodeEditor) moveAllCursorsToLineBound(toEnd bool) {
	bound := func(line int) int {
		if !toEnd {
			return 0
		}
		if line < 0 || line >= len(this.lines) {
			return 0
		}
		return len([]rune(this.lines[line]))
	}
	this.cursorCol = bound(this.cursorLine)
	for i := range this.additionalCursors {
		this.additionalCursors[i].col = bound(this.additionalCursors[i].line)
	}
	this.dedupCursors()
}

// --- Pure helpers (used by unit tests) ---
//
// These mirror the *AtAllCursors methods but operate on an explicit
// (text, primary, extras, ins) tuple and return the new state without
// touching the editor. They exist so the multi-cursor edit math can be
// covered by a fast, dependency-free test suite. The mutating *AtAllCursors
// methods remain the production path so existing callers don't change.

// applyInsertAtCursors inserts ins at the primary cursor and every extra
// cursor, processing positions back-to-front so earlier insertions don't
// shift later ones. Returns the new text plus the post-insert cursor
// positions. Only single-line inserts are supported (callers panic on '\n'
// in production via the OnTextInput multi-line fallback).
func applyInsertAtCursors(text string, primary cursorPos, extras []cursorPos, ins string) (string, cursorPos, []cursorPos) {
	insRunes := []rune(ins)
	if len(insRunes) == 0 {
		return text, primary, append([]cursorPos(nil), extras...)
	}
	lines := strings.Split(text, "\n")
	all := append([]cursorPos{primary}, extras...)
	// Descending order: bigger (line, col) first.
	sort.Slice(all, func(i, j int) bool {
		if all[i].line != all[j].line {
			return all[i].line > all[j].line
		}
		return all[i].col > all[j].col
	})
	for _, c := range all {
		if c.line < 0 || c.line >= len(lines) {
			continue
		}
		runes := []rune(lines[c.line])
		if c.col < 0 {
			c.col = 0
		}
		if c.col > len(runes) {
			c.col = len(runes)
		}
		newRunes := make([]rune, 0, len(runes)+len(insRunes))
		newRunes = append(newRunes, runes[:c.col]...)
		newRunes = append(newRunes, insRunes...)
		newRunes = append(newRunes, runes[c.col:]...)
		lines[c.line] = string(newRunes)
	}
	shift := func(p cursorPos) cursorPos {
		extra := 0
		for _, c := range all {
			if c.line == p.line && c.col < p.col {
				extra += len(insRunes)
			}
		}
		p.col += extra + len(insRunes)
		return p
	}
	newPrimary := shift(primary)
	newExtras := make([]cursorPos, 0, len(extras))
	for _, e := range extras {
		newExtras = append(newExtras, shift(e))
	}
	newExtras = dedupCursorsList(append([]cursorPos{newPrimary}, newExtras...))
	// Drop the primary from the head (first element) — the caller tracks it
	// separately. dedupCursorsList preserves order of first occurrence.
	if len(newExtras) > 0 && newExtras[0] == newPrimary {
		newExtras = newExtras[1:]
	}
	return strings.Join(lines, "\n"), newPrimary, newExtras
}

// applyBackspaceAtCursors deletes one rune to the left at the primary and
// every extra cursor, processed back-to-front. Line-joins (at col 0) collapse
// the current line into the previous one. Cursors at (0,0) are no-ops.
func applyBackspaceAtCursors(text string, primary cursorPos, extras []cursorPos) (string, cursorPos, []cursorPos) {
	lines := strings.Split(text, "\n")
	all := append([]cursorPos{primary}, extras...)
	sort.Slice(all, func(i, j int) bool {
		if all[i].line != all[j].line {
			return all[i].line > all[j].line
		}
		return all[i].col > all[j].col
	})
	type op struct {
		line    int
		col     int
		joined  bool
		prevLen int
	}
	var ops []op
	for _, c := range all {
		if c.line < 0 || c.line >= len(lines) {
			continue
		}
		if c.col > 0 {
			runes := []rune(lines[c.line])
			if c.col > len(runes) {
				c.col = len(runes)
			}
			newRunes := append(runes[:c.col-1], runes[c.col:]...)
			lines[c.line] = string(newRunes)
			ops = append(ops, op{line: c.line, col: c.col - 1})
		} else if c.line > 0 {
			prev := lines[c.line-1]
			prevLen := len([]rune(prev))
			lines[c.line-1] = prev + lines[c.line]
			lines = append(lines[:c.line], lines[c.line+1:]...)
			ops = append(ops, op{line: c.line, col: 0, joined: true, prevLen: prevLen})
		}
	}
	shift := func(p cursorPos) cursorPos {
		for _, o := range ops {
			if o.joined {
				if p.line == o.line {
					p.line = o.line - 1
					p.col += o.prevLen
				} else if p.line > o.line {
					p.line--
				}
			} else {
				if p.line == o.line && p.col > o.col {
					p.col--
				}
			}
		}
		return p
	}
	newPrimary := shift(primary)
	newExtras := make([]cursorPos, 0, len(extras))
	for _, e := range extras {
		newExtras = append(newExtras, shift(e))
	}
	newExtras = dedupCursorsList(append([]cursorPos{newPrimary}, newExtras...))
	if len(newExtras) > 0 && newExtras[0] == newPrimary {
		newExtras = newExtras[1:]
	}
	return strings.Join(lines, "\n"), newPrimary, newExtras
}

// applyDeleteAtCursors forward-deletes one rune at the primary and every
// extra cursor, processed back-to-front. Line-joins (at end-of-line) collapse
// the next line into the current one.
func applyDeleteAtCursors(text string, primary cursorPos, extras []cursorPos) (string, cursorPos, []cursorPos) {
	lines := strings.Split(text, "\n")
	all := append([]cursorPos{primary}, extras...)
	sort.Slice(all, func(i, j int) bool {
		if all[i].line != all[j].line {
			return all[i].line > all[j].line
		}
		return all[i].col > all[j].col
	})
	type op struct {
		line    int
		col     int
		joined  bool
		nextLen int
	}
	var ops []op
	for _, c := range all {
		if c.line < 0 || c.line >= len(lines) {
			continue
		}
		runes := []rune(lines[c.line])
		if c.col < 0 {
			c.col = 0
		}
		if c.col < len(runes) {
			newRunes := append(runes[:c.col], runes[c.col+1:]...)
			lines[c.line] = string(newRunes)
			ops = append(ops, op{line: c.line, col: c.col})
		} else if c.line < len(lines)-1 {
			curLen := len(runes)
			lines[c.line] = lines[c.line] + lines[c.line+1]
			lines = append(lines[:c.line+1], lines[c.line+2:]...)
			ops = append(ops, op{line: c.line, col: curLen, joined: true, nextLen: curLen})
		}
	}
	shift := func(p cursorPos) cursorPos {
		for _, o := range ops {
			if o.joined {
				if p.line == o.line+1 {
					p.line = o.line
					p.col += o.nextLen
				} else if p.line > o.line+1 {
					p.line--
				}
			} else {
				if p.line == o.line && p.col > o.col {
					p.col--
				}
			}
		}
		return p
	}
	newPrimary := shift(primary)
	newExtras := make([]cursorPos, 0, len(extras))
	for _, e := range extras {
		newExtras = append(newExtras, shift(e))
	}
	newExtras = dedupCursorsList(append([]cursorPos{newPrimary}, newExtras...))
	if len(newExtras) > 0 && newExtras[0] == newPrimary {
		newExtras = newExtras[1:]
	}
	return strings.Join(lines, "\n"), newPrimary, newExtras
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
		line    int
		col     int // column of the deleted character
		joined  bool
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
//
// This is the funnel every mutation path eventually hits, so it doubles as
// the tokenization-cache invalidation hook. Rule:
//   - line count changed (insert/delete/paste-multiline/undo-replace) →
//     drop the whole map, because the line-index keys have shifted under
//     us and a stale entry would silently mis-color a different line.
//   - line count unchanged (in-line typing, character delete, line
//     replacement) → leave the map alone; the fnv64a hash check inside
//     tokenizeLineCached detects the byte change and recomputes that
//     single entry on its next Draw.
func (this *CodeEditor) rebuildText() {
	this.text = strings.Join(this.lines, "\n")
	if len(this.lines) != this.tokenCacheLineCt {
		this.clearTokenCache()
		this.tokenCacheLineCt = len(this.lines)
	}
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

// runeWindowEqual reports whether the equal-length rune windows a and b match,
// folding case (unicode.ToLower) when caseSensitive is false. The caller
// guarantees len(a) == len(b).
func runeWindowEqual(a, b []rune, caseSensitive bool) bool {
	for i := range a {
		if caseSensitive {
			if a[i] != b[i] {
				return false
			}
		} else if unicode.ToLower(a[i]) != unicode.ToLower(b[i]) {
			return false
		}
	}
	return true
}

// findMatches returns every occurrence of query in text as line/rune-column
// ranges. Matching folds case when caseSensitive is false. The scan is
// rune-based (byte-safe for any input) and non-overlapping — it resumes past
// each hit — so "aa" in "aaaa" yields two matches, not three. An empty query
// yields no matches (never "all positions"). A query is treated as single-line:
// a query containing '\n' can never match, matching the editor's line-oriented
// find. This is the pure core wired to the widget by findUpdateMatches.
func findMatches(text, query string, caseSensitive bool) []findMatch {
	if query == "" {
		return nil
	}
	qr := []rune(query)
	qlen := len(qr)
	var out []findMatch
	for li, line := range strings.Split(text, "\n") {
		lr := []rune(line)
		for c := 0; c+qlen <= len(lr); {
			if runeWindowEqual(lr[c:c+qlen], qr, caseSensitive) {
				out = append(out, findMatch{line: li, col: c, end: c + qlen})
				c += qlen // non-overlapping: skip past the whole match
			} else {
				c++
			}
		}
	}
	return out
}

// replaceAllInText replaces every non-overlapping occurrence of query in text
// with repl and returns the new text plus the number of replacements. Matching
// folds case when caseSensitive is false. An empty query is a no-op (text
// unchanged, count 0). Replacements are counted on the ORIGINAL matches and the
// scan always advances past the matched span (never into repl), so a replacement
// that itself contains the query (e.g. "a"->"aa") counts the original hits and
// cannot loop.
//
// Case-sensitive uses strings.ReplaceAll (a single non-overlapping pass), which
// sidesteps the offset drift a match-by-match splice would suffer. Case-
// insensitive walks the runes with the same non-overlapping, advance-past-match
// rule so it is byte-safe and drift-free too.
func replaceAllInText(text, query, repl string, caseSensitive bool) (string, int) {
	if query == "" {
		return text, 0
	}
	if caseSensitive {
		n := strings.Count(text, query)
		if n == 0 {
			return text, 0
		}
		return strings.ReplaceAll(text, query, repl), n
	}
	tr := []rune(text)
	qr := []rune(query)
	qlen := len(qr)
	var b strings.Builder
	count := 0
	for i := 0; i < len(tr); {
		if i+qlen <= len(tr) && runeWindowEqual(tr[i:i+qlen], qr, false) {
			b.WriteString(repl)
			count++
			i += qlen
		} else {
			b.WriteRune(tr[i])
			i++
		}
	}
	return b.String(), count
}

// findUpdateMatches recomputes the match overlay for the current findText. Find
// is case-insensitive by default (see findMatches).
func (this *CodeEditor) findUpdateMatches() {
	this.findMatches = findMatches(this.Text(), this.findText, false)
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

// closeFindBar hides the find/replace bar and clears its transient state,
// restoring the single-row height so topOffset() reserves the right space.
func (this *CodeEditor) closeFindBar() {
	this.findActive = false
	this.replaceVisible = false
	this.replaceFocused = false
	this.findMatches = nil
	this.findBarHeight = codeEditorFindRowHeight
	this.Self().Update()
}

// toggleReplaceRow shows/hides the replace input row and resizes the bar so the
// text area's top offset tracks the taller bar. Focus follows the replace input
// when it becomes visible.
func (this *CodeEditor) toggleReplaceRow() {
	this.replaceVisible = !this.replaceVisible
	this.replaceFocused = this.replaceVisible
	if this.replaceVisible {
		this.findBarHeight = codeEditorFindRowHeight * 2
	} else {
		this.findBarHeight = codeEditorFindRowHeight
	}
	this.Self().Update()
}

// selectMatchAtOrAfter points findCurrentIdx at the first match at or after
// (line,col), wrapping to the first match when none follow, and moves the caret
// there. Used after a single Replace so the search continues past the text just
// inserted.
func (this *CodeEditor) selectMatchAtOrAfter(line, col int) {
	if len(this.findMatches) == 0 {
		return
	}
	target := 0
	for i, m := range this.findMatches {
		if m.line > line || (m.line == line && m.col >= col) {
			target = i
			break
		}
	}
	this.findCurrentIdx = target
	m := this.findMatches[target]
	this.cursorLine = m.line
	this.cursorCol = m.col
	this.clearSelection()
	this.ensureCursorVisible()
	this.Self().Update()
}

// replaceCurrent replaces the current match with replaceText, routes the edit
// through the text funnel (rebuildText fires onChanged / LSP didChange and
// invalidates the token cache), then re-runs the search and advances to the next
// match. The search resumes AFTER the inserted text so a replacement that itself
// contains the query is not immediately re-matched.
func (this *CodeEditor) replaceCurrent() {
	if this.findText == "" || len(this.findMatches) == 0 {
		return
	}
	if this.findCurrentIdx < 0 || this.findCurrentIdx >= len(this.findMatches) {
		this.findCurrentIdx = 0
	}
	m := this.findMatches[this.findCurrentIdx]
	if m.line < 0 || m.line >= len(this.lines) {
		return
	}
	runes := []rune(this.lines[m.line])
	col, end := m.col, m.end
	if col > len(runes) {
		col = len(runes)
	}
	if end > len(runes) {
		end = len(runes)
	}
	if col > end {
		col = end
	}
	this.lines[m.line] = string(runes[:col]) + this.replaceText + string(runes[end:])
	this.rebuildText()
	afterLine := m.line
	afterCol := col + len([]rune(this.replaceText))
	this.findUpdateMatches()
	this.selectMatchAtOrAfter(afterLine, afterCol)
}

// replaceAll replaces every match in one pass via replaceAllInText, records a
// single undoable full-text edit (kind 3, mirroring rename refactoring), routes
// the change through rebuildText, then re-runs the search over the new buffer.
// It returns the number of replacements. One-pass replacement counts the
// ORIGINAL matches and is immune to the offset drift a match-by-match splice
// would cause.
func (this *CodeEditor) replaceAll() int {
	if this.findText == "" {
		return 0
	}
	oldFullText := strings.Join(this.lines, "\n")
	newFullText, n := replaceAllInText(oldFullText, this.findText, this.replaceText, false)
	if n == 0 || oldFullText == newFullText {
		return n
	}
	this.pushUndo(editAction{
		kind:    3,
		line:    this.cursorLine,
		col:     this.cursorCol,
		text:    newFullText,
		oldText: oldFullText,
	})
	this.lines = strings.Split(newFullText, "\n")
	if len(this.lines) == 0 {
		this.lines = []string{""}
	}
	this.rebuildText()
	this.clampCursor()
	this.findUpdateMatches()
	this.findCurrentIdx = 0
	this.ensureCursorVisible()
	this.Self().Update()
	return n
}

// replaceButtonHit returns which replace-row button (if any) contains (x,y):
// 1 = Replace, 2 = Replace All, 0 = none. Geometry mirrors drawFindBar; keep the
// two in sync.
func (this *CodeEditor) replaceButtonHit(x, y float64) int {
	if !this.findActive || !this.replaceVisible {
		return 0
	}
	rowH := float64(codeEditorFindRowHeight)
	if y < rowH || y >= rowH*2 {
		return 0
	}
	btnX := findBarInputX + findBarInputW + 10
	if x >= btnX && x < btnX+findBarBtnW {
		return 1
	}
	allX := btnX + findBarBtnW + 6
	if x >= allX && x < allX+findBarBtnW {
		return 2
	}
	return 0
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

// --- Auto-close, Join, Trim, Duplicate line operations ---

// autoCloseFor maps an opening bracket or quote to the closing rune the editor
// auto-inserts after it. ok is false for any rune with no auto-close pair.
func autoCloseFor(open rune) (rune, bool) {
	switch open {
	case '(':
		return ')', true
	case '[':
		return ']', true
	case '{':
		return '}', true
	case '"':
		return '"', true
	case '`':
		return '`', true
	}
	return 0, false
}

// autoCloseOpenerFor reports the (open, closer) pair when s is exactly one opener
// rune; ok is false for multi-rune input or a non-opener.
func autoCloseOpenerFor(s string) (open, closer rune, ok bool) {
	r := []rune(s)
	if len(r) != 1 {
		return 0, 0, false
	}
	if cl, has := autoCloseFor(r[0]); has {
		return r[0], cl, true
	}
	return 0, 0, false
}

// isAutoCloseCloser reports whether s is exactly one closing bracket/quote the
// editor "types over" when the same char already sits under the caret.
func isAutoCloseCloser(s string) bool {
	switch s {
	case ")", "]", "}", "\"", "`":
		return true
	}
	return false
}

// wrapSelectionWith surrounds the current selection with the open/closer pair and
// leaves the selection on the inner (original) text. Recorded as a single
// undoable replace so one undo removes both delimiters, and routed through
// rebuildText so onChanged/tokenize fire.
func (this *CodeEditor) wrapSelectionWith(open, closer rune) {
	sl, sc, _, _ := this.selectionRange()
	selText := this.SelectedText()
	newText := string(open) + selText + string(closer)
	this.DeleteSelection()
	this.pushUndo(editAction{kind: 2, line: sl, col: sc, text: newText, oldText: selText})
	this.cursorLine = sl
	this.cursorCol = sc
	this.insertRawText(newText)
	// Restore the selection over the inner text (between the delimiters).
	this.hasSelection = true
	this.selStartLine = sl
	this.selStartCol = sc + 1
	this.selEndLine = this.cursorLine
	this.selEndCol = this.cursorCol - 1
	this.cursorLine = this.selEndLine
	this.cursorCol = this.selEndCol
	this.rebuildText()
}

// joinTwoLines joins a and b with a single space, collapsing a's trailing and
// b's leading whitespace. A blank side contributes no extra space.
func joinTwoLines(a, b string) string {
	a = strings.TrimRight(a, " \t")
	b = strings.TrimLeft(b, " \t")
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + " " + b
}

// joinLinesInText joins the inclusive line range [from, to] into a single line,
// collapsing each break to one space (joinTwoLines). from>=to (a single line or
// past the end) is a no-op. The input slice is not mutated.
func joinLinesInText(lines []string, from, to int) []string {
	if len(lines) == 0 {
		return lines
	}
	if from < 0 {
		from = 0
	}
	if to >= len(lines) {
		to = len(lines) - 1
	}
	if from >= to {
		return lines
	}
	joined := lines[from]
	for i := from + 1; i <= to; i++ {
		joined = joinTwoLines(joined, lines[i])
	}
	out := make([]string, 0, len(lines)-(to-from))
	out = append(out, lines[:from]...)
	out = append(out, joined)
	out = append(out, lines[to+1:]...)
	return out
}

// JoinLines merges the current line with the next (or every line of a multi-line
// selection) into one, collapsing each break to a single space. A no-op on the
// last line. Recorded as one undoable full-text edit routed through rebuildText.
func (this *CodeEditor) JoinLines() {
	this.clampCursor()
	from, to := this.cursorLine, this.cursorLine+1
	if this.hasSelection {
		sl, _, el, _ := this.selectionRange()
		from, to = sl, el
		if from == to {
			to = from + 1
		}
	}
	if to >= len(this.lines) {
		return
	}
	oldFull := strings.Join(this.lines, "\n")
	caretCol := len([]rune(strings.TrimRight(this.lines[from], " \t")))
	newLines := joinLinesInText(this.lines, from, to)
	newFull := strings.Join(newLines, "\n")
	if newFull == oldFull {
		return
	}
	this.pushUndo(editAction{kind: 3, line: this.cursorLine, col: this.cursorCol, text: newFull, oldText: oldFull})
	this.lines = newLines
	this.clearSelection()
	this.cursorLine = from
	this.cursorCol = caretCol
	this.clampCursor()
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
}

// trimTrailingInText removes trailing spaces and tabs from every line of text,
// preserving interior whitespace and empty lines.
func trimTrailingInText(text string) string {
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " \t")
	}
	return strings.Join(lines, "\n")
}

// TrimTrailingWhitespace strips trailing spaces/tabs from every line. The caret
// column is clamped to its (possibly shorter) line. Callable by the host and
// recorded as one undoable full-text edit routed through rebuildText.
func (this *CodeEditor) TrimTrailingWhitespace() {
	oldFull := strings.Join(this.lines, "\n")
	newFull := trimTrailingInText(oldFull)
	if newFull == oldFull {
		return
	}
	saveLine, saveCol := this.cursorLine, this.cursorCol
	this.pushUndo(editAction{kind: 3, line: saveLine, col: saveCol, text: newFull, oldText: oldFull})
	this.lines = strings.Split(newFull, "\n")
	if len(this.lines) == 0 {
		this.lines = []string{""}
	}
	this.clearSelection()
	this.cursorLine = saveLine
	this.cursorCol = saveCol
	this.clampCursor()
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
}

// duplicateLinesInText returns lines with the inclusive block [from, to] copied
// immediately below itself. The input slice is not mutated.
func duplicateLinesInText(lines []string, from, to int) []string {
	if len(lines) == 0 {
		return lines
	}
	if from < 0 {
		from = 0
	}
	if to >= len(lines) {
		to = len(lines) - 1
	}
	if from > to {
		from, to = to, from
	}
	block := make([]string, to-from+1)
	copy(block, lines[from:to+1])
	out := make([]string, 0, len(lines)+len(block))
	out = append(out, lines[:to+1]...)
	out = append(out, block...)
	out = append(out, lines[to+1:]...)
	return out
}

// DuplicateLines copies the current line (or each line of a multi-line
// selection) below itself. With no selection it reuses duplicateLine (the Cmd+D
// empty-selection fallback); a selection duplicates the whole block as one
// undoable full-text edit and re-selects the copy. Routed through rebuildText.
func (this *CodeEditor) DuplicateLines() {
	this.clampCursor()
	if !this.hasSelection {
		this.duplicateLine()
		return
	}
	sl, sc, el, ec := this.selectionRange()
	oldFull := strings.Join(this.lines, "\n")
	newLines := duplicateLinesInText(this.lines, sl, el)
	newFull := strings.Join(newLines, "\n")
	this.pushUndo(editAction{kind: 3, line: this.cursorLine, col: this.cursorCol, text: newFull, oldText: oldFull})
	this.lines = newLines
	span := el - sl + 1
	this.hasSelection = true
	this.selStartLine = sl + span
	this.selStartCol = sc
	this.selEndLine = el + span
	this.selEndCol = ec
	this.cursorLine = this.selEndLine
	this.cursorCol = ec
	this.clampCursor()
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

// --- AST Rename at Cursor (host-driven, F2) ---
//
// RenameSymbolAtCursor renames the Go identifier under the cursor across the
// current buffer using code_refactor.go::RenameSymbolCount. The host (silkide)
// wires F2 to this method via an input dialog — the editor itself does NOT
// bind a key; this matches the existing host-driven design of go-to-definition
// and find-references.
//
// On success, the buffer is replaced, the changed callback fires, the cursor
// snaps to the same byte offset (clamped), and (oldName, count, nil) is
// returned. On any failure (empty word at cursor, parse error, invalid
// newName, name collision) the buffer is left untouched and an error is
// returned alongside (oldName, 0).
func (this *CodeEditor) RenameSymbolAtCursor(newName string) (string, int, error) {
	oldName := this.wordAtCursor()
	if oldName == "" {
		return "", 0, errors.New("rename: no identifier at cursor")
	}
	oldSrc := this.Text()
	// Byte offset of the cursor in the OLD source. We restore the cursor to
	// the same byte position after the rename; if the name length changed,
	// this keeps the caret on roughly the same character.
	oldOffset := byteOffsetForCursor(this.lines, this.cursorLine, this.cursorCol)

	count, newSrc, err := RenameSymbolCount(oldSrc, oldName, newName)
	if err != nil {
		return oldName, 0, err
	}
	if count == 0 || newSrc == oldSrc {
		// No-op (e.g. oldName == newName): treat as success with count 0.
		return oldName, 0, nil
	}

	this.lines = strings.Split(newSrc, "\n")
	if len(this.lines) == 0 {
		this.lines = []string{""}
	}
	// Map the old byte offset back to (line, col) in the new source. If the
	// offset overshoots the new buffer, clamp to end of file.
	newLine, newCol := cursorForByteOffset(this.lines, oldOffset)
	this.cursorLine = newLine
	this.cursorCol = newCol
	this.clampCursor()
	this.clearSelection()

	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
	return oldName, count, nil
}

// byteOffsetForCursor returns the byte offset of (line, col) in the joined
// "\n"-separated text formed by lines. col is in rune units, matching the
// rest of the editor.
func byteOffsetForCursor(lines []string, line, col int) int {
	if line < 0 {
		return 0
	}
	off := 0
	for i := 0; i < line && i < len(lines); i++ {
		off += len(lines[i]) + 1 // +1 for the newline separator
	}
	if line < len(lines) {
		runes := []rune(lines[line])
		if col > len(runes) {
			col = len(runes)
		}
		if col < 0 {
			col = 0
		}
		off += len(string(runes[:col]))
	}
	return off
}

// cursorForByteOffset is the inverse of byteOffsetForCursor: given a byte
// offset into the joined text, return the (line, col) it lands on, with col
// expressed in runes. Offsets past the end clamp to the last position.
func cursorForByteOffset(lines []string, offset int) (int, int) {
	if offset <= 0 || len(lines) == 0 {
		return 0, 0
	}
	for i, ln := range lines {
		if offset <= len(ln) {
			return i, len([]rune(ln[:offset]))
		}
		offset -= len(ln) + 1 // consume line + newline
		if offset < 0 {
			// Offset landed exactly on the newline; place at end of this line.
			return i, len([]rune(ln))
		}
	}
	last := len(lines) - 1
	return last, len([]rune(lines[last]))
}

// --- Multi-line Tab / Shift+Tab indent ---
//
// applyLineIndent is a pure helper that inserts (remove==false) or removes
// (remove==true) one indent unit at the start of each line in
// [startLine, endLine]. It returns the modified slice (input untouched) and
// a per-line byte delta (positive on insert, negative or zero on remove) so
// callers can adjust cursor and selection columns when the line they sit on
// got narrower or wider.
//
// Remove semantics follow Go's tab-or-4-space convention: a leading tab is
// stripped if present, otherwise up to 4 leading spaces. Lines with no
// leading whitespace are left unchanged (delta 0). Lines OUTSIDE the
// [startLine, endLine] range are copied verbatim with delta 0 — keeping the
// returned slice the same length as the input makes the caller's bookkeeping
// trivial.
func applyLineIndent(lines []string, startLine, endLine int, indent string, remove bool) ([]string, []int) {
	out := make([]string, len(lines))
	copy(out, lines)
	deltas := make([]int, len(lines))
	if startLine < 0 {
		startLine = 0
	}
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}
	if startLine > endLine {
		return out, deltas
	}
	for i := startLine; i <= endLine; i++ {
		ln := out[i]
		if remove {
			// Strip one tab, or up to 4 leading spaces.
			if strings.HasPrefix(ln, "\t") {
				out[i] = ln[1:]
				deltas[i] = -1
				continue
			}
			n := 0
			for n < 4 && n < len(ln) && ln[n] == ' ' {
				n++
			}
			if n > 0 {
				out[i] = ln[n:]
				deltas[i] = -n
			}
			// else: no leading whitespace — leave untouched, delta 0
		} else {
			out[i] = indent + ln
			deltas[i] = len(indent)
		}
	}
	return out, deltas
}

// IndentSelection inserts one indent unit at the start of every line spanned
// by the active selection. With no selection it is a no-op (single-caret Tab
// goes through OnTextInput("\t") in OnKeyDown).
func (this *CodeEditor) IndentSelection() {
	if !this.hasSelection {
		return
	}
	sl, _, el, _ := this.selectionRange()
	newLines, deltas := applyLineIndent(this.lines, sl, el, "\t", false)
	this.lines = newLines
	this.shiftSelectionAfterIndent(sl, el, deltas)
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
}

// DedentSelection removes one indent unit (one tab or up to 4 spaces) from
// the start of every line spanned by the active selection. Lines with no
// leading whitespace are skipped.
func (this *CodeEditor) DedentSelection() {
	if !this.hasSelection {
		return
	}
	sl, _, el, _ := this.selectionRange()
	newLines, deltas := applyLineIndent(this.lines, sl, el, "\t", true)
	this.lines = newLines
	this.shiftSelectionAfterIndent(sl, el, deltas)
	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
}

// shiftSelectionAfterIndent re-anchors the selection + cursor after an
// indent/dedent so the selection still spans the SAME logical lines, with
// column offsets adjusted by the per-line delta. Columns that would go
// negative (over-aggressive dedent of a leading-whitespace cursor) are
// clamped to 0; columns past the new line length are clamped to it.
func (this *CodeEditor) shiftSelectionAfterIndent(sl, el int, deltas []int) {
	shift := func(line, col int) int {
		if line < 0 || line >= len(deltas) {
			return col
		}
		col += deltas[line]
		if col < 0 {
			col = 0
		}
		if line < len(this.lines) {
			runes := len([]rune(this.lines[line]))
			if col > runes {
				col = runes
			}
		}
		return col
	}
	this.selStartCol = shift(this.selStartLine, this.selStartCol)
	this.selEndCol = shift(this.selEndLine, this.selEndCol)
	this.cursorCol = shift(this.cursorLine, this.cursorCol)
	// Keep selection range coverage in [sl, el] even if both ends collapsed
	// to the same column — users expect the selection to persist.
	this.hasSelection = !(this.selStartLine == this.selEndLine && this.selStartCol == this.selEndCol) || sl != el
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
	// testLines maps each test-func declaration line (0-based) to its name so the
	// render loop can draw a run ▶ marker beside it. It is nil (no allocation)
	// unless the edited file is a *_test.go, keeping the common non-test file case
	// free of work in the draw hot path.
	testLines := this.testFuncLines()
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

	// Blame (annotate) column: dim, right-aligned annotations pinned to the text
	// area's right edge. Width + rune budget are computed once per frame here so
	// the loop only does a map lookup + truncate. blameW is 0 when the view is
	// off, so this branch never shifts gutterW / textOffX (blame off is a no-op).
	blameW := blameColumnWidth(this.blameVisible)
	var blameMaxChars int
	if blameW > 0 {
		if cw := this.measureText("0"); cw > 0 { // Menlo is monospaced
			blameMaxChars = int(blameW / cw)
		}
	}

	// Clip the main editor area to avoid drawing into minimap/status bar
	g.Save()
	g.Rectangle(0, topOff, editorRight, editorBottom-topOff)
	g.Clip()

	for row := startRow; row < startRow+visibleLines && row < len(vis); row++ {
		i := vis[row]
		y := float64(row)*lh - this.scrollY + topOff

		// Error line background tint (draw before current line highlight)
		if _, hasErr := this.errorLines[i]; hasErr {
			g.SetBrush1(paint.Color{R: 130, G: 50, B: 50, A: 60})
			g.Rectangle(this.gutterW, y, editorRight-this.gutterW, lh)
			g.Fill()
		}

		// Current line highlight
		if i == this.cursorLine && !this.hasSelection {
			g.SetBrush1(paint.Color{R: 44, G: 44, B: 54, A: 255})
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
				g.SetBrush1(paint.Color{R: 70, G: 110, B: 190, A: 95})
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

		// Run-test ▶ marker: a small green play triangle at the gutter's LEFT edge
		// for every top-level Go test function (Test/Benchmark/Fuzz/Example) when
		// the file is a *_test.go. Slot: x in [testRunGutterX, testRunGutterX+
		// testRunGutterSize] (~12px, the left "action" column), vertically centred.
		// It never overlaps the right-aligned line number, the fold triangle, or the
		// diff marker (all pinned to the gutter's inner/right edge); it shares the
		// left column with the breakpoint dot, which is atypical on a func signature
		// line. The click (OnLeftDown) runs the test and consumes the event. Only
		// visible lines draw — this is inside the viewport-bounded, clipped loop.
		if _, isTest := testLines[i]; isTest {
			g.SetBrush1(paint.Color{R: 90, G: 200, B: 100, A: 235})
			half := testRunGutterSize / 2
			tip := testRunGutterX + testRunGutterSize*0.8
			g.MoveTo(testRunGutterX, gutterCenterY-half)
			g.LineTo(tip, gutterCenterY)
			g.LineTo(testRunGutterX, gutterCenterY+half)
			g.LineTo(testRunGutterX, gutterCenterY-half)
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

		// Coverage stripe: a thin vertical bar in the gutter between the git
		// stripe (x=1..4) and the breakpoint dot (centred at x=10). Green for
		// covered, red for uncovered; absent entries skip the stripe so neutral
		// lines stay clean. Alpha kept low so it doesn't fight the line number.
		if this.coverage != nil {
			if covered, has := this.coverage[i]; has {
				if covered {
					g.SetBrush1(paint.Color{R: 80, G: 200, B: 120, A: 120})
				} else {
					g.SetBrush1(paint.Color{R: 220, G: 60, B: 60, A: 120})
				}
				g.Rectangle(5, y, 2, lh)
				g.Fill()
			}
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

		// Diff gutter marker: a host-pushed VCS bar at the gutter's INNER edge
		// (between the line number and the text), so it composes with—rather than
		// fights—the git stripe on the left edge. Added/Modified draw a thin
		// full-height bar; Removed draws a small downward triangle at the line's
		// top-left, signalling line(s) deleted after this one (Qt Creator / VS
		// Code style). Absent lines draw nothing. See diffMarkerColor.
		if len(this.diffMarkers) > 0 {
			if kind, ok := this.diffMarkers[i]; ok {
				if col, draw := diffMarkerColor(kind); draw {
					g.SetBrush1(col)
					if kind == DiffMarkerRemoved {
						// Downward triangle (~5px) at the line's top edge.
						tx := this.gutterW - 6
						ty := y + 1
						g.MoveTo(tx, ty)
						g.LineTo(tx+5, ty)
						g.LineTo(tx+2.5, ty+5)
						g.LineTo(tx, ty)
						g.Fill()
					} else {
						// Full-line-height vertical bar at the inner edge.
						g.Rectangle(this.gutterW-3, y, 3, lh)
						g.Fill()
					}
				}
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
		inBlock = this.drawHighlightedLine(g, i, lineText, textOffX, y+fe.Ascent, inBlock)

		// Blame annotation: dim + right-aligned in the reserved column at the text
		// area's right edge. Drawn only for visible lines carrying an annotation.
		// blameW is 0 when the view is off, so this whole block is skipped and no
		// pixels move — layout is identical to a build without blame.
		if blameW > 0 && blameMaxChars > 0 {
			if ann, ok := this.blame[i]; ok && ann != "" {
				shown := truncateBlame(ann, blameMaxChars)
				bx := editorRight - 6 - this.measureText(shown)
				g.SetFont(this.font)
				g.SetBrush1(paint.Color{R: 120, G: 120, B: 140, A: 150})
				g.DrawText1(bx, y+fe.Ascent, shown)
			}
		}

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
			g.SetBrush1(paint.Color{R: 212, G: 212, B: 222, A: 255})
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
			g.SetBrush1(paint.Color{R: 212, G: 212, B: 222, A: 160})
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

// drawFindBar draws the find (and, when shown, replace) bar at the top of the
// editor. The bar is one row tall for find-only and two rows when the replace
// input is visible; findBarHeight holds the current total.
func (this *CodeEditor) drawFindBar(g paint.Painter, w float64) {
	rowH := float64(codeEditorFindRowHeight)
	barH := this.findBarHeight
	// Background
	g.SetBrush1(paint.Color{R: 50, G: 50, B: 60, A: 255})
	g.Rectangle(0, 0, w, barH)
	g.Fill()
	// Bottom border
	g.SetPen1(paint.Color{R: 70, G: 70, B: 85, A: 255}, 1)
	g.Line(0, barH, w, barH)
	g.Stroke()

	fe := this.font.FontExtents()
	g.SetFont(this.font)

	// Find row (row 0): focused unless the replace input holds focus.
	findFocused := !(this.replaceVisible && this.replaceFocused)
	this.drawFindInputRow(g, 0, rowH, fe, "Find:", this.findText, this.findCursor, findFocused)

	// Match count, right of the find input.
	countStr := fmt.Sprintf("%d of %d", 0, 0)
	if len(this.findMatches) > 0 {
		countStr = fmt.Sprintf("%d of %d", this.findCurrentIdx+1, len(this.findMatches))
	}
	g.SetBrush1(paint.Color{R: 140, G: 140, B: 155, A: 255})
	g.DrawText1(findBarInputX+findBarInputW+10, rowH/2+fe.Ascent/2-1, countStr)

	// Replace row (row 1): only when toggled on (Cmd+H).
	if this.replaceVisible {
		this.drawFindInputRow(g, rowH, rowH, fe, "Repl:", this.replaceText, this.replaceCursor, this.replaceFocused)
		btnX := findBarInputX + findBarInputW + 10
		this.drawFindButton(g, btnX, rowH, rowH, fe, "Replace")
		this.drawFindButton(g, btnX+findBarBtnW+6, rowH, rowH, fe, "All")
	}
}

// drawFindInputRow draws one labeled input row of the find/replace bar at
// vertical offset rowY. The caret is drawn only when the row is focused, and a
// focused row gets a brighter border.
func (this *CodeEditor) drawFindInputRow(g paint.Painter, rowY, rowH float64, fe *paint.FontExtents, label, text string, cursor int, focused bool) {
	baseline := rowY + rowH/2 + fe.Ascent/2 - 1
	// Label
	g.SetBrush1(paint.Color{R: 180, G: 180, B: 190, A: 255})
	g.DrawText1(10, baseline, label)
	// Input background
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(findBarInputX, rowY+4, findBarInputW, rowH-8)
	g.Fill()
	if focused {
		g.SetPen1(paint.Color{R: 120, G: 140, B: 200, A: 255}, 1)
	} else {
		g.SetPen1(paint.Color{R: 80, G: 80, B: 100, A: 255}, 1)
	}
	g.Rectangle(findBarInputX, rowY+4, findBarInputW, rowH-8)
	g.Stroke()
	// Text
	if text != "" {
		g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
		g.DrawText1(findBarInputX+4, baseline, text)
	}
	// Caret (focused row only)
	if focused {
		tr := []rune(text)
		prefix := ""
		if cursor >= 0 && cursor <= len(tr) {
			prefix = string(tr[:cursor])
		}
		cx := findBarInputX + 4 + this.measureText(prefix)
		g.SetBrush1(paint.Color{R: 200, G: 200, B: 230, A: 255})
		g.Rectangle(cx, rowY+6, 1, rowH-12)
		g.Fill()
	}
}

// drawFindButton draws a small clickable button (Replace / All) in the replace row.
func (this *CodeEditor) drawFindButton(g paint.Painter, x, rowY, rowH float64, fe *paint.FontExtents, label string) {
	g.SetBrush1(paint.Color{R: 70, G: 80, B: 110, A: 255})
	g.Rectangle(x, rowY+5, findBarBtnW, rowH-10)
	g.Fill()
	g.SetPen1(paint.Color{R: 100, G: 115, B: 150, A: 255}, 1)
	g.Rectangle(x, rowY+5, findBarBtnW, rowH-10)
	g.Stroke()
	g.SetBrush1(paint.Color{R: 215, G: 220, B: 235, A: 255})
	tw := this.measureText(label)
	g.DrawText1(x+(findBarBtnW-tw)/2, rowY+rowH/2+fe.Ascent/2-1, label)
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
// idx is the line's index in this.lines and feeds the tokenization cache.
func (this *CodeEditor) drawHighlightedLine(g paint.Painter, idx int, line string, x, y float64, inBlock bool) bool {
	tokens, newInBlock := this.tokenizeLineCached(idx, line, inBlock)
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

		tokens, newBlock := this.tokenizeLineCached(i, line, inBlock)
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
//
// Pathological-input guard: when len(line) >= maxHighlightLineLength the
// tokenizer allocates a single fallback token spanning the whole line. If the
// line started inside a block comment it stays a comment token (and inBlock
// stays true), otherwise it renders as plain text. This trades coloring on
// truly long lines for predictable cost — the rune-decode + per-rune scan is
// the expensive part of the tokenizer.
func tokenizeLine(line string, inBlock bool) ([]token, bool) {
	if len(line) >= maxHighlightLineLength {
		if inBlock {
			return []token{{text: line, typ: tokComment}}, true
		}
		return []token{{text: line, typ: tokNormal}}, false
	}
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

// tokenLineHash returns a fnv64a hash that mixes the line bytes with the
// incoming inBlock flag. The flag goes in as a trailing sentinel byte so a
// line that flips between in-block and out-of-block context produces a
// distinct key, even though its bytes are identical.
func tokenLineHash(line string, inBlock bool) uint64 {
	h := fnv.New64a()
	// Writing to fnv64a is documented never to fail; the error is unused.
	_, _ = h.Write([]byte(line))
	if inBlock {
		_, _ = h.Write([]byte{1})
	} else {
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64()
}

// tokenizeLineCached is the Draw-path wrapper around tokenizeLine. It
// memoizes the (tokens, nextBlock) pair under (lineIdx, fnv64a(line, inBlock));
// a hit returns the cached slice directly, a miss runs the underlying
// tokenizer and stores the result.
//
// The cache is single-threaded — Draw and every mutation handler run on the
// GLFW main thread — so no locking is needed.
//
// Long lines past maxHighlightLineLength fall through to tokenizeLine's
// single-token fallback and are cached just like any other line: the entry
// is small (one token), and a hash hit still skips the per-rune scan.
func (this *CodeEditor) tokenizeLineCached(idx int, line string, inBlock bool) ([]token, bool) {
	h := tokenLineHash(line, inBlock)
	if e, ok := this.tokenCache[idx]; ok && e.hash == h {
		return e.tokens, e.nextBlock
	}
	tokens, next := tokenizeLine(line, inBlock)
	if this.tokenCache == nil {
		this.tokenCache = make(map[int]tokenCacheEntry)
	}
	this.tokenCache[idx] = tokenCacheEntry{hash: h, tokens: tokens, nextBlock: next}
	return tokens, next
}

// clearTokenCache drops every cached entry. Called from SetText and from
// rebuildText when the line count changes, since line-index keys are no
// longer trustworthy after an insert/delete shift.
func (this *CodeEditor) clearTokenCache() {
	this.tokenCache = nil
	this.tokenCacheLineCt = 0
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

	// If clicking in find bar area, absorb the click. A hit on a replace button
	// fires the action; otherwise a click just moves focus between the find and
	// replace inputs. Either way the click never reaches the text area.
	if this.findActive && y < this.findBarHeight {
		switch this.replaceButtonHit(x, y) {
		case 1:
			this.replaceCurrent()
		case 2:
			this.replaceAll()
		default:
			if this.replaceVisible {
				this.replaceFocused = y >= float64(codeEditorFindRowHeight)
				this.Self().Update()
			}
		}
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
			// Run-test ▶ marker takes priority over the fold zone and the
			// breakpoint fall-through: a click on the left-edge marker of a test
			// function fires the host run callback and consumes the click (no
			// breakpoint toggle, no caret move).
			if name, ok := this.testRunMarkerAt(x, line); ok {
				if this.cbTestRun != nil {
					this.cbTestRun(name)
				}
				return
			}
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

	// LSP hover signal: report to the host when the mouse settles over a NEW
	// identifier in the text area. We don't fetch or display anything here —
	// the host runs the async Hover RPC and shows the tooltip. Fire once per
	// identifier (gated on a change of word-start (line, col)) so the host gets
	// one event per word, not one per pixel. Suppressed while the completion
	// popup is up to avoid fighting it.
	this.maybeFireHover(x, y)
}

// maybeFireHover detects an identifier under the mouse in the text area and, if
// it differs from the last identifier reported, fires cbHoverRequested with the
// (line, word-start col) and a global anchor. Split out of OnMouseMove so the
// position-change gating is unit-testable without a GLFW context. No-op when no
// hover callback is registered or the completion popup is visible.
func (this *CodeEditor) maybeFireHover(x, y float64) {
	if this.cbHoverRequested == nil {
		return
	}
	if this.completion != nil && this.completion.visible {
		return
	}
	line, col, ok := this.hoverIdentAt(x, y)
	if !ok {
		// Off any identifier: reset so re-entering the same word re-fires.
		this.hoverReqLine = -1
		this.hoverReqCol = -1
		return
	}
	if line == this.hoverReqLine && col == this.hoverReqCol {
		return // still the same identifier — host already got this event
	}
	this.hoverReqLine = line
	this.hoverReqCol = col
	gx, gy := this.MapToGlobal(x, y)
	this.cbHoverRequested(line, col, gx, gy)
}

// hoverIdentAt maps mouse (x, y) to the identifier under it, returning the
// line and the word-START column plus ok=true when the position lands on a word
// in the TEXT area (not the gutter). Returns ok=false in the gutter, off a
// word, or out of range. The word-start col gives a stable per-identifier key
// so hover fires once per word rather than per column.
func (this *CodeEditor) hoverIdentAt(x, y float64) (line, col int, ok bool) {
	if x < this.gutterW { // gutter (line numbers / breakpoints), never an identifier
		return 0, 0, false
	}
	line, c := this.posFromXY(x, y)
	if line < 0 || line >= len(this.lines) {
		return 0, 0, false
	}
	// Confirm the resolved column sits on an identifier rune. wordBoundsAt
	// returns a single-rune span (col, col+1) for punctuation/whitespace, so a
	// "start != end" check is not enough — gate on the rune class directly.
	runes := []rune(this.lines[line])
	if c >= len(runes) {
		return 0, 0, false
	}
	ch := runes[c]
	if !(unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_') {
		return 0, 0, false
	}
	start, _ := this.wordBoundsAt(line, c)
	return line, start, true
}

func (this *CodeEditor) OnLeftUp(x, y float64) {
	this.mouseDown = false
}

// editorContextMenuItem is one row of the right-click menu produced by
// contextMenuItems. Splitting the menu out of OnRightDown keeps the
// per-entry wiring (label, enablement, action) directly testable:
// tests can call contextMenuItems and invoke Action without standing
// up a Popup, a Window, or a GLFW context.
type editorContextMenuItem struct {
	Label     string
	Enabled   bool
	Separator bool   // true when this entry is just a visual separator
	Action    func() // nil when Separator or Enabled is false
}

// contextMenuItems builds the right-click menu for the editor at the
// current cursor / selection state. The Cut and Copy entries are
// disabled when there is no selection (matches Qt Creator / VS Code).
// Paste is disabled when the clipboard carries no text. Rename Symbol
// is disabled when the cursor sits on whitespace / punctuation (empty
// word). Go to Definition and Find References require a word at
// cursor, mirroring the underlying methods which silently no-op on an
// empty word — disabling here just makes that visible to the user.
func (this *CodeEditor) contextMenuItems(hasSel, hasClip bool, word string) []editorContextMenuItem {
	hasWord := word != ""
	return []editorContextMenuItem{
		{
			Label:   "剪切",
			Enabled: hasSel,
			Action:  func() { this.editorCut() },
		},
		{
			Label:   "复制",
			Enabled: hasSel,
			Action:  func() { this.editorCopy() },
		},
		{
			Label:   "粘贴",
			Enabled: hasClip,
			Action:  func() { this.pasteClipboard() },
		},
		{
			Label:   "全选",
			Enabled: true,
			Action:  func() { this.editorSelectAll() },
		},
		{Separator: true},
		{
			Label:   "重命名符号",
			Enabled: hasWord,
			Action:  func() { this.promptRenameSymbol(word) },
		},
		{
			Label:   "跳转定义",
			Enabled: hasWord,
			Action:  func() { this.GoToDefinitionAtCursor() },
		},
		{
			Label:   "查找引用",
			Enabled: hasWord,
			Action:  func() { this.HighlightReferencesAtCursor() },
		},
	}
}

// clipboardHasText reports whether the framework clipboard currently
// holds plain text. Used by OnRightDown to decide whether to enable
// the Paste entry; tests drive contextMenuItems directly with a bool
// so this never runs without a real GLFW window.
func (this *CodeEditor) clipboardHasText() bool {
	d, err := Clipboard.Data("text/plain")
	if err != nil {
		return false
	}
	s, ok := d.(string)
	return ok && s != ""
}

// promptRenameSymbol pops an input dialog seeded with the identifier
// under the cursor and, on submit, delegates to RenameSymbolAtCursor.
// Errors are swallowed silently — the editor has no toast surface of
// its own, and a failed rename leaves the buffer untouched anyway.
func (this *CodeEditor) promptRenameSymbol(word string) {
	newName, ok := ShowInputBox(this.Self(), nil, "重命名符号", "新名称:", word)
	if !ok {
		return
	}
	_, _, _ = this.RenameSymbolAtCursor(newName)
}

// repositionCaretForRightClick mirrors Qt Creator's right-click caret
// rule and is the GL-free part of OnRightDown: clicks inside the text
// area move the caret to the click point, EXCEPT when the click lands
// inside an active selection (so Cut/Copy keep operating on it). Clicks
// in the gutter / minimap / find-bar / goto-bar / status bar leave the
// caret alone. Extracted from OnRightDown so tests can exercise the
// caret-move logic without invoking ShowContextMenu (which calls into
// cgo paths that are unsafe in a headless test process).
func (this *CodeEditor) repositionCaretForRightClick(x, y float64) {
	w, h := this.Size()
	mmW := this.minimapWidth()
	inGutter := x < this.gutterW && y >= this.topOffset()
	inMinimap := this.showMinimap && mmW > 0 && x > w-mmW
	inStatus := y > h-this.statusBarHeight
	inFindBar := this.findActive && y < this.findBarHeight
	gotoBarTop := 0.0
	if this.findActive {
		gotoBarTop = this.findBarHeight
	}
	inGotoBar := this.gotoLineActive && y >= gotoBarTop && y < gotoBarTop+this.findBarHeight
	if inGutter || inMinimap || inStatus || inFindBar || inGotoBar {
		return
	}
	line, col := this.posFromXY(x, y)
	if !this.hasSelection || !this.posInSelection(line, col) {
		this.clearSelection()
		this.cursorLine = line
		this.cursorCol = col
		this.clampCursor()
	}
	this.Self().Update()
}

// OnRightDown opens the editor's context menu at the click point.
// Following Qt Creator, the caret is first moved to the click position
// so the subsequent Rename / Go to Definition / Find References act on
// the word the user actually right-clicked, not wherever the caret
// happened to be.
func (this *CodeEditor) OnRightDown(x, y float64) {
	this.SetFocus()
	this.repositionCaretForRightClick(x, y)

	items := this.contextMenuItems(this.hasSelection, this.clipboardHasText(), this.wordAtCursor())
	ShowContextMenu(this.Self(), x, y, func(m *Menu) {
		for _, it := range items {
			if it.Separator {
				m.AddSeparator()
				continue
			}
			btn := m.AddButton1(it.Label, nil)
			if !it.Enabled {
				btn.SetEnabled(false)
				continue
			}
			action := it.Action
			btn.Action().BindFunc0(func() { action() })
		}
	})
}

// posInSelection reports whether (line, col) lies inside the current
// selection range. Used by OnRightDown to decide whether the right-
// click should keep or collapse the existing selection.
func (this *CodeEditor) posInSelection(line, col int) bool {
	if !this.hasSelection {
		return false
	}
	sl, sc, el, ec := this.selectionRange()
	if line < sl || line > el {
		return false
	}
	if line == sl && col < sc {
		return false
	}
	if line == el && col > ec {
		return false
	}
	return true
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

	// Route to find bar if active. When the replace input has focus, typing
	// edits replaceText and leaves the match set alone.
	if this.findActive {
		if this.replaceVisible && this.replaceFocused {
			replRunes := []rune(this.replaceText)
			insertRunes := []rune(s)
			newRunes := make([]rune, 0, len(replRunes)+len(insertRunes))
			newRunes = append(newRunes, replRunes[:this.replaceCursor]...)
			newRunes = append(newRunes, insertRunes...)
			newRunes = append(newRunes, replRunes[this.replaceCursor:]...)
			this.replaceText = string(newRunes)
			this.replaceCursor += len(insertRunes)
			this.Self().Update()
			return
		}
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
		// Auto-close: typing an opener with an active selection wraps the
		// selection in the matching pair instead of replacing it.
		if open, closer, ok := autoCloseOpenerFor(s); ok {
			this.wrapSelectionWith(open, closer)
			this.ensureCursorVisible()
			this.Self().Update()
			return
		}
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

	// Auto-close: type over an existing closing bracket/quote already under the
	// caret rather than inserting a duplicate. No buffer mutation, so no undo.
	if isAutoCloseCloser(s) {
		curRunes := []rune(this.lines[this.cursorLine])
		if this.cursorCol < len(curRunes) && string(curRunes[this.cursorCol]) == s {
			this.cursorCol++
			this.ensureCursorVisible()
			this.Self().Update()
			if s == ")" {
				HideToolTip()
			}
			return
		}
	}

	// Auto-close: typing an opener inserts the matching closer and parks the
	// caret between the pair. Routed through rebuildText like every edit.
	if open, closer, ok := autoCloseOpenerFor(s); ok {
		this.pushUndo(editAction{kind: 0, line: this.cursorLine, col: this.cursorCol, text: string(open) + string(closer)})
		pairLine := []rune(this.lines[this.cursorLine])
		newPair := make([]rune, 0, len(pairLine)+2)
		newPair = append(newPair, pairLine[:this.cursorCol]...)
		newPair = append(newPair, open, closer)
		newPair = append(newPair, pairLine[this.cursorCol:]...)
		this.lines[this.cursorLine] = string(newPair)
		this.cursorCol++ // caret between the pair
		this.rebuildText()
		this.ensureCursorVisible()
		this.Self().Update()
		this.tryTriggerCompletion(s)
		if isSignatureTrigger(s) && this.cbSignatureRequested != nil {
			this.cbSignatureRequested(this.cursorLine, this.cursorCol)
		}
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

	// LSP signature-help signal: on a "(" or "," trigger, report the cursor
	// position (already advanced past the insert) so the host runs the async
	// SignatureHelp RPC and shows it. Typing ")" closes the function call, so we
	// dismiss any shown help here; Esc dismissal is left to the host. The editor
	// never fetches signature text itself.
	if isSignatureTrigger(s) {
		if this.cbSignatureRequested != nil {
			this.cbSignatureRequested(this.cursorLine, this.cursorCol)
		}
	} else if s == ")" {
		HideToolTip()
	}
}

// isSignatureTrigger reports whether typing s should request LSP signature help.
// gopls treats "(" (call start) and "," (next argument) as signature-help
// triggers; everything else (including ")" which CLOSES the call) is not.
func isSignatureTrigger(s string) bool {
	return s == "(" || s == ","
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
		this.showCompletion("")
		return
	}
	if unicode.IsLetter(lastRune) || lastRune == '_' {
		// Build prefix from current word
		prefix := this.currentWordPrefix()
		if len(prefix) >= 1 {
			this.showCompletion(prefix)
		}
	} else {
		this.completion.Dismiss()
	}
}

// showCompletion opens the popup for prefix, then merges any externally
// injected candidates into it. It is the single internal entry point for
// (re)building the popup so external items survive every keystroke re-trigger.
func (this *CodeEditor) showCompletion(prefix string) {
	if this.completion == nil {
		this.completion = NewCompletionPopup(this)
	}
	this.completion.Show(prefix, this)
	this.mergeExternalIntoPopup()
}

// mergeExternalIntoPopup folds this.externalCompletions into the popup's
// candidate set (external wins on label collision) and re-ranks against the
// current prefix. No-op when there are no external items so the built-in
// behaviour is unchanged.
func (this *CodeEditor) mergeExternalIntoPopup() {
	if this.completion == nil || len(this.externalCompletions) == 0 {
		return
	}
	this.completion.items = mergeCompletions(this.completion.items, this.externalCompletions)
	this.completion.filter()
	if len(this.completion.filtered) == 0 {
		this.completion.visible = false
		return
	}
	this.completion.visible = true
	if this.completion.selectedIdx >= len(this.completion.filtered) {
		this.completion.selectedIdx = 0
	}
	this.completion.scrollY = 0
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
		// Ctrl+H toggles the replace input row.
		if key == 'H' && IsKeyDown(KeyCtrl) {
			this.toggleReplaceRow()
			return
		}
		// When the replace input has focus, edits target replaceText and Enter
		// runs Replace (current match); Shift+Enter runs Replace All.
		if this.replaceVisible && this.replaceFocused {
			switch key {
			case KeyEsc:
				this.closeFindBar()
				return
			case KeyEnter, KeyF3:
				if IsKeyDown(KeyShift) {
					this.replaceAll()
				} else {
					this.replaceCurrent()
				}
				return
			case KeyTab:
				this.replaceFocused = false
				this.Self().Update()
				return
			case KeyBackSpace:
				if this.replaceCursor > 0 {
					rr := []rune(this.replaceText)
					rr = append(rr[:this.replaceCursor-1], rr[this.replaceCursor:]...)
					this.replaceText = string(rr)
					this.replaceCursor--
					this.Self().Update()
				}
				return
			case KeyLeft:
				if this.replaceCursor > 0 {
					this.replaceCursor--
					this.Self().Update()
				}
				return
			case KeyRight:
				if this.replaceCursor < len([]rune(this.replaceText)) {
					this.replaceCursor++
					this.Self().Update()
				}
				return
			case KeyHome:
				this.replaceCursor = 0
				this.Self().Update()
				return
			case KeyEnd:
				this.replaceCursor = len([]rune(this.replaceText))
				this.Self().Update()
				return
			}
			return // consume all other keys while the replace input has focus
		}
		switch key {
		case KeyEsc:
			this.closeFindBar()
			return
		case KeyEnter, KeyF3:
			if IsKeyDown(KeyShift) {
				this.findPrev()
			} else {
				this.findNext()
			}
			return
		case KeyTab:
			if this.replaceVisible {
				this.replaceFocused = true
				this.Self().Update()
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
		prefix := this.currentWordPrefix()
		this.showCompletion(prefix)
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
				this.showCompletion(prefix)
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

		// Go-aware indent: copy current-line leading whitespace and add one
		// indent step when the cursor sits at the end of a line ending in '{'.
		// The editor's indent unit is a literal tab (KeyTab inserts "\t").
		indent := nextLineIndent(line, this.cursorCol, "\t")

		// Smart indent: reduce indent after '}'. Only fires when the cursor
		// sits immediately before a '}' (e.g. "if true {|}" pressing Enter
		// drops the closing brace back to the outer level).
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
		// Multi-line selection: Tab indents every spanned line, Shift+Tab
		// dedents. Snippet expansion only makes sense when there is a single
		// caret on one line, so it runs only in the no-selection branch below.
		if this.hasSelection && !ctrl {
			sl, _, el, _ := this.selectionRange()
			if sl != el {
				if shift {
					this.DedentSelection()
				} else {
					this.IndentSelection()
				}
				return
			}
		}
		// Try snippet expansion before inserting tab
		if !shift && !ctrl && !this.hasSelection {
			// New-style SnippetSet path first: Qt Creator / VS Code tab
			// triggers (iferr, forrange, Test, ...). If it matches, it wins.
			if this.tryExpandSnippetAtCursor() {
				return
			}
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
		// Multi-cursor: step every caret one rune to the left (with line wrap).
		// Selection state is dropped because per-cursor selections aren't tracked.
		if len(this.additionalCursors) > 0 && !shift {
			this.clearSelection()
			this.moveAllCursorsBy(0, -1)
			this.ensureCursorVisible()
			this.Self().Update()
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
		if len(this.additionalCursors) > 0 && !shift {
			this.clearSelection()
			this.moveAllCursorsBy(0, +1)
			this.ensureCursorVisible()
			this.Self().Update()
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
		// Plain Up with secondary cursors: move every caret up one line.
		if len(this.additionalCursors) > 0 && !shift {
			this.clearSelection()
			this.moveAllCursorsBy(-1, 0)
			this.ensureCursorVisible()
			this.Self().Update()
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
		// Plain Down with secondary cursors: move every caret down one line.
		if len(this.additionalCursors) > 0 && !shift {
			this.clearSelection()
			this.moveAllCursorsBy(+1, 0)
			this.ensureCursorVisible()
			this.Self().Update()
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
		if len(this.additionalCursors) > 0 && !shift {
			this.clearSelection()
			this.moveAllCursorsToLineBound(false)
			this.Self().Update()
			return
		}
		beginSelIfShift()
		this.cursorCol = 0
		endSelIfShift()
		this.Self().Update()

	case KeyEnd:
		if len(this.additionalCursors) > 0 && !shift {
			this.clearSelection()
			this.moveAllCursorsToLineBound(true)
			this.Self().Update()
			return
		}
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
			if this.findActive {
				this.closeFindBar()
			} else {
				this.findActive = true
				this.replaceVisible = false
				this.replaceFocused = false
				this.findBarHeight = codeEditorFindRowHeight
				// If text is selected, use it as find text
				if this.hasSelection {
					this.findText = this.SelectedText()
					this.findCursor = len([]rune(this.findText))
					this.findUpdateMatches()
				}
				this.Self().Update()
			}
		}

	case 'H':
		if ctrl && !this.findActive {
			// Ctrl+H: open the find bar with the replace row shown and focused.
			this.findActive = true
			this.findBarHeight = codeEditorFindRowHeight
			if this.hasSelection {
				this.findText = this.SelectedText()
				this.findCursor = len([]rune(this.findText))
				this.findUpdateMatches()
			}
			this.replaceVisible = false // toggleReplaceRow flips it on + focuses it
			this.toggleReplaceRow()
		}

	case 'D':
		if (ctrl || isActionModifier()) && shift {
			// Cmd+Shift+D (macOS) / Ctrl+Shift+D: duplicate the current line or
			// every line of the selection below itself.
			this.DuplicateLines()
		} else if ctrl {
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

	case 'J':
		if ctrl || isActionModifier() {
			// Cmd+J (macOS) / Ctrl+J: join the current line (or the selected
			// lines) with the next, collapsing the break to a single space.
			this.JoinLines()
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

// --- Run-Test Gutter Marker ---
//
// A small green ▶ (play) triangle is drawn in the gutter beside every top-level
// Go test function when the edited file is a *_test.go. Clicking it fires
// cbTestRun(name) so the host (silkide) can run `go test -run ^Name$`.
//
// Slot: the LEFT edge of the gutter, x in [testRunGutterX, testRunGutterX+
// testRunGutterSize] (~12px), vertically centred on the line. The gutter's
// inner/right edge is fully committed on a func line — right-aligned line number
// (ends at gutterW-8), fold triangle (gutterW-12 hit strip), and diff marker
// (gutterW-6..gutterW) — so the marker lives on the left. That left column also
// hosts the breakpoint dot (centre x=10), but a breakpoint on a bare test
// signature line is atypical, and the click handler gives the run marker
// priority so the two never fight for one click (F9 still toggles a breakpoint
// on the cursor line).

const (
	testRunGutterX    = 1.0  // left inset of the run ▶ marker
	testRunGutterSize = 12.0 // marker box size (width == height, ~12px)
)

// isGoTestFuncName reports whether name is a Go test-style function name: one of
// the prefixes Test / Benchmark / Fuzz / Example followed by either nothing or a
// non-lowercase rune. This mirrors cmd/go's isTest rule exactly, so bare "Test"
// counts, "TestFoo" / "Test_foo" / "Test1" count, but "Testfoo" and "Testing"
// do NOT (the rune right after the prefix is a lowercase letter). Pure (no
// receiver) so it is unit-testable in isolation.
func isGoTestFuncName(name string) bool {
	for _, prefix := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
		if isTestFuncWithPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// isTestFuncWithPrefix implements cmd/go's isTest rule for a single prefix:
// name must start with prefix and, if longer, the first trailing rune must not
// be a lowercase letter.
func isTestFuncWithPrefix(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return !unicode.IsLower(r)
}

// testFuncLines returns a map of 0-based line index -> test-func name for every
// top-level test function in the buffer, but ONLY when the edited file is a
// *_test.go. For any other file it returns nil (no allocation), keeping the draw
// hot path free of work in the common case. ParseSymbols is the same O(lines)
// scan the breadcrumb already runs every frame, so recomputing here is cheap.
func (this *CodeEditor) testFuncLines() map[int]string {
	if !strings.HasSuffix(this.FilePath(), "_test.go") {
		return nil
	}
	var out map[int]string
	for _, s := range this.ParseSymbols() {
		if s.Kind == SymFunc && isGoTestFuncName(s.Name) {
			if out == nil {
				out = make(map[int]string)
			}
			out[s.Line] = s.Name
		}
	}
	return out
}

// testRunMarkerHitX reports whether a gutter x-coordinate falls inside the run ▶
// marker's horizontal slot. Pure / layout-independent so the hit-test can be
// unit-tested without font metrics.
func testRunMarkerHitX(x float64) bool {
	return x >= 0 && x <= testRunGutterX+testRunGutterSize
}

// testRunMarkerAt reports the test-func name to run when the gutter is clicked at
// horizontal position x on editor line `line` (the caller maps the click Y to a
// line via posFromXY). ok is false when x is outside the marker slot or `line`
// is not a test-func line, so any other gutter click falls through to the
// existing fold / breakpoint handling.
func (this *CodeEditor) testRunMarkerAt(x float64, line int) (string, bool) {
	if !testRunMarkerHitX(x) {
		return "", false
	}
	name, ok := this.testFuncLines()[line]
	return name, ok
}

// SigTestRunRequested registers the callback fired when the user clicks the
// run-test ▶ gutter marker beside a Go test function. The argument is the
// function name (e.g. "TestFoo"); the host runs `go test -run ^Name$`. Mirrors
// the SigWidgetClicked / SigChanged idiom.
func (this *CodeEditor) SigTestRunRequested(fn func(name string)) {
	this.cbTestRun = fn
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

// --- Coverage Gutter ---
//
// Coverage is part of the editor's UI/state layer only; the host parses
// `go tool cover` output (or any other source) and pushes the resulting
// line -> covered map in via SetCoverage. The editor renders a thin
// green/red stripe in the gutter per entry. Lines absent from the map (or
// the whole map being nil) get no stripe, so neutral lines stay clean and
// don't fight the text.

// SetCoverage installs a coverage map. The argument is copied so the host is
// free to mutate its own copy afterwards. Passing nil clears coverage (same
// effect as ClearCoverage).
func (this *CodeEditor) SetCoverage(cov map[int]bool) {
	if cov == nil {
		this.coverage = nil
		this.Self().Update()
		return
	}
	c := make(map[int]bool, len(cov))
	for ln, covered := range cov {
		c[ln] = covered
	}
	this.coverage = c
	this.Self().Update()
}

// ClearCoverage drops any coverage data. The stripe becomes invisible.
func (this *CodeEditor) ClearCoverage() {
	this.coverage = nil
	this.Self().Update()
}

// HasCoverage reports whether a coverage map is currently installed.
func (this *CodeEditor) HasCoverage() bool {
	return this.coverage != nil
}

// LineCovered queries a single line. The second return value indicates whether
// the line has any coverage entry at all (so callers can distinguish "covered
// = false" from "no data").
func (this *CodeEditor) LineCovered(line int) (covered bool, has bool) {
	if this.coverage == nil {
		return false, false
	}
	covered, has = this.coverage[line]
	return covered, has
}

// --- Diff Gutter Markers ---
//
// VCS-style diff markers are the coloured bar in the left margin (VS Code / Qt
// Creator / GitHub) flagging which lines were Added, Modified, or Removed. The
// editor does NOT compute the diff: the host pushes a precomputed map via
// SetDiffMarkers and the editor renders it. Lines are keyed 0-based (same
// convention as breakpoints / bookmarks / coverage) and are NOT re-mapped when
// lines are inserted or deleted.

// DiffMarkerKind is the VCS diff state of a single line.
type DiffMarkerKind int

const (
	DiffMarkerNone     DiffMarkerKind = iota // no marker — draw nothing
	DiffMarkerAdded                          // green bar — new line
	DiffMarkerModified                       // blue bar — changed line
	DiffMarkerRemoved                        // red triangle — line(s) removed AFTER this line
)

// lineMarker pairs a (visible) line index with its diff kind. It is the unit
// returned by visibleDiffMarkers so the render path can iterate an in-range,
// flattened slice instead of probing the map per row.
type lineMarker struct {
	line int
	kind DiffMarkerKind
}

// diffMarkerColor maps a marker kind to its gutter colour and whether it should
// be drawn at all. DiffMarkerNone (and any unknown kind) returns draw=false so
// callers can skip cleanly. Colours match the editor's gutter palette: green
// add, blue modify, red remove — the same hues the git gutter already uses.
func diffMarkerColor(kind DiffMarkerKind) (paint.Color, bool) {
	switch kind {
	case DiffMarkerAdded:
		return paint.Color{R: 80, G: 200, B: 120, A: 230}, true
	case DiffMarkerModified:
		return paint.Color{R: 70, G: 140, B: 220, A: 230}, true
	case DiffMarkerRemoved:
		return paint.Color{R: 220, G: 60, B: 60, A: 230}, true
	default:
		return paint.Color{}, false
	}
}

// visibleDiffMarkers flattens the marker map to the entries whose line falls in
// the inclusive [firstLine, lastLine] viewport range, sorted by line. It is a
// pure helper (no GL, no receiver state) so the viewport-culling logic can be
// unit-tested directly. Entries with a non-drawable kind (DiffMarkerNone) are
// skipped. A nil/empty map yields nil.
func visibleDiffMarkers(markers map[int]DiffMarkerKind, firstLine, lastLine int) []lineMarker {
	if len(markers) == 0 {
		return nil
	}
	out := make([]lineMarker, 0, len(markers))
	for line, kind := range markers {
		if line < firstLine || line > lastLine {
			continue
		}
		if _, draw := diffMarkerColor(kind); !draw {
			continue
		}
		out = append(out, lineMarker{line: line, kind: kind})
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(a, b int) bool { return out[a].line < out[b].line })
	return out
}

// SetDiffMarkers replaces the whole marker set. The argument is copied so the
// host is free to mutate its own copy afterwards; entries with kind
// DiffMarkerNone are dropped so the set stays minimal. Passing nil clears the
// markers (same effect as ClearDiffMarkers). Triggers a repaint.
func (this *CodeEditor) SetDiffMarkers(markers map[int]DiffMarkerKind) {
	if markers == nil {
		this.diffMarkers = nil
		this.Self().Update()
		return
	}
	m := make(map[int]DiffMarkerKind, len(markers))
	for line, kind := range markers {
		if kind == DiffMarkerNone {
			continue
		}
		m[line] = kind
	}
	this.diffMarkers = m
	this.Self().Update()
}

// ClearDiffMarkers drops all diff markers. The gutter bars become invisible.
func (this *CodeEditor) ClearDiffMarkers() {
	this.diffMarkers = nil
	this.Self().Update()
}

// DiffMarkers returns a copy of the current marker set, so mutating the result
// does not affect the editor's internal state.
func (this *CodeEditor) DiffMarkers() map[int]DiffMarkerKind {
	m := make(map[int]DiffMarkerKind, len(this.diffMarkers))
	for line, kind := range this.diffMarkers {
		m[line] = kind
	}
	return m
}

// SetDiffFromLines builds the marker set from three line lists (0-based) and
// installs it. added → DiffMarkerAdded, modified → DiffMarkerModified, removed
// → DiffMarkerRemoved. Overlap precedence is Removed > Modified > Added: a line
// listed in more than one bucket takes the highest-precedence kind, applied by
// writing Added first, then Modified, then Removed last so it wins. Triggers a
// repaint via SetDiffMarkers.
func (this *CodeEditor) SetDiffFromLines(added, modified, removed []int) {
	m := make(map[int]DiffMarkerKind, len(added)+len(modified)+len(removed))
	for _, line := range added {
		m[line] = DiffMarkerAdded
	}
	for _, line := range modified {
		m[line] = DiffMarkerModified
	}
	for _, line := range removed {
		m[line] = DiffMarkerRemoved
	}
	this.SetDiffMarkers(m)
}

// --- Blame (Annotate) Layer ---

// blameBaseColumnW is the reserved width (px) of the blame annotation column. It
// doubles as the truncation budget and the right-alignment span. Kept a package
// const so blameColumnWidth stays GL-free and unit-testable.
const blameBaseColumnW = 120.0

// blameColumnWidth returns the blame column width in pixels: blameBaseColumnW
// when the annotate view is visible, else 0. A 0 width means the column consumes
// no space and the gutter / text layout (gutterW, textOffX) is byte-identical to
// a build without blame — blame OFF never shifts anything.
func blameColumnWidth(visible bool) float64 {
	if !visible {
		return 0
	}
	return blameBaseColumnW
}

// truncateBlame shortens a blame annotation to at most max runes, replacing the
// tail with a single-rune ellipsis "…" when it would otherwise overflow. max<=0
// yields "" (no column room); a string already within budget is returned as-is.
// Operates on runes so multi-byte author names truncate cleanly.
func truncateBlame(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

// SetBlameAnnotations installs the per-line blame column and switches the
// annotate view on. Keys are 0-based line numbers; values are the host-computed
// annotation string (conventionally "shorthash author"). The map is copied so
// the host may mutate its own copy afterwards. Passing nil installs an empty set
// but still turns the view on (a blank column); use ClearBlame to hide it.
// Triggers a repaint.
func (this *CodeEditor) SetBlameAnnotations(m map[int]string) {
	blame := make(map[int]string, len(m))
	for line, ann := range m {
		blame[line] = ann
	}
	this.blame = blame
	this.blameVisible = true
	this.Self().Update()
}

// ClearBlame drops all blame annotations and hides the annotate column. The
// gutter / text layout returns to its blame-off geometry. Triggers a repaint.
func (this *CodeEditor) ClearBlame() {
	this.blame = nil
	this.blameVisible = false
	this.Self().Update()
}

// BlameVisible reports whether the annotate (blame) column is currently shown.
func (this *CodeEditor) BlameVisible() bool {
	return this.blameVisible
}

// BlameAnnotations returns a copy of the current blame set, so mutating the
// result does not affect the editor's internal state.
func (this *CodeEditor) BlameAnnotations() map[int]string {
	m := make(map[int]string, len(this.blame))
	for line, ann := range this.blame {
		m[line] = ann
	}
	return m
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

// nextLineIndent returns the leading-whitespace prefix to insert at the start
// of a new line produced by pressing Enter inside currentLine at cursorCol.
//
// Rules (Go-aware, kept intentionally simple):
//  1. Copy the leading whitespace of currentLine ("indent preservation").
//  2. If the cursor sits at the END of the line AND the last non-space rune of
//     the portion before the cursor is '{', add one indentUnit on top.
//
// The function is pure (no editor state, no side effects) so it can be unit
// tested in isolation. cursorCol is a rune-count, matching CodeEditor.cursorCol.
// indentUnit is whatever the caller's editor uses for a single indent step
// (the CodeEditor uses a literal "\t").
func nextLineIndent(currentLine string, cursorCol int, indentUnit string) string {
	// Leading whitespace of the current line.
	indent := ""
	for _, r := range currentLine {
		if r == ' ' || r == '\t' {
			indent += string(r)
		} else {
			break
		}
	}

	runes := []rune(currentLine)
	if cursorCol < 0 {
		cursorCol = 0
	}
	if cursorCol > len(runes) {
		cursorCol = len(runes)
	}
	// "End of line" means: nothing but whitespace remains after the cursor.
	atEnd := strings.TrimSpace(string(runes[cursorCol:])) == ""
	if !atEnd {
		return indent
	}

	before := strings.TrimRight(string(runes[:cursorCol]), " \t")
	if before == "" {
		return indent
	}
	if before[len(before)-1] == '{' {
		indent += indentUnit
	}
	return indent
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
//
// Pathological-input guard: when len(lines) >= maxFoldComputeLines the scan is
// skipped and nil is returned. Folding becomes unavailable on the giant file;
// every other editor feature still works. The scan itself is iterative (no
// recursion), so depth alone is safe — the threshold exists to bound the total
// work done on every Draw call that asks for fold regions.
func computeFoldRegions(lines []string) []foldRegion {
	if len(lines) >= maxFoldComputeLines {
		return nil
	}
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

// --- External completion injection ---
//
// The editor's built-in completer only knows keywords, builtin types, gui.*
// helpers, and identifiers scraped from the open buffer. A host (e.g. silkide
// driving a gopls LSP client) can feed richer candidates into the SAME popup
// without the editor having to import core or understand the LSP wire format:
// the host converts its protocol items into ExternalCompletion values and
// pushes them in via SetExternalCompletions.
//
// Lifetime: injected items PERSIST until replaced by another
// SetExternalCompletions call or removed by ClearExternalCompletions. They are
// merged into the popup on every (re)build (see showCompletion), so they keep
// showing as the user types and the prefix narrows — matching how an LSP
// completion list behaves. The host is responsible for clearing them when they
// go stale (e.g. on file/cursor change or when the popup is dismissed).
//
// Dedup precedence: on a label (CompletionItem.Text) collision between a
// built-in and an external candidate, the EXTERNAL one wins — the LSP knows the
// real symbol and signature, so it replaces the buffer-scraped guess.

// ExternalCompletion is a completion candidate supplied by an external provider
// (e.g. an LSP server). The host converts its protocol items into this shape;
// the editor merges them with its built-in candidates in the same popup.
type ExternalCompletion struct {
	Label  string // shown in the list
	Detail string // right-aligned hint (type/signature)
	Insert string // text inserted on accept (defaults to Label if empty)
}

// SetExternalCompletions replaces the injected candidate set. Pass the items a
// host fetched from its provider; they are merged into the popup on the next
// (re)build and persist until replaced or cleared. A nil/empty slice is
// equivalent to ClearExternalCompletions.
func (this *CodeEditor) SetExternalCompletions(items []ExternalCompletion) {
	this.externalCompletions = items
}

// ClearExternalCompletions drops all injected candidates, returning the popup
// to its built-in sources.
func (this *CodeEditor) ClearExternalCompletions() {
	this.externalCompletions = nil
}

// TriggerCompletion programmatically opens the completion popup at the current
// cursor, merging in any external candidates. A host calls this after fetching
// provider results (e.g. an LSP completion response) and injecting them via
// SetExternalCompletions.
func (this *CodeEditor) TriggerCompletion() {
	this.showCompletion(this.currentWordPrefix())
	this.Self().Update()
}

// externalToItem converts an ExternalCompletion to the editor's internal
// CompletionItem. Insert defaults to Label when empty so accepting the item
// inserts the visible text; external items are tagged CikFunction so they pick
// up the function-kind icon/color in the popup.
func externalToItem(e ExternalCompletion) CompletionItem {
	text := e.Insert
	if text == "" {
		text = e.Label
	}
	return CompletionItem{Text: text, Kind: CikFunction, Detail: e.Detail}
}

// mergeCompletions appends external candidates to the built-in ones, deduping
// by Text with EXTERNAL winning on collision (the external item replaces the
// built-in in place, preserving overall order). The built-in slice is not
// mutated. Ranking/limiting is left to the caller's filter() pass.
func mergeCompletions(builtin []CompletionItem, external []ExternalCompletion) []CompletionItem {
	if len(external) == 0 {
		out := make([]CompletionItem, len(builtin))
		copy(out, builtin)
		return out
	}
	out := make([]CompletionItem, len(builtin))
	copy(out, builtin)
	// Index built-ins by Text so an external item with the same label replaces
	// the built-in in place rather than duplicating it.
	idx := make(map[string]int, len(out))
	for i, it := range out {
		if _, ok := idx[it.Text]; !ok {
			idx[it.Text] = i
		}
	}
	for _, e := range external {
		item := externalToItem(e)
		if pos, ok := idx[item.Text]; ok {
			out[pos] = item // external wins
			continue
		}
		idx[item.Text] = len(out)
		out = append(out, item)
	}
	return out
}

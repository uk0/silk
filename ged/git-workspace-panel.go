package ged

import (
	"strconv"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.GitWorkspacePanel", gui.TypeOf(GitWorkspacePanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.GitWorkspacePanel",
		Name: "源码管理 / Source Control",
		Icon: "tree-view",
		Desc: "分支 / 远程 + 冲突·已暂存·未暂存·未跟踪分组",
	})
}

// GitWorkspacePanel is the IDE's source-control pane: the working tree
// grouped the way git actually sees it, plus the repository-level actions
// that live above the file list. It is the grouped counterpart to
// GitChangesPanel's flat row list, and like every sibling panel it is a
// pure display/interaction widget — it never shells out to git.
//
// Layout, top to bottom:
//
//	branch bar    the current branch, its upstream and the ahead/behind
//	              marker (gitBranchBarText). Clicking it opens an inline
//	              name input in "checkout" mode.
//	remote chips  [取回 fetch] [拉取 pull] [推送 push]
//	branch chips  [新建 new branch] [暂存 stash] [弹出 stash pop]
//	file list     collapsible 冲突 / 已暂存 / 未暂存 / 未跟踪 groups, one row
//	              per file, driven by the per-file GitIndexState the host
//	              reported (GitWorkspaceModel owns the grouping).
//	remotes band  a collapsible list of the configured remotes, pinned to
//	              the bottom edge the way GitChangesPanel pins its commit
//	              band.
//
// The host (silkide) drives every git call and pushes state in — SetFiles /
// SetBranch / SetRemotes — then listens on the callbacks:
//
//	SigCheckout(branch)     switch to an existing branch (git checkout)
//	SigCreateBranch(name)   create + switch (git checkout -b)
//	SigFetch/SigPull/SigPush  remote sync
//	SigStash/SigStashPop    git stash / git stash pop
//	SigResolve(path)        mark a conflicted file resolved (git add <path>)
//	SigFileActivated(path)  open the file in the editor
//
// After any of those the host re-reads git and pushes fresh state back; the
// panel never predicts the outcome of an action it asked for.
//
// Both chip strips are left-aligned at fixed widths, so every action
// hit-test is a pure function of x alone (gitWsChipAt) — no font
// measurement and no dependency on the widget's size off the paint path.
type GitWorkspacePanel struct {
	gui.Widget

	model   GitWorkspaceModel
	branch  GitBranchInfo
	remotes []GitRemote

	// remotesCollapsed folds the bottom remotes band down to its header.
	remotesCollapsed bool

	// Inline branch-name input in the branch bar: which action it will
	// submit to (none when closed) and the typed text. A rolled text line
	// in the same idiom as GitChangesPanel's commit input — no embedded
	// gui.Edit.
	inputMode gitWsInputMode
	inputText string

	scrollY   float64
	hoverIdx  int
	rowHeight float64

	cbCheckout  func(branch string)
	cbCreate    func(name string)
	cbFetch     func()
	cbPull      func()
	cbPush      func()
	cbStash     func()
	cbStashPop  func()
	cbResolve   func(path string)
	cbActivated func(path string)
}

// gitWsInputMode is which action the branch bar's inline input submits to.
type gitWsInputMode int

const (
	gitWsInputNone     gitWsInputMode = iota // input closed
	gitWsInputCheckout                       // Enter fires SigCheckout
	gitWsInputCreate                         // Enter fires SigCreateBranch
)

// NewGitWorkspacePanel creates an empty source-control panel.
func NewGitWorkspacePanel() *GitWorkspacePanel {
	p := new(GitWorkspacePanel)
	p.Init(p)
	return p
}

func (this *GitWorkspacePanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
}

// SetFiles replaces the grouped file list with the states the host read out
// of git. Per-group collapse state survives the push, so a refresh after a
// stage does not re-open groups the user folded.
func (this *GitWorkspacePanel) SetFiles(files []GitFileState) {
	this.model.SetFiles(files)
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// SetEntries is the convenience push for a host that has raw porcelain
// entries: it buckets them with ClassifyGitEntry and defers to SetFiles.
func (this *GitWorkspacePanel) SetEntries(entries []core.GitStatusEntry) {
	this.SetFiles(GitFileStatesFromEntries(entries))
}

// Files returns a defensive copy of the pushed file states, ungrouped.
func (this *GitWorkspacePanel) Files() []GitFileState {
	return this.model.Files()
}

// Rows returns the flattened row list the panel draws — group headers plus
// the files of every expanded group. Exposed so a host (or a test) can see
// exactly what is on screen without a paint.
func (this *GitWorkspacePanel) Rows() []GitWorkspaceRow {
	return this.model.Rows()
}

// Collapsed reports whether a group is folded.
func (this *GitWorkspacePanel) Collapsed(state GitIndexState) bool {
	return this.model.Collapsed(state)
}

// SetCollapsed folds (on) or unfolds (off) a group and repaints. Host-
// callable so a persisted UI layout can restore the fold state.
func (this *GitWorkspacePanel) SetCollapsed(state GitIndexState, on bool) {
	this.model.SetCollapsed(state, on)
	this.Self().Update()
}

// SetBranch replaces the branch bar's contents.
func (this *GitWorkspacePanel) SetBranch(info GitBranchInfo) {
	this.branch = info
	this.Self().Update()
}

// Branch returns the branch info last pushed in.
func (this *GitWorkspacePanel) Branch() GitBranchInfo {
	return this.branch
}

// SetRemotes replaces the bottom remotes list with a defensive copy.
func (this *GitWorkspacePanel) SetRemotes(remotes []GitRemote) {
	out := make([]GitRemote, len(remotes))
	copy(out, remotes)
	this.remotes = out
	this.Self().Update()
}

// Remotes returns a defensive copy of the remotes list.
func (this *GitWorkspacePanel) Remotes() []GitRemote {
	out := make([]GitRemote, len(this.remotes))
	copy(out, this.remotes)
	return out
}

// Clear empties the file list and closes the inline input. Branch info and
// remotes stay: they describe the repository, not the working tree, and the
// host drops them by pushing empty values when the repo context goes away.
func (this *GitWorkspacePanel) Clear() {
	this.model.Clear()
	this.scrollY = 0
	this.hoverIdx = -1
	this.inputMode = gitWsInputNone
	this.inputText = ""
	this.Self().Update()
}

// --- Signals ---

// SigCheckout registers the callback fired when the user submits a branch
// name in the branch bar's input. The host runs `git checkout <branch>` and
// pushes the refreshed branch + file state back.
func (this *GitWorkspacePanel) SigCheckout(fn func(branch string)) {
	this.cbCheckout = fn
}

// SigCreateBranch registers the callback fired when the user submits a name
// after the [新建] chip. The host runs `git checkout -b <name>`.
func (this *GitWorkspacePanel) SigCreateBranch(fn func(name string)) {
	this.cbCreate = fn
}

// SigFetch registers the [取回] chip's callback (`git fetch`).
func (this *GitWorkspacePanel) SigFetch(fn func()) { this.cbFetch = fn }

// SigPull registers the [拉取] chip's callback (`git pull`).
func (this *GitWorkspacePanel) SigPull(fn func()) { this.cbPull = fn }

// SigPush registers the [推送] chip's callback (`git push`).
func (this *GitWorkspacePanel) SigPush(fn func()) { this.cbPush = fn }

// SigStash registers the [暂存] chip's callback (`git stash`).
func (this *GitWorkspacePanel) SigStash(fn func()) { this.cbStash = fn }

// SigStashPop registers the [弹出] chip's callback (`git stash pop`).
func (this *GitWorkspacePanel) SigStashPop(fn func()) { this.cbStashPop = fn }

// SigResolve registers the callback fired when the user clicks the mark
// column of a row in the 冲突 group. It receives the conflicted path; the
// host marks it resolved (`git add <path>`) and re-reads git.
func (this *GitWorkspacePanel) SigResolve(fn func(path string)) {
	this.cbResolve = fn
}

// SigFileActivated registers the callback fired when a file row's body is
// clicked. The host opens that path in the editor.
func (this *GitWorkspacePanel) SigFileActivated(fn func(path string)) {
	this.cbActivated = fn
}

// --- Geometry (pure, GL-free) ---

const (
	gitWsBarH    = 22.0          // height of each of the three top bars
	gitWsListTop = 3 * gitWsBarH // first row of the grouped file list
	gitWsChipW   = 52.0          // action chip width
	gitWsChipGap = 4.0           // gap between chips
	gitWsChipX0  = 6.0           // left inset of a chip strip
	gitWsGlyphX  = 6.0           // group disclosure glyph / conflict mark
	gitWsLetterX = 24.0          // file row status letter
	gitWsTextX   = 38.0          // file row path text
)

// The two chip strips. Order is load-bearing: gitWsChipAt returns an index
// into these slices and OnLeftDown switches on it.
var (
	gitWsRemoteChips = []string{"取回", "拉取", "推送"} // fetch / pull / push
	gitWsBranchChips = []string{"新建", "暂存", "弹出"} // new branch / stash / stash pop
)

// gitWsBarAt maps a y coordinate to one of the three fixed top bars — 0 the
// branch bar, 1 the remote chips, 2 the branch/stash chips — or -1 when y
// falls below them (the scrolling list or the remotes band). Pure.
func gitWsBarAt(y float64) int {
	if y < 0 || y >= gitWsListTop {
		return -1
	}
	return int(y / gitWsBarH)
}

// gitWsChipRect returns the [x0, x1) span of chip i in a left-aligned strip.
func gitWsChipRect(i int) (x0, x1 float64) {
	x0 = gitWsChipX0 + float64(i)*(gitWsChipW+gitWsChipGap)
	return x0, x0 + gitWsChipW
}

// gitWsChipAt maps an x coordinate to a chip index in a strip of count
// chips, or -1 when x lands in a gap, left of the strip, or past its end.
// Pure and size-independent, so every action's hit-test is unit-testable.
func gitWsChipAt(x float64, count int) int {
	for i := 0; i < count; i++ {
		x0, x1 := gitWsChipRect(i)
		if x >= x0 && x < x1 {
			return i
		}
	}
	return -1
}

// gitWsMarkHitX reports whether x lands in a file row's left mark column
// ([0, gitWsLetterX), left of the status letter). On a 冲突 row that column
// is the "mark resolved" affordance, so a click there fires SigResolve
// instead of opening the file — the same column-split idiom
// git-changes-panel.go's checkboxHitX uses for its stage checkbox.
func gitWsMarkHitX(x float64) bool {
	return x < gitWsLetterX
}

// remotesBandRows is how many rows the bottom remotes band occupies: its
// header plus one row per remote when expanded, header only when folded.
func (this *GitWorkspacePanel) remotesBandRows() int {
	if this.remotesCollapsed {
		return 1
	}
	return 1 + len(this.remotes)
}

// remotesBandHeight is the pixel height the remotes band reserves at the
// bottom (its rows plus a 1px separator on top).
func (this *GitWorkspacePanel) remotesBandHeight() float64 {
	return this.rowHeight*float64(this.remotesBandRows()) + 1
}

// remotesBand returns the y where the bottom remotes band starts and
// whether it is shown at all. It is hidden when the widget is too short to
// leave at least one file row above it (which includes an unrealized,
// zero-height widget), so the list keeps the whole area — the same idiom as
// GitChangesPanel.commitBand.
func (this *GitWorkspacePanel) remotesBand() (top float64, ok bool) {
	_, h := this.Size()
	top = h - this.remotesBandHeight()
	ok = top >= gitWsListTop+this.rowHeight
	return
}

// remotesHeaderAt reports whether y lands on the remotes band's header row
// (the fold toggle).
func (this *GitWorkspacePanel) remotesHeaderAt(y float64) bool {
	top, ok := this.remotesBand()
	if !ok {
		return false
	}
	return y >= top+1 && y < top+1+this.rowHeight
}

// listBottom is the y where the scrolling file list stops: the top of the
// remotes band when it is shown, the widget's bottom edge otherwise.
func (this *GitWorkspacePanel) listBottom() float64 {
	if top, ok := this.remotesBand(); ok {
		return top
	}
	_, h := this.Size()
	return h
}

// rowAt maps a y coordinate to an index into Rows(), or -1 when y lands on
// the top bars, inside the remotes band, or past the last row. Folds the
// scroll offset in and defers to the pure refRowAtY helper.
func (this *GitWorkspacePanel) rowAt(y float64) int {
	if y < gitWsListTop {
		return -1
	}
	if top, ok := this.remotesBand(); ok && y >= top {
		return -1
	}
	return refRowAtY(y+this.scrollY, gitWsListTop, this.rowHeight, len(this.model.Rows()))
}

// --- Inline branch input ---

// openInput opens the branch bar's inline input in the given mode, clearing
// any text from a previous mode. Re-opening the mode already showing keeps
// what the user typed.
func (this *GitWorkspacePanel) openInput(mode gitWsInputMode) {
	if this.inputMode == mode {
		return
	}
	this.inputMode = mode
	this.inputText = ""
	this.Self().Update()
}

// closeInput closes the inline input and drops the typed text.
func (this *GitWorkspacePanel) closeInput() {
	if this.inputMode == gitWsInputNone {
		return
	}
	this.inputMode = gitWsInputNone
	this.inputText = ""
	this.Self().Update()
}

// submitInput fires the callback for the open mode with the trimmed name,
// then closes the input. A blank name is a no-op — nothing fires and the
// input stays open — so the host only ever gets a runnable request.
func (this *GitWorkspacePanel) submitInput() {
	name := strings.TrimSpace(this.inputText)
	if name == "" {
		return
	}
	mode := this.inputMode
	this.closeInput()
	switch mode {
	case gitWsInputCheckout:
		if this.cbCheckout != nil {
			this.cbCheckout(name)
		}
	case gitWsInputCreate:
		if this.cbCreate != nil {
			this.cbCreate(name)
		}
	}
}

// fire invokes an action callback when the host registered one.
func (this *GitWorkspacePanel) fire(fn func()) {
	if fn != nil {
		fn()
	}
}

// toggleRemotes folds / unfolds the bottom remotes band.
func (this *GitWorkspacePanel) toggleRemotes() {
	this.remotesCollapsed = !this.remotesCollapsed
	this.Self().Update()
}

// --- Drawing ---

// Draw paints the branch bar, the two chip strips, the grouped file list and
// the bottom remotes band.
func (this *GitWorkspacePanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching the sibling git panes.
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)

	this.drawBranchBar(g, font, w)
	this.drawChipStrip(g, font, gitWsBarH, gitWsRemoteChips, paint.Color{R: 60, G: 95, B: 140, A: 255})
	this.drawChipStrip(g, font, 2*gitWsBarH, gitWsBranchChips, paint.Color{R: 95, G: 80, B: 135, A: 255})
	this.drawRows(g, font, w)

	if top, ok := this.remotesBand(); ok {
		this.drawRemotes(g, font, w, top)
	}
}

// drawBranchBar paints the top bar: the branch label with its upstream and
// ahead/behind marker, or — while the inline input is open — a prompt naming
// the pending action plus the typed name and a caret.
func (this *GitWorkspacePanel) drawBranchBar(g paint.Painter, font paint.Font, w float64) {
	fe := font.FontExtents()
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, 0, w, gitWsBarH)
	g.Fill()

	if this.inputMode != gitWsInputNone {
		prompt := "切换到 / checkout: "
		if this.inputMode == gitWsInputCreate {
			prompt = "新建分支 / new branch: "
		}
		g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
		g.DrawText1(8, fe.Ascent+4, prompt)
		x := 8 + font.TextExtents(prompt).Width
		g.SetBrush1(paint.Color{R: 215, G: 220, B: 230, A: 255})
		g.DrawText1(x, fe.Ascent+4, this.inputText)
		cx := x + font.TextExtents(this.inputText).Width + 1
		g.SetBrush1(paint.Color{R: 150, G: 190, B: 240, A: 255})
		g.Rectangle(cx, 4, 1.5, gitWsBarH-8)
		g.Fill()
		return
	}

	g.SetBrush1(paint.Color{R: 120, G: 170, B: 230, A: 255})
	g.DrawText1(8, fe.Ascent+4, "⑂ "+gitBranchBarText(this.branch))
}

// drawChipStrip paints one left-aligned strip of action chips at y=top.
func (this *GitWorkspacePanel) drawChipStrip(g paint.Painter, font paint.Font, top float64, labels []string, accent paint.Color) {
	fe := font.FontExtents()
	const m = 3.0 // vertical inset within the bar
	for i, label := range labels {
		x0, x1 := gitWsChipRect(i)
		g.SetBrush1(accent)
		g.Rectangle(x0, top+m, x1-x0, gitWsBarH-2*m)
		g.Fill()
		g.SetBrush1(paint.Color{R: 225, G: 232, B: 240, A: 255})
		lw := font.TextExtents(label).Width
		g.DrawText1(x0+(x1-x0-lw)/2, top+fe.Ascent+4, label)
	}
}

// drawRows paints the grouped file list: a disclosure glyph + label + tally
// per group header, then one row per file in an expanded group — status
// letter in its accent colour, dimmed directory, emphasised basename — with
// a resolve mark in the gutter of 冲突 rows.
func (this *GitWorkspacePanel) drawRows(g paint.Painter, font paint.Font, w float64) {
	rows := this.model.Rows()
	if len(rows) == 0 {
		return
	}
	fe := font.FontExtents()
	rh := this.rowHeight
	bottom := this.listBottom()

	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((bottom-gitWsListTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(rows); i++ {
		y := gitWsListTop + float64(i)*rh - this.scrollY
		if y >= bottom {
			break
		}
		r := rows[i]

		if i == this.hoverIdx {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		if r.Kind == GitWorkspaceGroupRow {
			glyph := "▼"
			if r.Collapsed {
				glyph = "▶"
			}
			g.SetBrush1(paint.Color{R: 110, G: 120, B: 140, A: 255})
			g.DrawText1(gitWsGlyphX, y+fe.Ascent+2, glyph)
			g.SetBrush1(paint.Color{R: 205, G: 210, B: 220, A: 255})
			g.DrawText1(gitWsLetterX, y+fe.Ascent+2, gitGroupHeaderText(r.State, r.Count))
			continue
		}

		// Conflict rows carry a "mark resolved" check in the left gutter.
		if r.State == GitConflict {
			g.SetBrush1(paint.Color{R: 230, G: 180, B: 60, A: 255})
			g.DrawText1(gitWsGlyphX, y+fe.Ascent+2, "✔")
		}

		letter := statusLetter(r.File.Entry)
		g.SetBrush1(statusColor(letter))
		g.DrawText1(gitWsLetterX, y+fe.Ascent+2, letter)

		x := gitWsTextX
		dir, base := splitPathLabel(rowLabel(r.File.Entry))
		if dir != "" {
			g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
			g.DrawText1(x, y+fe.Ascent+2, dir)
			x += font.TextExtents(dir).Width
		}
		g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
		g.DrawText1(x, y+fe.Ascent+2, base)
	}
}

// drawRemotes paints the bottom band at y=top: a hairline separator, a
// foldable "远程 / Remotes (N)" header, then one "name  url" row per remote
// while expanded.
func (this *GitWorkspacePanel) drawRemotes(g paint.Painter, font paint.Font, w, top float64) {
	fe := font.FontExtents()
	rh := this.rowHeight

	g.SetBrush1(paint.Color{R: 45, G: 45, B: 54, A: 255})
	g.Rectangle(0, top, w, 1)
	g.Fill()

	headerY := top + 1
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, headerY, w, rh)
	g.Fill()
	glyph := "▼"
	if this.remotesCollapsed {
		glyph = "▶"
	}
	g.SetBrush1(paint.Color{R: 110, G: 120, B: 140, A: 255})
	g.DrawText1(gitWsGlyphX, headerY+fe.Ascent+2, glyph)
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(gitWsLetterX, headerY+fe.Ascent+2,
		"远程 / Remotes ("+strconv.Itoa(len(this.remotes))+")")

	if this.remotesCollapsed {
		return
	}
	for i, rm := range this.remotes {
		y := headerY + float64(i+1)*rh
		g.SetBrush1(paint.Color{R: 150, G: 190, B: 240, A: 255})
		g.DrawText1(gitWsLetterX, y+fe.Ascent+2, rm.Name)
		g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
		g.DrawText1(gitWsLetterX+font.TextExtents(rm.Name).Width+10, y+fe.Ascent+2, rm.URL)
	}
}

// --- Events ---

// OnLeftDown routes a click. The three top bars take it first (branch bar
// opens the checkout input, the chips fire their action), then the bottom
// remotes band (its header folds, its rows are inert), then the grouped file
// list: a group header folds/unfolds, a 冲突 row's mark column asks the host
// to resolve the file, any other file-row click opens it.
func (this *GitWorkspacePanel) OnLeftDown(x, y float64) {
	this.SetFocus()

	switch gitWsBarAt(y) {
	case 0:
		this.openInput(gitWsInputCheckout)
		return
	case 1:
		switch gitWsChipAt(x, len(gitWsRemoteChips)) {
		case 0:
			this.fire(this.cbFetch)
		case 1:
			this.fire(this.cbPull)
		case 2:
			this.fire(this.cbPush)
		}
		return
	case 2:
		switch gitWsChipAt(x, len(gitWsBranchChips)) {
		case 0:
			this.openInput(gitWsInputCreate)
		case 1:
			this.fire(this.cbStash)
		case 2:
			this.fire(this.cbStashPop)
		}
		return
	}

	// Any click below the bars blurs the inline input.
	this.closeInput()

	if this.remotesHeaderAt(y) {
		this.toggleRemotes()
		return
	}
	if top, ok := this.remotesBand(); ok && y >= top {
		return // remote rows are display-only
	}

	row, ok := this.model.RowAt(this.rowAt(y))
	if !ok {
		return
	}
	if row.Kind == GitWorkspaceGroupRow {
		this.model.ToggleCollapsed(row.State)
		this.hoverIdx = -1
		this.Self().Update()
		return
	}
	path := row.File.Entry.Path
	if row.State == GitConflict && gitWsMarkHitX(x) {
		if this.cbResolve != nil {
			this.cbResolve(path)
		}
		return
	}
	if this.cbActivated != nil {
		this.cbActivated(path)
	}
}

// OnKeyDown drives the inline branch-name input while it is open: Enter
// submits, Esc closes, Backspace deletes a rune. Keys are ignored when the
// input is closed (the panel has no other key handling).
func (this *GitWorkspacePanel) OnKeyDown(key int, repeat bool) {
	if this.inputMode == gitWsInputNone {
		return
	}
	switch key {
	case gui.KeyEnter:
		this.submitInput()
	case gui.KeyEsc:
		this.closeInput()
	case gui.KeyBackSpace:
		if r := []rune(this.inputText); len(r) > 0 {
			this.inputText = string(r[:len(r)-1])
			this.Self().Update()
		}
	}
}

// OnTextInput feeds typed characters into the inline input while it is open.
// Enter / Backspace arrive via OnKeyDown, not here.
func (this *GitWorkspacePanel) OnTextInput(s string) {
	if this.inputMode == gitWsInputNone {
		return
	}
	if s == "\r" || s == "\n" {
		return
	}
	this.inputText += s
	this.Self().Update()
}

// OnMouseMove tracks hover state for the row highlight.
func (this *GitWorkspacePanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *GitWorkspacePanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the grouped file list, clamped to its content.
func (this *GitWorkspacePanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	maxScroll := float64(len(this.model.Rows()))*this.rowHeight - (this.listBottom() - gitWsListTop)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

func (this *GitWorkspacePanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 220, MinHeight: 120}
}

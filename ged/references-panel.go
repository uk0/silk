package ged

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.ReferencesPanel", gui.TypeOf(ReferencesPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.ReferencesPanel",
		Name: "引用 / References",
		Icon: "search",
		Desc: "符号的所有引用 (查找全部引用)",
	})
}

// RefKind classifies what a reference site does with the symbol, the way
// Qt Creator's and VS Code's reference views tag their rows: the symbol's
// own declaration, a write (assignment / mutation) or a plain read. The
// zero value is RefRead so a host that does not classify its hits — every
// caller of the legacy SetLocations — still gets sensible rows.
type RefKind int

const (
	RefRead        RefKind = iota // plain usage (zero value)
	RefWrite                      // assignment / mutation of the symbol
	RefDeclaration                // the declaration site itself
)

// ReferenceLoc is one usage site of a symbol — a single row in the
// "Find All References" list. It is the panel's own flat shape, NOT
// core.LSPLocation: the host (silkide) drives core.LSPClient.References,
// then converts each LSPLocation (uri + 0-based range) into a
// ReferenceLoc (file path + 1-based line + column + the source line
// text) before pushing the slice in via SetLocations. Keeping the panel
// on its own struct means it never has to know about LSP wire types or
// 0-vs-1-based conventions — that translation lives in the host.
type ReferenceLoc struct {
	File    string  // absolute or workspace-relative path to the source file
	Line    int     // 1-based line number, ready to display/jump to
	Col     int     // column on that line (host's choice of base; passed through to the jump callback)
	Preview string  // the source line's text, trimmed, shown alongside the locator
	Kind    RefKind // read (zero value) / write / declaration — drives the row badge
}

// ReferencesPanel is the bottom-dock pane that lists every usage of a
// symbol, modelled on VS Code's references view and on the sibling
// ProblemsPanel: a counted header, one row per location, a per-row
// file:line locator, alternating row tint, wheel scroll and a hover/
// selection highlight. Clicking a row emits SigLocationActivated so the
// host can open that file at that line.
//
// Like DebugPanel it is a pure display/interaction widget: it never
// talks to gopls itself. The host fetches the references, converts them
// to []ReferenceLoc and calls SetReferences (or the older SetLocations);
// the panel only renders and reports clicks back.
//
// Two views share the same row geometry. The flat list is the original
// one and stays the default, so SetLocations keeps behaving exactly as
// before. SetReferences opts into the grouped view: references are
// stacked under a collapsible per-file header ("5 references in foo.go"),
// with the push ordered deterministically (path, line, col) so the same
// query always paints the same rows. On top of that both views support a
// substring filter (SetFilter), a query title in the header (SetQuery),
// per-row kind badges and an in-flight state with a cancel button
// (SetLoading / SigCancel).
type ReferencesPanel struct {
	gui.Widget

	locs      []ReferenceLoc
	scrollY   float64
	hoverIdx  int // index into the visible row list, -1 when none
	selected  int // index into locs of the last-activated reference, -1 when none
	rowHeight float64

	grouped   bool            // stack rows under collapsible per-file headers
	collapsed map[string]bool // collapsed group paths, keyed by file (grouped view)
	filter    string          // case-insensitive substring narrowing the visible rows
	query     string          // symbol / query title shown in the header
	loading   bool            // a search is in flight: header shows progress + cancel

	cbActivate func(file string, line, col int)
	cbCancel   func()
}

// NewReferencesPanel creates an empty references panel.
func NewReferencesPanel() *ReferencesPanel {
	p := new(ReferencesPanel)
	p.Init(p)
	return p
}

func (this *ReferencesPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverIdx = -1
	this.selected = -1
	this.collapsed = make(map[string]bool)
}

// SetLocations replaces the reference rows with a defensive copy and
// resets the view. A copy is taken so the host can keep mutating (or
// reuse) the slice it handed in without corrupting the panel's state.
//
// This is the original flat push: the rows keep the order the host
// produced and the view mode is left alone. Use SetReferences for the
// grouped, deterministically ordered view.
func (this *ReferencesPanel) SetLocations(locs []ReferenceLoc) {
	this.locs = make([]ReferenceLoc, len(locs))
	copy(this.locs, locs)
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = -1
	this.Self().Update()
}

// SetReferences replaces the reference rows with a defensive copy in a
// deterministic order — stable sort by file path, then line, then column
// — and switches the pane to the grouped-by-file view. Determinism is the
// point: gopls returns references in whatever order its index walk
// produced, so re-running the same query would otherwise reshuffle the
// list under the user's cursor. Because the sort makes each file's rows
// contiguous, the group order is plain path order.
//
// Group collapse state is reset (fresh groups start expanded) since the
// previous paths need not exist in the new result. The filter and the
// query title are view settings and survive the push; call SetFilter("")
// or SetQuery("") to drop them.
func (this *ReferencesPanel) SetReferences(refs []ReferenceLoc) {
	out := make([]ReferenceLoc, len(refs))
	copy(out, refs)
	sortReferences(out)
	this.locs = out
	this.grouped = true
	this.collapsed = make(map[string]bool)
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = -1
	this.Self().Update()
}

// Locations returns a defensive copy of the reference rows in display
// order. Returning the backing slice would let callers mutate the
// panel's state from the outside.
func (this *ReferencesPanel) Locations() []ReferenceLoc {
	out := make([]ReferenceLoc, len(this.locs))
	copy(out, this.locs)
	return out
}

// Clear removes all reference rows and resets the view, including the
// query title, the collapse state and the in-flight flag. The filter is
// a user-typed view setting and is left in place.
func (this *ReferencesPanel) Clear() {
	this.locs = nil
	this.scrollY = 0
	this.hoverIdx = -1
	this.selected = -1
	this.collapsed = make(map[string]bool)
	this.query = ""
	this.loading = false
	this.Self().Update()
}

// SetGrouped selects the view: true stacks references under collapsible
// per-file headers, false renders the original flat list. SetReferences
// turns grouping on; call this afterwards to opt back out.
func (this *ReferencesPanel) SetGrouped(on bool) {
	if this.grouped == on {
		return
	}
	this.grouped = on
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// Grouped reports whether the grouped-by-file view is active.
func (this *ReferencesPanel) Grouped() bool { return this.grouped }

// IsCollapsed reports whether the group for a file path is collapsed.
// Exposed for tests and for hosts that persist the pane's UI state.
func (this *ReferencesPanel) IsCollapsed(file string) bool {
	return this.collapsed[file]
}

// SetFilter narrows the visible rows to the references whose file path or
// source-line preview contains text, case-insensitively. Filtered-out
// rows stay in the backing slice — clearing the filter brings them back —
// and a group whose every child is filtered out disappears with them.
func (this *ReferencesPanel) SetFilter(text string) {
	if this.filter == text {
		return
	}
	this.filter = text
	this.scrollY = 0
	this.hoverIdx = -1
	this.Self().Update()
}

// Filter returns the current filter text.
func (this *ReferencesPanel) Filter() string { return this.filter }

// SetQuery sets the title shown in the header — normally the symbol the
// search ran for. With a title the header reads "Bar — 3 matches in 2
// files"; without one it keeps the plain "引用 / References (3)" tally.
func (this *ReferencesPanel) SetQuery(title string) {
	if this.query == title {
		return
	}
	this.query = title
	this.Self().Update()
}

// Query returns the current header title.
func (this *ReferencesPanel) Query() string { return this.query }

// SetLoading marks a search as in flight: the header shows a progress
// hint and a cancel button. The host owns the search, so it also owns
// this flag — call SetLoading(false) when the result (or the failure)
// arrives.
func (this *ReferencesPanel) SetLoading(on bool) {
	if this.loading == on {
		return
	}
	this.loading = on
	this.Self().Update()
}

// Loading reports whether a search is currently marked in flight.
func (this *ReferencesPanel) Loading() bool { return this.loading }

// SigLocationActivated registers the callback fired when the user clicks
// a reference row. It receives the target file, the 1-based line, and
// the column — the host opens file:line in the editor.
func (this *ReferencesPanel) SigLocationActivated(fn func(file string, line, col int)) {
	this.cbActivate = fn
}

// SigCancel registers the callback fired when the user clicks the header's
// cancel button while a search is in flight. The panel clears its own
// loading flag as it fires, so the button stops indicating progress even
// if the host's cancellation is asynchronous.
func (this *ReferencesPanel) SigCancel(fn func()) {
	this.cbCancel = fn
}

// --- Pure helpers (GL-free, unit-testable) ---

// refRowAtY maps a y coordinate to a reference-row index for a list
// whose rows start at topOffset, with count rows of height rowH. The
// caller folds the scroll offset into y before calling. It returns -1
// when y lands above the rows (the header band), past the last row, or
// when rowH is degenerate. Pure so the hit-test needs no widget or GL.
// (Named refRowAtY, not rowAtY, because git-changes-panel.go already
// owns a package-level rowAtY — same namespacing as debug-panel.go's
// frameRowAtY.)
func refRowAtY(y, topOffset, rowH float64, count int) int {
	if rowH <= 0 || y < topOffset {
		return -1
	}
	idx := int((y - topOffset) / rowH)
	if idx < 0 || idx >= count {
		return -1
	}
	return idx
}

// refRowLabel formats a location's left-hand locator as "basename:line"
// (e.g. "foo.go:42"), dropping the directory so the list stays scannable
// regardless of how deep the file lives. Pure and testable. (Named
// refRowLabel to avoid git-changes-panel.go's package-level rowLabel.)
func refRowLabel(loc ReferenceLoc) string {
	return filepath.Base(loc.File) + ":" + strconv.Itoa(loc.Line)
}

// referenceCountLabel renders the header tally, e.g. "引用 / References
// (3)". Kept as a free function so the header text is pure and testable
// without the renderer.
func referenceCountLabel(count int) string {
	return "引用 / References (" + strconv.Itoa(count) + ")"
}

// sortReferences orders references in place: file path, then line, then
// column. The sort is stable so hits that share all three (two kinds
// reported at one locator, say) keep the order the host produced.
func sortReferences(locs []ReferenceLoc) {
	sort.SliceStable(locs, func(i, j int) bool {
		a, b := locs[i], locs[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
}

// refMatchesFilter reports whether a reference survives the filter: a
// case-insensitive substring match against its file path or its source
// line preview. An empty filter matches everything.
func refMatchesFilter(loc ReferenceLoc, filter string) bool {
	if filter == "" {
		return true
	}
	f := strings.ToLower(filter)
	return strings.Contains(strings.ToLower(loc.File), f) ||
		strings.Contains(strings.ToLower(loc.Preview), f)
}

// refRowKind distinguishes the two drawable line shapes.
type refRowKind int

const (
	refRowGroup refRowKind = iota // collapsible per-file header
	refRowMatch                   // one reference
)

// refRow is one drawable line in the panel. Splitting the flat row list
// out of the widget keeps the layout decision (which rows show, in what
// order) pure and unit-testable; the panel only walks this slice when
// drawing and hit-testing. RefIdx points back into the panel's reference
// slice so a click resolves without re-walking the groups.
type refRow struct {
	Kind   refRowKind
	File   string // owning file path, on both row kinds
	RefIdx int    // index into ReferencesPanel.locs; -1 on group rows
	Count  int    // group rows: how many (filtered) children the group holds
}

// buildRefRows flattens references plus filter and collapse state into
// the row sequence the panel renders. Order is deterministic for any
// input: in the flat view rows keep slice order; in the grouped view
// groups appear in order of first appearance among the surviving rows
// (which is path order after SetReferences' sort) and each group's
// children keep slice order. Pure helper — no widget, no GL.
func buildRefRows(locs []ReferenceLoc, filter string, collapsed map[string]bool, grouped bool) []refRow {
	if !grouped {
		rows := make([]refRow, 0, len(locs))
		for i, loc := range locs {
			if !refMatchesFilter(loc, filter) {
				continue
			}
			rows = append(rows, refRow{Kind: refRowMatch, File: loc.File, RefIdx: i})
		}
		return rows
	}

	var order []string
	kids := make(map[string][]int)
	for i, loc := range locs {
		if !refMatchesFilter(loc, filter) {
			continue
		}
		if _, seen := kids[loc.File]; !seen {
			order = append(order, loc.File)
		}
		kids[loc.File] = append(kids[loc.File], i)
	}

	rows := make([]refRow, 0, len(order)+len(locs))
	for _, file := range order {
		idxs := kids[file]
		rows = append(rows, refRow{Kind: refRowGroup, File: file, RefIdx: -1, Count: len(idxs)})
		if collapsed[file] {
			continue
		}
		for _, i := range idxs {
			rows = append(rows, refRow{Kind: refRowMatch, File: file, RefIdx: i})
		}
	}
	return rows
}

// refCounts tallies the references that survive the filter and how many
// distinct files they span — the two numbers the header reports.
func refCounts(locs []ReferenceLoc, filter string) (matches, files int) {
	seen := make(map[string]bool, len(locs))
	for _, loc := range locs {
		if !refMatchesFilter(loc, filter) {
			continue
		}
		matches++
		if !seen[loc.File] {
			seen[loc.File] = true
			files++
		}
	}
	return matches, files
}

// refHeaderLabel renders the header text. Without a query it keeps the
// legacy tally so the pane looks unchanged until a host sets one; with a
// query it reads like Qt Creator's search-results header — the query,
// then the match and file counts of what is actually on screen (i.e.
// after the filter).
func refHeaderLabel(query string, matches, files int) string {
	if query == "" {
		return referenceCountLabel(matches)
	}
	return query + " — " + refPlural(matches, "match", "matches") +
		" in " + refPlural(files, "file", "files")
}

// refPlural renders "1 match" / "3 matches" so the header never reads
// "1 matches".
func refPlural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// refGroupLabel formats a group header as "basename (N)" — the file's
// name plus how many references it holds.
func refGroupLabel(file string, n int) string {
	return filepath.Base(file) + " (" + strconv.Itoa(n) + ")"
}

// refKindBadge maps a reference kind to its short row badge. An unknown
// kind gets no badge ("") so a value the panel does not understand
// renders as a plain row instead of a bogus label.
func refKindBadge(k RefKind) string {
	switch k {
	case RefDeclaration:
		return "decl"
	case RefWrite:
		return "write"
	case RefRead:
		return "read"
	}
	return ""
}

// refKindTint is the badge fill per kind: amber for the declaration,
// red for a write, muted blue-grey for a plain read. Distinct enough to
// tell apart at badge size in either theme.
func refKindTint(k RefKind) paint.Color {
	switch k {
	case RefDeclaration:
		return paint.Color{R: 176, G: 132, B: 58, A: 255}
	case RefWrite:
		return paint.Color{R: 172, G: 82, B: 82, A: 255}
	}
	return paint.Color{R: 108, G: 122, B: 142, A: 255}
}

// refCancelHit reports whether (x,y) lands on the header's cancel
// hotspot — a fixed-width band at the right end of the header band, for
// a pane w wide. Pure so the hit-test needs no widget or GL. Only
// consulted while a search is in flight, so a non-loading header click
// can never hit a button that is not drawn.
func refCancelHit(x, y, w float64) bool {
	return y >= 0 && y < referencesHeaderH && x >= w-refCancelW && x <= w
}

// visibleRows is the row list the draw and hit-test paths share, built
// from the current references, filter, collapse state and view mode.
func (this *ReferencesPanel) visibleRows() []refRow {
	return buildRefRows(this.locs, this.filter, this.collapsed, this.grouped)
}

// toggleGroup flips a file group's collapse state. Pulled out of
// OnLeftDown so tests can drive the toggle without faking row geometry.
func (this *ReferencesPanel) toggleGroup(file string) {
	if this.collapsed == nil {
		this.collapsed = make(map[string]bool)
	}
	this.collapsed[file] = !this.collapsed[file]
	this.hoverIdx = -1
	this.Self().Update()
}

// --- Drawing ---

const (
	referencesHeaderH = 22.0
	refCancelW        = 60.0 // header-right cancel hotspot width
	refGroupIndent    = 26.0 // child-row indent under a group header
)

// refStripeColor is the alternating-row tint. FormColor sits just off the
// view background in the light theme; in the dark one the two are the
// same zinc, so FormLightColor does the job instead.
func refStripeColor() paint.Color {
	t := gui.Theme()
	if gui.CurrentThemeMode() == gui.ThemeDark {
		return t.FormLightColor
	}
	return t.FormColor
}

// refWashColor is the hover / selection wash. The theme has no dedicated
// pair, so both are the accent at low alpha: that reads over either the
// light or the dark view background and, unlike a solid accent fill,
// leaves the row's own text legible.
func refWashColor(alpha uint8) paint.Color {
	c := gui.Theme().HighLightColor
	return paint.Color{R: c.R, G: c.G, B: c.B, A: alpha}
}

// Draw renders the header — query title, match/file tally, the active
// filter and, while a search is in flight, a progress hint plus the
// cancel button — followed by the visible rows: collapsible file headers
// with their child references in the grouped view, a flat list otherwise.
// Each reference row carries its locator, a kind badge and the trimmed
// source line. Colors come from gui.Theme() so the pane follows the IDE's
// light/dark scheme.
func (this *ReferencesPanel) Draw(g paint.Painter) {
	w, h := this.Size()
	t := gui.Theme()

	g.SetBrush1(t.ViewBGColor)
	g.Rectangle(0, 0, w, h)
	g.Fill()

	rowFont := paint.NewFont("Menlo", 12, false, false)
	boldFont := paint.NewFont("Menlo", 12, true, false)
	badgeFont := paint.NewFont("Menlo", 9, false, false)

	// Header band: query + tally, then the filter chip / progress hint.
	g.SetBrush1(t.FormColor)
	g.Rectangle(0, 0, w, referencesHeaderH)
	g.Fill()
	g.SetPen1(t.BorderColor, 1)
	g.MoveTo(0, referencesHeaderH)
	g.LineTo(w, referencesHeaderH)
	g.Stroke()

	g.SetFont(boldFont)
	hfe := boldFont.FontExtents()
	headBase := hfe.Ascent + (referencesHeaderH-hfe.Ascent-hfe.Descent)/2
	matches, files := refCounts(this.locs, this.filter)
	head := refHeaderLabel(this.query, matches, files)
	g.SetBrush1(t.TextColor)
	g.DrawText1(8, headBase, head)
	headX := 8 + boldFont.TextExtents(head).Width + 12

	g.SetFont(rowFont)
	if this.loading {
		g.SetBrush1(t.HighLightColor)
		g.DrawText1(headX, headBase, "搜索中… / Searching…")

		// Cancel button, geometry mirrored by refCancelHit.
		bx := w - refCancelW
		g.SetBrush1(t.FormLightColor)
		g.Rectangle(bx+2, 3, refCancelW-6, referencesHeaderH-6)
		g.Fill()
		g.SetPen1(t.BorderColor, 1)
		g.Rectangle(bx+2, 3, refCancelW-6, referencesHeaderH-6)
		g.Stroke()
		g.SetBrush1(t.TextColor)
		g.DrawText1(bx+8, headBase, "Cancel")
	} else if this.filter != "" {
		g.SetBrush1(t.FormDarkColor)
		g.DrawText1(headX, headBase, "⌕ "+this.filter)
	}

	rows := this.visibleRows()
	if len(rows) == 0 {
		g.SetBrush1(t.FormDarkColor)
		empty := "No references"
		if len(this.locs) > 0 {
			empty = "No matches for " + this.filter
		}
		g.DrawText1(8, referencesHeaderH+16, empty)
		return
	}

	rh := this.rowHeight
	areaTop := referencesHeaderH
	fe := rowFont.FontExtents()
	startIdx := int(this.scrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((h-areaTop)/rh) + 2

	for i := startIdx; i < startIdx+visibleCount && i < len(rows); i++ {
		r := rows[i]
		y := areaTop + float64(i)*rh - this.scrollY

		// Selection wins over hover wins over the group band / stripe.
		switch {
		case r.Kind == refRowMatch && r.RefIdx == this.selected:
			g.SetBrush1(refWashColor(96))
			g.Rectangle(0, y, w, rh)
			g.Fill()
		case i == this.hoverIdx:
			g.SetBrush1(refWashColor(40))
			g.Rectangle(0, y, w, rh)
			g.Fill()
		case r.Kind == refRowGroup:
			g.SetBrush1(t.FormColor)
			g.Rectangle(0, y, w, rh)
			g.Fill()
		case i%2 == 1:
			g.SetBrush1(refStripeColor())
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		textY := y + fe.Ascent + (rh-fe.Ascent-fe.Descent)/2

		if r.Kind == refRowGroup {
			glyph := "▼"
			if this.collapsed[r.File] {
				glyph = "▶"
			}
			g.SetFont(rowFont)
			g.SetBrush1(t.FormDarkColor)
			g.DrawText1(8, textY, glyph)

			label := refGroupLabel(r.File, r.Count)
			g.SetFont(boldFont)
			g.SetBrush1(t.TextColor)
			g.DrawText1(22, textY, label)

			// Dimmed directory after the name, so same-named files in
			// different packages stay distinguishable.
			if dir := filepath.Dir(r.File); dir != "" && dir != "." {
				g.SetFont(rowFont)
				g.SetBrush1(t.FormDarkColor)
				g.DrawText1(22+boldFont.TextExtents(label).Width+10, textY, dir)
			}
			continue
		}

		loc := this.locs[r.RefIdx]
		x := 8.0
		// In the grouped view the file lives on the header row, so the
		// child only needs its line number.
		label := refRowLabel(loc)
		if this.grouped {
			x = refGroupIndent
			label = strconv.Itoa(loc.Line)
		}
		g.SetFont(rowFont)
		g.SetBrush1(t.HighLightColor)
		g.DrawText1(x, textY, label)
		x += rowFont.TextExtents(label).Width + 10

		if badge := refKindBadge(loc.Kind); badge != "" {
			g.SetFont(badgeFont)
			bext := badgeFont.TextExtents(badge)
			bw := bext.Width + 8
			bh := rh - 8
			g.SetBrush1(refKindTint(loc.Kind))
			g.Rectangle(x, y+4, bw, bh)
			g.Fill()
			g.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 235})
			g.DrawText1(x+4, y+4+bh*0.5+bext.Height*0.5, badge)
			x += bw + 8
		}

		g.SetFont(rowFont)
		g.SetBrush1(t.TextColor)
		g.DrawText1(x, textY, loc.Preview)
	}
}

// --- Events ---

// OnLeftDown routes a click by row kind: a group header toggles its
// collapse state, a reference row fires the activated callback (the host
// opens file:line) and highlights itself. In the header band only the
// cancel button reacts, and only while a search is in flight.
func (this *ReferencesPanel) OnLeftDown(x, y float64) {
	this.SetFocus()

	if y < referencesHeaderH {
		w, _ := this.Size()
		if this.loading && refCancelHit(x, y, w) {
			this.loading = false
			this.Self().Update()
			if this.cbCancel != nil {
				this.cbCancel()
			}
		}
		return
	}

	rows := this.visibleRows()
	idx := this.rowAt(y)
	if idx < 0 || idx >= len(rows) {
		return
	}
	r := rows[idx]
	if r.Kind == refRowGroup {
		this.toggleGroup(r.File)
		return
	}
	if r.RefIdx < 0 || r.RefIdx >= len(this.locs) {
		return
	}
	this.selected = r.RefIdx
	this.Self().Update()
	if this.cbActivate != nil {
		loc := this.locs[r.RefIdx]
		this.cbActivate(loc.File, loc.Line, loc.Col)
	}
}

// OnMouseMove tracks hover state for the row highlight.
func (this *ReferencesPanel) OnMouseMove(x, y float64) {
	idx := this.rowAt(y)
	if idx != this.hoverIdx {
		this.hoverIdx = idx
		this.Self().Update()
	}
}

// OnMouseLeave clears the hover highlight.
func (this *ReferencesPanel) OnMouseLeave() {
	if this.hoverIdx != -1 {
		this.hoverIdx = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls the row list vertically, clamped to the content.
func (this *ReferencesPanel) OnMouseWheel(x, y, z float64) {
	this.scrollY -= z * 3 * this.rowHeight
	if this.scrollY < 0 {
		this.scrollY = 0
	}
	_, h := this.Size()
	maxScroll := float64(len(this.visibleRows()))*this.rowHeight - (h - referencesHeaderH)
	if maxScroll < 0 {
		maxScroll = 0
	}
	if this.scrollY > maxScroll {
		this.scrollY = maxScroll
	}
	this.Self().Update()
}

// rowAt maps a y coordinate (below the header) to a visible-row index,
// or -1 when y lands on the header band or past the last row. It folds
// the scroll offset into y and defers to the pure refRowAtY helper.
func (this *ReferencesPanel) rowAt(y float64) int {
	return refRowAtY(y+this.scrollY, referencesHeaderH, this.rowHeight, len(this.visibleRows()))
}

func (this *ReferencesPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 80}
}

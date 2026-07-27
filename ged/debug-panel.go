package ged

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("ged.DebugPanel", gui.TypeOf(DebugPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.DebugPanel",
		Name: "调试 / Debug",
		Icon: "debug",
		Desc: "断点处的调用栈与局部变量",
	})
}

// DebugPanel is the bottom-dock pane a debugger uses to show the breakpoint
// table, the call stack, the variables of the selected frame, the goroutines,
// watched expressions and a debug console. It is a pure display/interaction
// widget: it never talks to dlv itself. The host (silkide) drives a
// core.DebugSession — ListBreakpoints() / Stacktrace() / ListLocals() / Eval()
// — and pushes the results in via SetBreakpoints / SetCallStack / SetVariables
// (or SetLocals + SetArguments) / SetGoroutines / SetWatches / SetConsoleLines.
// The panel renders them and emits signals back:
//
//	SigFrameSelected  — a stack-frame row was clicked. The host re-fetches
//	                    locals for that frame (ListLocals at the frame's
//	                    depth) and calls SetVariables to refresh the lower
//	                    section.
//	SigFrameActivated — a stack-frame row was double-clicked / Enter'd. The
//	                    host opens frame.File:frame.Line in the editor.
//	SigContextChanged — the (goroutine, frame) evaluation context changed, by
//	                    a row click, a keyboard move or an explicit
//	                    SetSelectedGoroutine / SetSelectedFrame. The host
//	                    re-runs ListLocals / Eval IN THAT SCOPE instead of the
//	                    hardcoded (-1, 0). It is the canonical context signal;
//	                    a host should refresh locals from EITHER this or
//	                    SigFrameSelected, not both.
//	SigWatchAdded     — the user submitted a new expression in the watch
//	                    input. The host evaluates it (Eval) and pushes the
//	                    refreshed list back via SetWatches.
//	SigWatchRemoved   — the user dropped a watch row. The host stops
//	                    tracking that expression.
//	SigGoroutineActivated — a goroutine row was clicked. The host opens the
//	                    goroutine's File:Line in the editor. The click also
//	                    switches the evaluation context to that goroutine
//	                    (SigContextChanged).
//	SigVariableEdited — the user submitted a new value for a variable. The
//	                    host calls dlv SetVariable(path, newValue), re-fetches
//	                    the variables for the current frame, and pushes them
//	                    back via SetVariables / SetLocals.
//	SigExpandVar      — the user opened a composite variable whose children
//	                    are not loaded yet. The host evaluates that path in
//	                    the current context and pushes the children back via
//	                    SetChildren(path, rows) — lazy expansion, so a deep
//	                    struct is never fetched whole.
//	SigToggleBreakpoint — the enabled checkbox of a breakpoint row was
//	                    clicked. The host arms/disarms it in dlv (Amend, or
//	                    Clear + re-create) and may re-push via SetBreakpoints.
//	SigEditCondition  — a breakpoint condition was submitted in the inline
//	                    editor. An EMPTY cond means "clear the condition".
//	SigDeleteBreakpoint — the ✕ affordance of a breakpoint row was clicked.
//	                    The host clears it in dlv and in the editor gutter.
//	SigEval           — the user submitted an expression in the debug console.
//	                    The host evaluates it in the current context and
//	                    appends the command + result via SetConsoleLines.
//
// Six bands split the widget vertically, top to bottom: breakpoints, the call
// stack, the variables (Arguments + Locals as a lazily expandable tree), the
// goroutines, the watch section, and the debug console. Each band has its own
// header (with a count) and its own independent vertical scroll. The two
// optional bands take NO height when they have nothing to show: the
// breakpoints band collapses when the breakpoint list is empty, and the
// console band only appears once SetConsoleVisible(true) is called (or
// SetConsoleLines pushes output).
//
// Variables are editable in place: a double-click (or Enter) on a row opens an
// inline value editor over the value column, and submitting emits
// SigVariableEdited — the panel never calls dlv SetVariable itself, the host
// does and re-pushes the variables. Breakpoint conditions work the same way: a
// double-click on a breakpoint row opens an inline condition editor and
// submitting emits SigEditCondition.
type DebugPanel struct {
	gui.Widget

	frames   []core.StackFrame
	selected int // index into frames of the highlighted row; 0 = top frame

	stackScrollY float64
	varScrollY   float64
	hoverStack   int // hovered call-stack row, -1 when none
	hoverVar     int // hovered variables row, -1 when none
	rowHeight    float64

	// Double-click detection, mirroring file-explorer.go: the framework
	// has no native double-click event, so we time consecutive clicks on
	// the same frame row ourselves.
	lastClickIdx  int
	lastClickTime time.Time

	cbFrameSelected  func(index int, frame core.StackFrame)
	cbFrameActivated func(frame core.StackFrame)

	// Goroutines section: host-fed goroutine list with its own scroll and
	// hover, plus the row-activated callback. Like the other bands it is pure
	// display — SetGoroutines pushes the data, SigGoroutineActivated reports a
	// click, and the host opens the goroutine's file:line.
	goroutines  []core.Goroutine
	goroScrollY float64
	hoverGoro   int // hovered goroutine row, -1 when none

	cbGoroutineActivated func(g core.Goroutine)

	// Evaluation context: WHICH goroutine and WHICH frame every locals fetch
	// and expression evaluation runs in. selGoroutine is a dlv goroutine id,
	// -1 meaning "the current/selected goroutine"; the frame is `selected`
	// above (the highlighted stack row), 0 being the top frame. The host used
	// to hardcode (-1, 0); now it follows SigContextChanged.
	selGoroutine     int64
	cbContextChanged func(goroutineID int64, frame int)

	// Variables section. args/locals are the two root groups; children arrive
	// lazily per path (see SetChildren) and expansion state is a path set, so
	// a re-push of the groups collapses the tree instead of leaving stale
	// children from another frame behind. visCache is the flattened,
	// currently-visible row list — the hit-test and draw order — rebuilt from
	// the model whenever visDirty is set.
	args        []VarRow
	locals      []VarRow
	varChildren map[string][]VarRow
	varExpanded map[string]bool
	visCache    []varVisRow
	visDirty    bool

	cbExpandVar func(path string)

	// Inline variable-value editing. editingVar is the index into the
	// flattened visible rows whose value is being edited in place (-1 when
	// none); varInput is the in-progress text, seeded from the row's Value on
	// entry. Its own double-click timer keeps it independent of the
	// call-stack one.
	editingVar       int
	varInput         string
	lastVarClickIdx  int
	lastVarClickTime time.Time

	cbVariableEdited func(name string, newValue string)

	// Breakpoints table: the host-fed breakpoint list with its own scroll,
	// hover and inline condition editor. editingCond is the row being edited
	// (-1 when none) and condInput the in-progress condition text.
	breakpoints     []BreakpointRow
	bpScrollY       float64
	hoverBP         int // hovered breakpoint row, -1 when none
	editingCond     int
	condInput       string
	lastBPClickIdx  int
	lastBPClickTime time.Time

	cbToggleBreakpoint func(file string, line int, enabled bool)
	cbEditCondition    func(file string, line int, cond string)
	cbDeleteBreakpoint func(file string, line int)

	// Watch section: host-fed watched expressions plus an in-line editor the
	// user types new expressions into. Like the rest of the panel it never
	// evaluates anything itself — it emits SigWatchAdded / SigWatchRemoved
	// and renders whatever the host pushes back via SetWatches.
	watches      []WatchEntry
	watchScrollY float64
	hoverWatch   int    // hovered watch row, -1 when none
	watchInput   string // in-progress expression in the input line
	watchFocused bool   // whether the expression input line holds focus

	cbWatchAdded   func(expr string)
	cbWatchRemoved func(expr string)

	// Debug console: a host-fed transcript plus a prompt line at the bottom
	// of the band. consoleVisible gates the whole band so the panel keeps its
	// original four-band look until a host opts in.
	consoleLines   []string
	consoleScrollY float64
	consoleInput   string
	consoleFocused bool
	consoleVisible bool

	cbEval func(expr string)
}

// WatchEntry is one watched expression and its last evaluation. Expr is the
// Go expression the user typed; Value and Type are the dlv Eval result on
// success; Err is the error string when evaluation failed. A non-empty Err
// means Value/Type are not meaningful and the row renders the error. The
// host fills these in and pushes them via SetWatches.
type WatchEntry struct {
	Expr  string
	Value string
	Type  string
	Err   string
}

// BreakpointRow is one row of the breakpoints table. It is the panel's own
// flat shape, NOT core.Breakpoint: dlv's breakpoint carries an id and a
// function name but no UI state, while the table needs the condition, the hit
// count and the enabled/verified flags. The host converts ListBreakpoints()
// (plus whatever it knows about disabled/unbound breakpoints) into
// []BreakpointRow and pushes it via SetBreakpoints.
//
// Enabled is the armed state — a disabled breakpoint stays in the table but
// does not stop the program. Verified is dlv's view: false means the address
// could not be bound (a stale line, a file not in the binary), which the row
// renders hollow so the user sees why it never hits.
type BreakpointRow struct {
	File     string
	Line     int
	Cond     string // Go expression; empty for an unconditional breakpoint
	HitCount uint64
	Enabled  bool
	Verified bool
}

// VarRow is one node of the variables tree: a local, an argument, or a child
// of either. Name/Type/Value are the dlv projection (same three fields as
// core.Variable); HasChildren marks a composite — a struct, slice, map or
// pointer — the user can open. Children are NOT carried here: they are
// fetched lazily and installed by path via SetChildren, so pushing a frame's
// variables never drags a deep object graph along.
type VarRow struct {
	Name        string
	Type        string
	Value       string
	HasChildren bool
}

// varVisRow is one flattened, currently-visible variables row: either a group
// header ("Arguments" / "Locals") or a variable at some tree depth. The
// flattened list IS the hit-test and draw order of the variables band, so a y
// coordinate maps to an index in it exactly like the other bands' rows.
type varVisRow struct {
	row        VarRow
	path       string // dlv-evaluable path ("p.Next", "s[0]"); "" for a header
	label      string // header text; "" for a variable row
	depth      int    // indent level
	expandable bool   // draws a twisty: a composite the user can open
	expanded   bool   // its children are currently shown
}

// isHeader reports whether the row is a group header rather than a variable.
func (r varVisRow) isHeader() bool { return r.label != "" }

// NewDebugPanel creates an empty debug panel.
func NewDebugPanel() *DebugPanel {
	p := new(DebugPanel)
	p.Init(p)
	return p
}

func (this *DebugPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.rowHeight = 20
	this.hoverStack = -1
	this.hoverVar = -1
	this.hoverWatch = -1
	this.hoverGoro = -1
	this.hoverBP = -1
	this.lastClickIdx = -1
	this.lastVarClickIdx = -1
	this.lastBPClickIdx = -1
	this.editingVar = -1
	this.editingCond = -1
	this.selGoroutine = -1
}

// SetCallStack replaces the call-stack frames and resets the selection to
// the top frame (index 0). It does NOT emit SigFrameSelected — pushing a
// fresh stack is the host telling the panel "we just stopped here", not
// the user picking a frame; the host already knows the top frame and has
// loaded its locals. For the same reason it does not emit SigContextChanged.
func (this *DebugPanel) SetCallStack(frames []core.StackFrame) {
	this.frames = frames
	this.selected = 0
	this.stackScrollY = 0
	this.hoverStack = -1
	this.lastClickIdx = -1
	this.Self().Update()
}

// SetVariables replaces the local-variable rows from dlv's flat projection.
// The host calls this both on the initial stop (locals of the top frame) and
// again whenever the context changes. Rows pushed this way have no children
// (core.Variable cannot express one) — use SetLocals to push composites the
// user can expand.
func (this *DebugPanel) SetVariables(vars []core.Variable) {
	rows := make([]VarRow, len(vars))
	for i, v := range vars {
		rows[i] = VarRow{Name: v.Name, Type: v.Type, Value: v.Value}
	}
	this.SetLocals(rows)
}

// SetLocals replaces the Locals group with a defensive copy of rows, dropping
// every lazily-loaded child and every expansion: a fresh push is a fresh
// scope, so stale children from another frame must not survive it. The host
// re-fetches children on demand when the user re-opens a composite.
func (this *DebugPanel) SetLocals(rows []VarRow) {
	out := make([]VarRow, len(rows))
	copy(out, rows)
	this.locals = out
	this.resetVarTree()
	this.varScrollY = 0
	this.hoverVar = -1
	this.Self().Update()
}

// SetArguments replaces the Arguments group — the current frame's function
// parameters, which dlv reports separately from its locals (ListFunctionArgs
// vs ListLocalVars). The group renders above the locals under its own header
// and behaves identically: same tree, same lazy children, same inline editor.
// Like SetLocals it collapses the tree, since a push means a new scope.
func (this *DebugPanel) SetArguments(rows []VarRow) {
	out := make([]VarRow, len(rows))
	copy(out, rows)
	this.args = out
	this.resetVarTree()
	this.varScrollY = 0
	this.hoverVar = -1
	this.Self().Update()
}

// Locals returns a defensive copy of the Locals group's root rows.
func (this *DebugPanel) Locals() []VarRow {
	out := make([]VarRow, len(this.locals))
	copy(out, this.locals)
	return out
}

// Arguments returns a defensive copy of the Arguments group's root rows.
func (this *DebugPanel) Arguments() []VarRow {
	out := make([]VarRow, len(this.args))
	copy(out, this.args)
	return out
}

// SetChildren installs the children of the variable at path — the answer to a
// SigExpandVar request — and shows them: pushing children means the user asked
// for them. The path is the one the signal carried, i.e. a dlv-evaluable
// expression ("p.Next", "s[0]"), so the host can Eval it in the current
// context and hand the result back without keeping its own row bookkeeping.
// Pushing an empty slice marks the node loaded-but-empty, so re-opening it
// does not ask again.
func (this *DebugPanel) SetChildren(path string, rows []VarRow) {
	if path == "" {
		return
	}
	out := make([]VarRow, len(rows))
	copy(out, rows)
	if this.varChildren == nil {
		this.varChildren = make(map[string][]VarRow)
	}
	this.varChildren[path] = out
	if this.varExpanded == nil {
		this.varExpanded = make(map[string]bool)
	}
	this.varExpanded[path] = true
	this.visDirty = true
	this.Self().Update()
}

// SigExpandVar registers the callback fired when the user opens a composite
// variable whose children have not been loaded yet. The host evaluates path in
// the current (goroutine, frame) context and pushes the children back via
// SetChildren. It fires at most once per path: an already-loaded node
// expands from the cache.
func (this *DebugPanel) SigExpandVar(fn func(path string)) {
	this.cbExpandVar = fn
}

// resetVarTree drops every loaded child and every expansion, and invalidates
// the flattened row cache. Called whenever a variables group is replaced.
func (this *DebugPanel) resetVarTree() {
	this.varChildren = nil
	this.varExpanded = nil
	this.editingVar = -1
	this.varInput = ""
	this.lastVarClickIdx = -1
	this.visDirty = true
}

// varPath joins a parent path and a child row name into the child's path.
// Index-like names ("[0]") append directly so the result stays a valid Go
// expression (s[0], not s.[0]); every other name is dotted (p.Next).
func varPath(parent, name string) string {
	if parent == "" {
		return name
	}
	if strings.HasPrefix(name, "[") {
		return parent + name
	}
	return parent + "." + name
}

// visRows returns the flattened, currently-visible variables rows, rebuilding
// the cache when the model changed since the last call.
func (this *DebugPanel) visRows() []varVisRow {
	if this.visDirty {
		this.visCache = this.buildVisRows()
		this.visDirty = false
	}
	return this.visCache
}

// buildVisRows flattens the two groups into display order. Group headers only
// appear when there ARE arguments: with locals alone the band header already
// names the single group, so the rows stay flush left like before.
func (this *DebugPanel) buildVisRows() []varVisRow {
	grouped := len(this.args) > 0
	depth := 0
	var out []varVisRow
	if grouped {
		depth = 1
		out = append(out, varVisRow{label: "参数 / Arguments (" + strconv.Itoa(len(this.args)) + ")"})
	}
	out = this.appendVarRows(out, this.args, "", depth)
	if grouped {
		out = append(out, varVisRow{label: "局部变量 / Locals (" + strconv.Itoa(len(this.locals)) + ")"})
	}
	return this.appendVarRows(out, this.locals, "", depth)
}

// appendVarRows flattens rows — and, for every expanded node, its loaded
// children — onto out in display order.
func (this *DebugPanel) appendVarRows(out []varVisRow, rows []VarRow, parent string, depth int) []varVisRow {
	for _, r := range rows {
		path := varPath(parent, r.Name)
		kids, loaded := this.varChildren[path]
		expanded := this.varExpanded[path]
		out = append(out, varVisRow{
			row:        r,
			path:       path,
			depth:      depth,
			expandable: r.HasChildren || loaded,
			expanded:   expanded,
		})
		if expanded && len(kids) > 0 {
			out = this.appendVarRows(out, kids, path, depth+1)
		}
	}
	return out
}

// toggleVarPath opens or closes the variable at path. Opening a node whose
// children are not loaded yet fires SigExpandVar so the host fetches them;
// the node is marked open immediately, so the children render as soon as
// SetChildren lands.
func (this *DebugPanel) toggleVarPath(path string) {
	if path == "" {
		return
	}
	if this.varExpanded[path] {
		delete(this.varExpanded, path)
		this.visDirty = true
		this.Self().Update()
		return
	}
	if this.varExpanded == nil {
		this.varExpanded = make(map[string]bool)
	}
	this.varExpanded[path] = true
	this.visDirty = true
	this.Self().Update()
	if _, loaded := this.varChildren[path]; !loaded && this.cbExpandVar != nil {
		this.cbExpandVar(path)
	}
}

// Clear empties the stop-scoped state — call stack, variables, goroutines —
// and blanks every watch VALUE while KEEPING the watch
// expressions: a real debugger keeps your watches across stops and only
// re-evaluates them. The evaluation context falls back to (-1, 0) without
// firing SigContextChanged (a host-driven reset, not a user pick). The
// breakpoint table and the console transcript survive — neither is tied to a
// single stop. Use ClearAll when the session ends entirely.
func (this *DebugPanel) Clear() {
	this.frames = nil
	this.selected = 0
	this.selGoroutine = -1
	this.stackScrollY = 0
	this.varScrollY = 0
	this.hoverStack = -1
	this.hoverVar = -1
	this.lastClickIdx = -1
	// Variables (both groups) and their lazily loaded children are scoped to
	// the stop: drop them, and close any in-progress value edit.
	this.args = nil
	this.locals = nil
	this.resetVarTree()
	// Goroutines are stop-scoped like the stack: drop them on continue.
	this.goroutines = nil
	this.goroScrollY = 0
	this.hoverGoro = -1
	// Keep the watched expressions; blank only their evaluated fields so the
	// stale values don't linger while the program runs.
	for i := range this.watches {
		this.watches[i].Value = ""
		this.watches[i].Type = ""
		this.watches[i].Err = ""
	}
	this.Self().Update()
}

// ClearAll resets the panel to empty: everything Clear drops, plus every watch
// expression, the console transcript, and the in-progress inputs. The host
// calls this when the debug session ends entirely, so there is no session to
// keep watches or output for. The breakpoint table stays — breakpoints outlive
// a session the same way the editor gutter marks do.
func (this *DebugPanel) ClearAll() {
	this.Clear()
	this.watches = nil
	this.watchScrollY = 0
	this.watchInput = ""
	this.watchFocused = false
	this.hoverWatch = -1
	this.consoleLines = nil
	this.consoleScrollY = 0
	this.consoleInput = ""
	this.consoleFocused = false
	this.Self().Update()
}

// CallStack returns a defensive copy of the call-stack frames in top-down
// order. A copy keeps the host from mutating the panel's backing slice.
func (this *DebugPanel) CallStack() []core.StackFrame {
	out := make([]core.StackFrame, len(this.frames))
	copy(out, this.frames)
	return out
}

// Variables returns the Locals group's root rows in dlv's flat projection —
// the inverse of SetVariables. Nested children and the Arguments group are not
// included; use Locals / Arguments for the full rows.
func (this *DebugPanel) Variables() []core.Variable {
	out := make([]core.Variable, len(this.locals))
	for i, r := range this.locals {
		out[i] = core.Variable{Name: r.Name, Type: r.Type, Value: r.Value}
	}
	return out
}

// SetGoroutines replaces the goroutine rows with a defensive copy, resets the
// band's scroll, and repaints. The host fills this by calling dlv
// ListGoroutines at a stop and pushing the result; the panel only renders it.
func (this *DebugPanel) SetGoroutines(gs []core.Goroutine) {
	out := make([]core.Goroutine, len(gs))
	copy(out, gs)
	this.goroutines = out
	this.goroScrollY = 0
	this.hoverGoro = -1
	this.Self().Update()
}

// Goroutines returns a defensive copy of the goroutine rows in display order.
func (this *DebugPanel) Goroutines() []core.Goroutine {
	out := make([]core.Goroutine, len(this.goroutines))
	copy(out, this.goroutines)
	return out
}

// SelectedFrame returns the index of the highlighted stack frame, default
// 0 (the top frame). This is the frame whose variables the panel shows and
// the frame half of the evaluation context.
func (this *DebugPanel) SelectedFrame() int {
	return this.selected
}

// SelectedGoroutine returns the dlv goroutine id every evaluation should run
// in, -1 meaning "the current/selected goroutine" (dlv's own default).
func (this *DebugPanel) SelectedGoroutine() int64 {
	return this.selGoroutine
}

// SetSelectedGoroutine switches the evaluation context to goroutine id and
// resets the frame to the top one, firing SigContextChanged when that is a
// real change. Pass -1 for "whatever goroutine dlv has selected". No
// validation against the goroutine rows: the host may set the context before
// pushing them.
func (this *DebugPanel) SetSelectedGoroutine(id int64) {
	this.setContext(id, 0)
}

// SetSelectedFrame switches the evaluation context to frame index i of the
// current goroutine (0 = top frame), firing SigContextChanged when that is a
// real change. Out-of-range indices are ignored; with no call stack pushed
// yet only 0 is accepted.
func (this *DebugPanel) SetSelectedFrame(i int) {
	if i < 0 {
		return
	}
	if len(this.frames) == 0 {
		if i != 0 {
			return
		}
	} else if i >= len(this.frames) {
		return
	}
	this.setContext(this.selGoroutine, i)
}

// SigContextChanged registers the callback fired whenever the (goroutine,
// frame) evaluation context changes — a stack-row click, a keyboard move, a
// goroutine-row click, or an explicit SetSelectedGoroutine / SetSelectedFrame.
// The host re-runs ListLocals / Eval in that scope, instead of the fixed
// (-1, 0) it used before, and pushes the results back.
func (this *DebugPanel) SigContextChanged(fn func(goroutineID int64, frame int)) {
	this.cbContextChanged = fn
}

// setContext stores a new (goroutine, frame) pair and fires SigContextChanged
// when it actually moved. A no-op for an unchanged context, so re-clicking the
// selected row does not make the host re-evaluate everything.
func (this *DebugPanel) setContext(gid int64, frame int) {
	if this.selGoroutine == gid && this.selected == frame {
		return
	}
	this.selGoroutine = gid
	this.selected = frame
	this.Self().Update()
	if this.cbContextChanged != nil {
		this.cbContextChanged(gid, frame)
	}
}

// SigFrameSelected registers the callback fired when the user clicks a
// stack-frame row. The host re-fetches locals for that frame and calls
// SetVariables. The callback receives a copy of the frame so the host can
// hold onto it past a later Clear without aliasing the panel's slice.
// SigContextChanged carries the same news in scope form; wire one of the two.
func (this *DebugPanel) SigFrameSelected(fn func(index int, frame core.StackFrame)) {
	this.cbFrameSelected = fn
}

// SigFrameActivated registers the callback fired when the user
// double-clicks a stack-frame row or presses Enter on it. The host opens
// frame.File:frame.Line in the editor.
func (this *DebugPanel) SigFrameActivated(fn func(frame core.StackFrame)) {
	this.cbFrameActivated = fn
}

// SigGoroutineActivated registers the callback fired when the user clicks a
// goroutine row. The host opens the goroutine's File:Line in the editor. The
// click also switches the evaluation context to that goroutine's top frame,
// which reports itself through SigContextChanged. The callback receives a copy
// so the host can hold onto it past a later Clear without aliasing the panel's
// slice.
func (this *DebugPanel) SigGoroutineActivated(fn func(g core.Goroutine)) {
	this.cbGoroutineActivated = fn
}

// SigVariableEdited registers the callback fired when the user submits a new
// value for a variable through the inline editor. The first argument is the
// row's dlv-evaluable path — the plain name for a root local or argument,
// "p.Field" / "s[0]" for a nested row — so the host can apply it with dlv
// SetVariable(path, newValue) in the current context, then re-fetch the
// variables and push them back via SetVariables / SetLocals (host-driven,
// like the rest of the panel — it never calls dlv itself).
func (this *DebugPanel) SigVariableEdited(fn func(name string, newValue string)) {
	this.cbVariableEdited = fn
}

// SetBreakpoints replaces the breakpoint table with a defensive copy and
// repaints. The host builds the rows from dlv ListBreakpoints() plus its own
// enabled/condition bookkeeping. An empty list collapses the band entirely, so
// a panel that has never seen a breakpoint looks exactly like it did before
// the table existed.
func (this *DebugPanel) SetBreakpoints(rows []BreakpointRow) {
	out := make([]BreakpointRow, len(rows))
	copy(out, rows)
	this.breakpoints = out
	this.bpScrollY = 0
	this.hoverBP = -1
	this.editingCond = -1
	this.condInput = ""
	this.lastBPClickIdx = -1
	this.Self().Update()
}

// Breakpoints returns a defensive copy of the breakpoint rows in display
// order.
func (this *DebugPanel) Breakpoints() []BreakpointRow {
	out := make([]BreakpointRow, len(this.breakpoints))
	copy(out, this.breakpoints)
	return out
}

// SigToggleBreakpoint registers the callback fired when the enabled checkbox
// of a breakpoint row is clicked. enabled is the REQUESTED new state, so the
// host does not have to read the row back. The panel flips its own row right
// away for responsiveness (like a watch remove); the host applies it in dlv
// and may re-push the table.
func (this *DebugPanel) SigToggleBreakpoint(fn func(file string, line int, enabled bool)) {
	this.cbToggleBreakpoint = fn
}

// SigEditCondition registers the callback fired when a breakpoint condition is
// submitted in the inline editor. An EMPTY cond means "clear the condition" —
// unlike a watch or a variable edit, a blank submit is meaningful here and
// still fires. The panel does not apply the condition itself: that needs a dlv
// Amend, so the host applies it and re-pushes via SetBreakpoints.
func (this *DebugPanel) SigEditCondition(fn func(file string, line int, cond string)) {
	this.cbEditCondition = fn
}

// SigDeleteBreakpoint registers the callback fired when the ✕ affordance of a
// breakpoint row is clicked. The panel drops the row immediately (a direct
// manipulation of what is shown) and the host clears it in dlv and in the
// editor gutter.
func (this *DebugPanel) SigDeleteBreakpoint(fn func(file string, line int)) {
	this.cbDeleteBreakpoint = fn
}

// SetWatches replaces the watched expressions and their evaluated results
// with a defensive copy, then repaints. The host builds this list by
// evaluating every WatchExprs() entry (dlv Eval) at the current stop and
// pushing the results — Value/Type set on success, Err set on failure.
func (this *DebugPanel) SetWatches(w []WatchEntry) {
	out := make([]WatchEntry, len(w))
	copy(out, w)
	this.watches = out
	this.watchScrollY = 0
	this.hoverWatch = -1
	this.Self().Update()
}

// Watches returns a defensive copy of the watched expressions and their
// last evaluations, in display order.
func (this *DebugPanel) Watches() []WatchEntry {
	out := make([]WatchEntry, len(this.watches))
	copy(out, this.watches)
	return out
}

// WatchExprs returns just the watched expression strings, in display order.
// The host re-evaluates all of them on each stop and pushes the results
// back via SetWatches.
func (this *DebugPanel) WatchExprs() []string {
	out := make([]string, len(this.watches))
	for i, w := range this.watches {
		out[i] = w.Expr
	}
	return out
}

// SigWatchAdded registers the callback fired when the user submits a new
// expression in the watch input. The host evaluates it and pushes the
// refreshed list back via SetWatches.
func (this *DebugPanel) SigWatchAdded(fn func(expr string)) {
	this.cbWatchAdded = fn
}

// SigWatchRemoved registers the callback fired when the user removes a
// watch row (the ✕ affordance) or the host calls RemoveWatch. The host
// stops tracking that expression.
func (this *DebugPanel) SigWatchRemoved(fn func(expr string)) {
	this.cbWatchRemoved = fn
}

// RemoveWatch removes the first watch whose expression equals expr and
// fires SigWatchRemoved. It is the host-callable form of the row ✕ button
// so a host UI (or a test) can drop a watch without a click; a no-op when
// expr is not currently watched.
func (this *DebugPanel) RemoveWatch(expr string) {
	for i, w := range this.watches {
		if w.Expr == expr {
			this.removeWatchAt(i)
			return
		}
	}
}

// SetConsoleLines replaces the debug console transcript with a defensive copy,
// pins the view to the newest line, and shows the console band when there is
// anything to show. The host owns the transcript: it echoes the submitted
// command, appends the evaluation result (or the error), and pushes the whole
// list back — the panel never formats or evaluates anything itself.
func (this *DebugPanel) SetConsoleLines(lines []string) {
	out := make([]string, len(lines))
	copy(out, lines)
	this.consoleLines = out
	if len(out) > 0 {
		this.consoleVisible = true
	}
	// Pin to the tail: a console shows its newest output.
	b := this.bands()
	contentH := float64(len(out)) * this.rowHeight
	this.consoleScrollY = clampScroll(contentH, contentH, this.consoleViewH(b))
	this.Self().Update()
}

// ConsoleLines returns a defensive copy of the console transcript.
func (this *DebugPanel) ConsoleLines() []string {
	out := make([]string, len(this.consoleLines))
	copy(out, this.consoleLines)
	return out
}

// SetConsoleVisible shows or hides the console band. Hidden is the default so
// the panel keeps its original layout until a host wires the console up;
// SetConsoleLines shows it implicitly when it pushes output.
func (this *DebugPanel) SetConsoleVisible(on bool) {
	if this.consoleVisible == on {
		return
	}
	this.consoleVisible = on
	if !on {
		this.consoleFocused = false
	}
	this.Self().Update()
}

// ConsoleVisible reports whether the console band currently takes space.
func (this *DebugPanel) ConsoleVisible() bool {
	return this.consoleVisible
}

// SigEval registers the callback fired when the user submits an expression at
// the console prompt. The host evaluates it in the current (goroutine, frame)
// context and appends the command and its result to the transcript via
// SetConsoleLines. A blank submit is ignored.
func (this *DebugPanel) SigEval(fn func(expr string)) {
	this.cbEval = fn
}

// truncateValue shortens a variable value to at most max runes, replacing
// the tail with a single-character ellipsis when it overflows. A value
// that is already within max comes back unchanged; max <= 0 yields "".
// Kept as a free function so the truncation rule is pure and testable
// without a widget or GL context.
func truncateValue(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	// Reserve the last column for the ellipsis so the result is exactly
	// max runes wide.
	return string(r[:max-1]) + "…"
}

// frameRowAtY maps a y coordinate to a stack-frame row index within a
// section whose rows start at topOffset, with count rows of height rowH.
// The caller folds the section's scroll offset into y before calling. It
// returns -1 when y lands above the rows or past the last row. Pure so the
// hit-test is testable without a live widget.
func frameRowAtY(y, topOffset, rowH float64, count int) int {
	if rowH <= 0 || y < topOffset {
		return -1
	}
	idx := int((y - topOffset) / rowH)
	if idx < 0 || idx >= count {
		return -1
	}
	return idx
}

// --- Layout geometry ---

const debugHeaderH = 22.0

// stackBandHeight is the pixel height the call-stack section gets,
// including its header. The variables section takes the rest. The stack
// gets the upper ~45% so a deep stack still leaves room for locals; a
// minimum keeps the header + a couple of rows visible in a short dock.
func (this *DebugPanel) stackBandHeight(totalH float64) float64 {
	h := totalH * 0.45
	min := debugHeaderH + this.rowHeight*2
	if h < min {
		h = min
	}
	if h > totalH-debugHeaderH {
		// Always leave at least the variables header visible.
		h = totalH - debugHeaderH
	}
	if h < debugHeaderH {
		h = debugHeaderH
	}
	return h
}

// watchRemoveW is the width of the ✕ remove hot-zone on the right of a
// watch row: a click past (widget width - watchRemoveW) removes the row.
const watchRemoveW = 20.0

const (
	// debugBPToggleW is the width of the enabled-checkbox hot-zone on the
	// left of a breakpoint row, and the x where its "file:line" starts.
	debugBPToggleW = 18.0
	// debugBPDeleteW is the width of the ✕ delete hot-zone on the right of a
	// breakpoint row.
	debugBPDeleteW = 20.0
	// debugTwistyW is the width of the expand/collapse hot-zone in front of a
	// composite variable, debugIndentW the per-depth indent of the tree.
	debugTwistyW = 12.0
	debugIndentW = 12.0
)

// watchBandHeight is the pixel height reserved at the bottom for the watch
// section: its header, the expression input line, and the watched rows. It
// takes ~30% of the widget with a floor that always keeps the header and
// input line visible, and never exceeds half the widget so the call stack
// and locals keep the upper half.
func (this *DebugPanel) watchBandHeight(totalH float64) float64 {
	h := totalH * 0.3
	min := debugHeaderH + this.rowHeight // header + input line
	if h < min {
		h = min
	}
	if max := totalH * 0.5; h > max {
		h = max
	}
	return h
}

// goroutineBandHeight sizes the goroutines band, carved from the bottom of
// the middle region (everything between the stack and watch bands). It takes
// ~40% of that region with a floor of a header + two rows, capped so the
// locals band above always keeps at least its own header visible.
func (this *DebugPanel) goroutineBandHeight(midH float64) float64 {
	h := midH * 0.4
	if min := debugHeaderH + this.rowHeight*2; h < min {
		h = min
	}
	if max := midH - debugHeaderH; h > max {
		h = max
	}
	if h < debugHeaderH {
		h = debugHeaderH
	}
	return h
}

// breakpointBandHeight sizes the breakpoints band at the very top. It is 0
// when there is no breakpoint — the band takes no space at all, so a panel
// without breakpoints keeps the original four-band geometry. Otherwise it is
// exactly as tall as its rows need, capped at ~30% of the widget so a long
// list cannot crowd out the stack.
func (this *DebugPanel) breakpointBandHeight(totalH float64) float64 {
	if len(this.breakpoints) == 0 {
		return 0
	}
	h := debugHeaderH + float64(len(this.breakpoints))*this.rowHeight
	if max := totalH * 0.3; h > max {
		h = max
	}
	if min := debugHeaderH + this.rowHeight; h < min {
		h = min
	}
	return h
}

// consoleBandHeight sizes the debug console band at the very bottom: 0 while
// the console is hidden (the default), otherwise ~25% of the widget with a
// floor that always keeps its header and prompt line visible.
func (this *DebugPanel) consoleBandHeight(totalH float64) float64 {
	if !this.consoleVisible {
		return 0
	}
	h := totalH * 0.25
	if min := debugHeaderH + this.rowHeight; h < min {
		h = min
	}
	return h
}

// debugBands holds the y bounds of every band for the current widget height,
// top to bottom: breakpoints, call stack, variables, goroutines, watch,
// console. An absent band has top == bottom. Kept in one place so Draw and
// every hit-test agree on the boundaries.
type debugBands struct {
	bpTop, bpBottom           float64
	stackTop, stackBottom     float64
	varTop, varBottom         float64
	goroTop, goroBottom       float64
	watchTop, watchBottom     float64
	consoleTop, consoleBottom float64
}

// bands computes the band split for the current widget height. The optional
// breakpoints and console bands are carved off the top and bottom first; the
// four core bands then split what is left with the same math they always used,
// so with neither optional band present the geometry is unchanged.
func (this *DebugPanel) bands() debugBands {
	_, h := this.Size()

	bpH := this.breakpointBandHeight(h)
	conH := this.consoleBandHeight(h)
	// The two optional bands never take more than 60% together, so the core
	// four always keep the middle of the widget (and a degenerate height can
	// never make the middle region negative).
	if maxAux := h * 0.6; bpH+conH > maxAux {
		if total := bpH + conH; total > 0 {
			scale := maxAux / total
			if scale < 0 {
				scale = 0
			}
			bpH *= scale
			conH *= scale
		}
	}
	inner := h - bpH - conH
	if inner < 0 {
		inner = 0
	}

	watchRel := inner - this.watchBandHeight(inner)
	stackRel := this.stackBandHeight(watchRel)
	varBottomRel := watchRel - this.goroutineBandHeight(watchRel-stackRel)
	if varBottomRel < stackRel {
		varBottomRel = stackRel
	}

	var b debugBands
	b.bpTop, b.bpBottom = 0, bpH
	b.stackTop, b.stackBottom = bpH, bpH+stackRel
	b.varTop, b.varBottom = b.stackBottom, bpH+varBottomRel
	b.goroTop, b.goroBottom = b.varBottom, bpH+watchRel
	b.watchTop, b.watchBottom = b.goroBottom, bpH+inner
	b.consoleTop, b.consoleBottom = b.watchBottom, h
	return b
}

// consoleInputY is the y of the console prompt line: the last row of the
// console band, never above its header.
func (this *DebugPanel) consoleInputY(b debugBands) float64 {
	y := b.consoleBottom - this.rowHeight
	if min := b.consoleTop + debugHeaderH; y < min {
		y = min
	}
	return y
}

// consoleViewH is the height available to console output lines: the band
// minus its header and its prompt line.
func (this *DebugPanel) consoleViewH(b debugBands) float64 {
	return this.consoleInputY(b) - (b.consoleTop + debugHeaderH)
}

// --- Drawing ---

// Draw paints the stacked sections top to bottom: breakpoints (when any), the
// call stack, the variables tree, the goroutines, the watch section, and the
// debug console (when shown), each with a counted header and its own scrolled
// row list.
func (this *DebugPanel) Draw(g paint.Painter) {
	w, h := this.Size()

	// Dark background, matching the sibling panes (log/problems).
	g.SetBrush1(paint.Color{R: 25, G: 25, B: 30, A: 255})
	g.Rectangle(0, 0, w, h)
	g.Fill()

	font := paint.NewFont("Menlo", 12, false, false)
	g.SetFont(font)

	b := this.bands()
	this.drawBreakpointSection(g, font, w, b.bpTop, b.bpBottom-b.bpTop)
	this.drawStackSection(g, font, w, b.stackTop, b.stackBottom-b.stackTop)
	this.drawVarSection(g, font, w, b.varTop, b.varBottom-b.varTop)
	this.drawGoroutineSection(g, font, w, b.goroTop, b.goroBottom-b.goroTop)
	this.drawWatchSection(g, font, w, b.watchTop, b.watchBottom-b.watchTop)
	this.drawConsoleSection(g, font, w, b.consoleTop, b.consoleBottom-b.consoleTop)
}

// drawBreakpointSection paints the breakpoints table at y=top: a header with
// the count, then one row per breakpoint — the enabled marker on the left
// (filled when armed and bound, hollow otherwise), "file:line", the condition
// (or the inline condition editor on the edited row), the hit count
// right-aligned, and a ✕ delete affordance on hover.
func (this *DebugPanel) drawBreakpointSection(g paint.Painter, font paint.Font, w, top, bandH float64) {
	if bandH <= 0 {
		return
	}
	fe := font.FontExtents()

	// Header band, drawn at the section's top.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, top, w, debugHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, top+fe.Ascent+4, "断点 / Breakpoints ("+strconv.Itoa(len(this.breakpoints))+")")

	rh := this.rowHeight
	areaTop := top + debugHeaderH
	startIdx := int(this.bpScrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((bandH-debugHeaderH)/rh) + 2
	bottom := top + bandH

	for i := startIdx; i < startIdx+visibleCount && i < len(this.breakpoints); i++ {
		y := areaTop + float64(i)*rh - this.bpScrollY
		if y+rh <= areaTop || y >= bottom {
			continue
		}
		bp := this.breakpoints[i]

		// Hover wins over the alternating stripe.
		if i == this.hoverBP {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		// Enabled marker: a filled red dot when armed and bound, hollow when
		// disabled or when dlv could not bind the address.
		mark := "○"
		if bp.Enabled {
			mark = "●"
		}
		if bp.Enabled && bp.Verified {
			g.SetBrush1(paint.Color{R: 225, G: 90, B: 90, A: 255})
		} else {
			g.SetBrush1(paint.Color{R: 135, G: 110, B: 110, A: 255})
		}
		g.DrawText1(3, y+fe.Ascent+2, mark)

		// "file:line" in light grey (dimmed when the row is disabled).
		loc := filepath.Base(bp.File) + ":" + strconv.Itoa(bp.Line)
		if bp.Enabled {
			g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
		} else {
			g.SetBrush1(paint.Color{R: 140, G: 140, B: 150, A: 255})
		}
		g.DrawText1(debugBPToggleW, y+fe.Ascent+2, loc)
		x := debugBPToggleW + font.TextExtents(loc).Width + 8

		// Condition column: the inline editor on the edited row, else the
		// condition text when there is one.
		if i == this.editingCond {
			g.SetBrush1(paint.Color{R: 40, G: 48, B: 60, A: 255})
			g.Rectangle(x, y, w-x-debugBPDeleteW, rh)
			g.Fill()
			g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
			g.DrawText1(x+4, y+fe.Ascent+2, this.condInput)
			cx := x + 4 + font.TextExtents(this.condInput).Width + 1
			g.SetBrush1(paint.Color{R: 150, G: 190, B: 240, A: 255})
			g.Rectangle(cx, y+3, 1.5, rh-6)
			g.Fill()
		} else if bp.Cond != "" {
			g.SetBrush1(paint.Color{R: 195, G: 175, B: 120, A: 255})
			g.DrawText1(x, y+fe.Ascent+2, "when "+truncateValue(bp.Cond, 40))
		}

		// Hit count, right-aligned just left of the ✕ zone.
		if bp.HitCount > 0 && i != this.editingCond {
			hc := "× " + strconv.FormatUint(bp.HitCount, 10)
			ext := font.TextExtents(hc)
			g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
			g.DrawText1(w-debugBPDeleteW-ext.Width-4, y+fe.Ascent+2, hc)
		}

		// ✕ delete affordance on the right, shown on hover.
		if i == this.hoverBP {
			g.SetBrush1(paint.Color{R: 200, G: 130, B: 130, A: 255})
			g.DrawText1(w-debugBPDeleteW+4, y+fe.Ascent+2, "✕")
		}
	}
}

// drawStackSection paints the call-stack band at y=top: a header with the
// frame count, then one row per frame (Function left, file:line dimmed/right).
func (this *DebugPanel) drawStackSection(g paint.Painter, font paint.Font, w, top, bandH float64) {
	fe := font.FontExtents()

	// Header band, drawn at the section's top.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, top, w, debugHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, top+fe.Ascent+4, "调用栈 / Call Stack ("+strconv.Itoa(len(this.frames))+")")

	if len(this.frames) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := top + debugHeaderH
	startIdx := int(this.stackScrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((bandH-debugHeaderH)/rh) + 2
	bottom := top + bandH

	for i := startIdx; i < startIdx+visibleCount && i < len(this.frames); i++ {
		y := areaTop + float64(i)*rh - this.stackScrollY
		if y+rh <= areaTop || y >= bottom {
			continue
		}
		fr := this.frames[i]

		// Selection wins over hover wins over the alternating stripe.
		if i == this.selected {
			g.SetBrush1(paint.Color{R: 55, G: 70, B: 95, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i == this.hoverStack {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		// Function name on the left, in light grey.
		g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
		g.DrawText1(8, y+fe.Ascent+2, fr.Function)

		// "file:line" right-aligned in muted blue-grey.
		loc := filepath.Base(fr.File) + ":" + strconv.Itoa(fr.Line)
		locExt := font.TextExtents(loc)
		g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
		g.DrawText1(w-locExt.Width-8, y+fe.Ascent+2, loc)
	}
}

// drawVarSection paints the variables band starting at y=top: a header with
// the root count, then the flattened tree — the "Arguments" / "Locals" group
// headers when there are arguments, a twisty in front of every composite, and
// one row per variable as "Name  Type  = Value" (or the inline value editor on
// the edited row).
func (this *DebugPanel) drawVarSection(g paint.Painter, font paint.Font, w, top, bandH float64) {
	fe := font.FontExtents()

	// Header band, drawn at the section's top.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, top, w, debugHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, top+fe.Ascent+4, "变量 / Variables ("+strconv.Itoa(len(this.args)+len(this.locals))+")")

	rows := this.visRows()
	if len(rows) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := top + debugHeaderH
	startIdx := int(this.varScrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((bandH-debugHeaderH)/rh) + 2
	bottom := top + bandH

	for i := startIdx; i < startIdx+visibleCount && i < len(rows); i++ {
		y := areaTop + float64(i)*rh - this.varScrollY
		if y+rh <= areaTop || y >= bottom {
			continue
		}
		vr := rows[i]

		// Hover wins over the alternating stripe.
		if i == this.hoverVar {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		// Group header row: just the label, in muted blue-grey.
		if vr.isHeader() {
			g.SetBrush1(paint.Color{R: 150, G: 165, B: 185, A: 255})
			g.DrawText1(8, y+fe.Ascent+2, vr.label)
			continue
		}

		x := this.twistyX(vr.depth)
		if vr.expandable {
			mark := "▸"
			if vr.expanded {
				mark = "▾"
			}
			g.SetBrush1(paint.Color{R: 150, G: 190, B: 240, A: 255})
			g.DrawText1(x, y+fe.Ascent+2, mark)
		}
		x += debugTwistyW

		// Name in accent blue.
		g.SetBrush1(paint.Color{R: 120, G: 170, B: 230, A: 255})
		g.DrawText1(x, y+fe.Ascent+2, vr.row.Name)
		x += font.TextExtents(vr.row.Name).Width + 8

		// Type, dimmed.
		if vr.row.Type != "" {
			g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
			g.DrawText1(x, y+fe.Ascent+2, vr.row.Type)
			x += font.TextExtents(vr.row.Type).Width + 8
		}

		// The value column: an inline editor when this row is being edited,
		// otherwise the (truncated) value so a huge struct dump can't run off
		// the row. The editor mirrors the watch input — a focused text line
		// with a caret; only the value is editable, Name/Type stay put.
		if i == this.editingVar {
			g.SetBrush1(paint.Color{R: 40, G: 48, B: 60, A: 255})
			g.Rectangle(x, y, w-x, rh)
			g.Fill()
			g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
			g.DrawText1(x+4, y+fe.Ascent+2, this.varInput)
			cx := x + 4 + font.TextExtents(this.varInput).Width + 1
			g.SetBrush1(paint.Color{R: 150, G: 190, B: 240, A: 255})
			g.Rectangle(cx, y+3, 1.5, rh-6)
			g.Fill()
		} else {
			g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
			g.DrawText1(x, y+fe.Ascent+2, "= "+truncateValue(vr.row.Value, 80))
		}
	}
}

// twistyX is the x of the expand/collapse marker for a row at tree depth d.
func (this *DebugPanel) twistyX(d int) float64 {
	return 8 + float64(d)*debugIndentW
}

// drawGoroutineSection paints the goroutines band at y=top: a header with the
// goroutine count, then one row per goroutine as "#ID Function (file:line)":
// the id in accent blue, the function in light grey, the location dimmed and
// right-aligned. The goroutine holding the evaluation context is highlighted
// like the selected stack frame. Alternating stripe + hover, own scroll.
func (this *DebugPanel) drawGoroutineSection(g paint.Painter, font paint.Font, w, top, bandH float64) {
	fe := font.FontExtents()

	// Header band, drawn at the section's top.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, top, w, debugHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, top+fe.Ascent+4, "协程 / Goroutines ("+strconv.Itoa(len(this.goroutines))+")")

	if len(this.goroutines) == 0 {
		return
	}

	rh := this.rowHeight
	areaTop := top + debugHeaderH
	startIdx := int(this.goroScrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((bandH-debugHeaderH)/rh) + 2
	bottom := top + bandH

	for i := startIdx; i < startIdx+visibleCount && i < len(this.goroutines); i++ {
		y := areaTop + float64(i)*rh - this.goroScrollY
		if y+rh <= areaTop || y >= bottom {
			continue
		}
		gr := this.goroutines[i]

		// The context goroutine wins over hover wins over the stripe.
		if int64(gr.ID) == this.selGoroutine {
			g.SetBrush1(paint.Color{R: 55, G: 70, B: 95, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i == this.hoverGoro {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		// "#ID" in accent blue.
		id := "#" + strconv.Itoa(gr.ID)
		g.SetBrush1(paint.Color{R: 120, G: 170, B: 230, A: 255})
		g.DrawText1(8, y+fe.Ascent+2, id)
		x := 8 + font.TextExtents(id).Width + 8

		// Function name, light grey.
		g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
		g.DrawText1(x, y+fe.Ascent+2, gr.Function)

		// "file:line" right-aligned in muted blue-grey.
		loc := filepath.Base(gr.File) + ":" + strconv.Itoa(gr.Line)
		locExt := font.TextExtents(loc)
		g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
		g.DrawText1(w-locExt.Width-8, y+fe.Ascent+2, loc)
	}
}

// drawWatchSection paints the watch band at y=top: a header with the watch
// count, the expression input line (a caret + typed text when focused, a
// dim prompt when empty), then one row per watched expression as
// "expr = value", or "expr: err" in red when the evaluation failed.
func (this *DebugPanel) drawWatchSection(g paint.Painter, font paint.Font, w, top, bandH float64) {
	fe := font.FontExtents()
	rh := this.rowHeight

	// Header band, drawn at the section's top.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, top, w, debugHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, top+fe.Ascent+4, "监视 / Watch ("+strconv.Itoa(len(this.watches))+")")

	// Expression input line, just under the header.
	inputY := top + debugHeaderH
	if this.watchFocused {
		g.SetBrush1(paint.Color{R: 40, G: 48, B: 60, A: 255})
	} else {
		g.SetBrush1(paint.Color{R: 30, G: 30, B: 36, A: 255})
	}
	g.Rectangle(0, inputY, w, rh)
	g.Fill()
	if this.watchInput == "" && !this.watchFocused {
		g.SetBrush1(paint.Color{R: 110, G: 120, B: 135, A: 255})
		g.DrawText1(8, inputY+fe.Ascent+2, "+ 表达式 / add expression")
	} else {
		g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
		g.DrawText1(8, inputY+fe.Ascent+2, this.watchInput)
		if this.watchFocused {
			// Caret at the end of the typed text.
			cx := 8 + font.TextExtents(this.watchInput).Width + 1
			g.SetBrush1(paint.Color{R: 150, G: 190, B: 240, A: 255})
			g.Rectangle(cx, inputY+3, 1.5, rh-6)
			g.Fill()
		}
	}

	if len(this.watches) == 0 {
		return
	}

	rowsTop := inputY + rh
	startIdx := int(this.watchScrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((bandH-debugHeaderH-rh)/rh) + 2
	bottom := top + bandH

	for i := startIdx; i < startIdx+visibleCount && i < len(this.watches); i++ {
		y := rowsTop + float64(i)*rh - this.watchScrollY
		if y+rh <= rowsTop || y >= bottom {
			continue
		}
		e := this.watches[i]

		// Hover wins over the alternating stripe.
		if i == this.hoverWatch {
			g.SetBrush1(paint.Color{R: 50, G: 50, B: 62, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		} else if i%2 == 1 {
			g.SetBrush1(paint.Color{R: 32, G: 32, B: 38, A: 255})
			g.Rectangle(0, y, w, rh)
			g.Fill()
		}

		// Expression in accent blue.
		g.SetBrush1(paint.Color{R: 120, G: 170, B: 230, A: 255})
		g.DrawText1(8, y+fe.Ascent+2, e.Expr)
		x := 8 + font.TextExtents(e.Expr).Width + 8

		if e.Err != "" {
			// Failed eval: the error in red, prefixed with a colon.
			g.SetBrush1(paint.Color{R: 230, G: 110, B: 110, A: 255})
			g.DrawText1(x, y+fe.Ascent+2, ": "+truncateValue(e.Err, 60))
		} else {
			// Type, dimmed, then "= Value" truncated so a big dump can't run
			// off the row.
			if e.Type != "" {
				g.SetBrush1(paint.Color{R: 130, G: 145, B: 165, A: 255})
				g.DrawText1(x, y+fe.Ascent+2, e.Type)
				x += font.TextExtents(e.Type).Width + 8
			}
			g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
			g.DrawText1(x, y+fe.Ascent+2, "= "+truncateValue(e.Value, 60))
		}

		// ✕ remove affordance on the right, shown on hover.
		if i == this.hoverWatch {
			g.SetBrush1(paint.Color{R: 200, G: 130, B: 130, A: 255})
			g.DrawText1(w-watchRemoveW+4, y+fe.Ascent+2, "✕")
		}
	}
}

// drawConsoleSection paints the debug console band at y=top: a header with the
// line count, the host-pushed transcript, and a "> " prompt line pinned to the
// bottom of the band (a caret + typed text when focused, a dim hint when
// empty).
func (this *DebugPanel) drawConsoleSection(g paint.Painter, font paint.Font, w, top, bandH float64) {
	if bandH <= 0 {
		return
	}
	fe := font.FontExtents()
	rh := this.rowHeight

	// Header band, drawn at the section's top.
	g.SetBrush1(paint.Color{R: 35, G: 35, B: 42, A: 255})
	g.Rectangle(0, top, w, debugHeaderH)
	g.Fill()
	g.SetBrush1(paint.Color{R: 200, G: 200, B: 210, A: 255})
	g.DrawText1(8, top+fe.Ascent+4, "控制台 / Debug Console ("+strconv.Itoa(len(this.consoleLines))+")")

	bottom := top + bandH
	inputY := bottom - rh
	if min := top + debugHeaderH; inputY < min {
		inputY = min
	}

	// Transcript, oldest first, clipped to the region above the prompt.
	areaTop := top + debugHeaderH
	startIdx := int(this.consoleScrollY / rh)
	if startIdx < 0 {
		startIdx = 0
	}
	visibleCount := int((inputY-areaTop)/rh) + 2
	for i := startIdx; i < startIdx+visibleCount && i < len(this.consoleLines); i++ {
		y := areaTop + float64(i)*rh - this.consoleScrollY
		if y+rh <= areaTop || y >= inputY {
			continue
		}
		g.SetBrush1(paint.Color{R: 190, G: 195, B: 205, A: 255})
		g.DrawText1(8, y+fe.Ascent+2, this.consoleLines[i])
	}

	// Prompt line at the bottom of the band.
	if this.consoleFocused {
		g.SetBrush1(paint.Color{R: 40, G: 48, B: 60, A: 255})
	} else {
		g.SetBrush1(paint.Color{R: 30, G: 30, B: 36, A: 255})
	}
	g.Rectangle(0, inputY, w, bottom-inputY)
	g.Fill()
	g.SetBrush1(paint.Color{R: 130, G: 195, B: 140, A: 255})
	g.DrawText1(8, inputY+fe.Ascent+2, ">")
	x := 8 + font.TextExtents(">").Width + 6
	if this.consoleInput == "" && !this.consoleFocused {
		g.SetBrush1(paint.Color{R: 110, G: 120, B: 135, A: 255})
		g.DrawText1(x, inputY+fe.Ascent+2, "表达式 / eval expression")
	} else {
		g.SetBrush1(paint.Color{R: 210, G: 210, B: 220, A: 255})
		g.DrawText1(x, inputY+fe.Ascent+2, this.consoleInput)
		if this.consoleFocused {
			cx := x + font.TextExtents(this.consoleInput).Width + 1
			g.SetBrush1(paint.Color{R: 150, G: 190, B: 240, A: 255})
			g.Rectangle(cx, inputY+3, 1.5, rh-6)
			g.Fill()
		}
	}
}

// --- Events ---

// OnLeftDown routes a click to the section it lands in. Any click first
// cancels an in-progress inline edit (a double-click on a row, handled below,
// re-opens it). The console prompt and the watch input take focus when clicked
// and lose it on any click elsewhere. In the breakpoints table the left marker
// toggles enabled, the right ✕ deletes, and a quick second click in between
// opens the condition editor. A goroutine row click switches the evaluation
// context to that goroutine and fires SigGoroutineActivated. In the call stack
// a click selects the frame (firing SigFrameSelected + SigContextChanged) and a
// quick second click on the same frame activates it. In the variables tree a
// click on a composite's twisty expands/collapses it (requesting children the
// first time) and a quick second click elsewhere on a row opens the inline
// value editor.
func (this *DebugPanel) OnLeftDown(x, y float64) {
	this.SetFocus()

	// A click anywhere first drops any in-progress inline edit; a double-click
	// on a row re-opens one below.
	this.cancelEditVar()
	this.cancelEditCond()

	// Console prompt line: clicking it focuses the console input.
	if this.consoleInputAt(y) {
		this.focusWatchInput(false)
		this.focusConsoleInput(true)
		return
	}
	// Any other click blurs the console input.
	this.focusConsoleInput(false)

	// Watch input line: clicking it focuses the expression editor.
	if this.watchInputAt(y) {
		this.focusWatchInput(true)
		return
	}
	// Any other click blurs the watch input.
	this.focusWatchInput(false)

	// Watch row: the ✕ hot-zone on the right removes the row; elsewhere the
	// row is inert (the value is display-only).
	if wi := this.watchRowAt(y); wi >= 0 {
		w, _ := this.Size()
		if x >= w-watchRemoveW {
			this.removeWatchAt(wi)
		}
		return
	}

	// Breakpoint row: marker zone toggles enabled, ✕ zone deletes, a quick
	// second click in between opens the inline condition editor.
	if bi := this.breakpointRowAt(y); bi >= 0 {
		w, _ := this.Size()
		switch {
		case x < debugBPToggleW:
			this.toggleBreakpointAt(bi)
		case x >= w-debugBPDeleteW:
			this.deleteBreakpointAt(bi)
		default:
			now := time.Now()
			if bi == this.lastBPClickIdx && now.Sub(this.lastBPClickTime) < 400*time.Millisecond {
				this.lastBPClickTime = time.Time{} // reset to avoid triple-click
				this.beginEditCond(bi)
				return
			}
			this.lastBPClickTime = now
			this.lastBPClickIdx = bi
		}
		return
	}

	// Goroutine row: a click switches the context and activates it (the host
	// opens its file:line).
	if gi := this.goroutineRowAt(y); gi >= 0 {
		this.activateGoroutine(gi)
		return
	}

	// Variables row: the twisty zone expands/collapses a composite; a quick
	// second click elsewhere on the row opens the inline value editor (same
	// double-click idiom as the call stack, own timer).
	if vi := this.varRowAt(y); vi >= 0 {
		rows := this.visRows()
		vr := rows[vi]
		if vr.isHeader() {
			return
		}
		if vr.expandable {
			tx := this.twistyX(vr.depth)
			if x >= tx && x < tx+debugTwistyW {
				this.toggleVarPath(vr.path)
				return
			}
		}
		now := time.Now()
		if vi == this.lastVarClickIdx && now.Sub(this.lastVarClickTime) < 400*time.Millisecond {
			this.lastVarClickTime = time.Time{} // reset to avoid triple-click
			this.beginEditVar(vi)
			return
		}
		this.lastVarClickTime = now
		this.lastVarClickIdx = vi
		return
	}

	idx := this.stackRowAt(y)
	if idx < 0 {
		return
	}

	now := time.Now()
	// Double-click detection (same idiom as file-explorer.go).
	if idx == this.lastClickIdx && now.Sub(this.lastClickTime) < 400*time.Millisecond {
		this.lastClickTime = time.Time{} // reset to avoid triple-click
		this.activateFrame(idx)
		return
	}
	this.lastClickTime = now
	this.lastClickIdx = idx

	this.selectFrame(idx)
}

// selectFrame highlights frame idx, moves the evaluation context to it
// (firing SigContextChanged on a real move) and fires SigFrameSelected. The
// latter fires even when idx is already selected so a host wired to it can
// re-pull locals for the same frame.
func (this *DebugPanel) selectFrame(idx int) {
	if idx < 0 || idx >= len(this.frames) {
		return
	}
	this.setContext(this.selGoroutine, idx)
	if this.cbFrameSelected != nil {
		this.cbFrameSelected(idx, this.frames[idx])
	}
}

// activateFrame fires SigFrameActivated for frame idx (open in editor), after
// moving the evaluation context to it.
func (this *DebugPanel) activateFrame(idx int) {
	if idx < 0 || idx >= len(this.frames) {
		return
	}
	this.setContext(this.selGoroutine, idx)
	if this.cbFrameActivated != nil {
		this.cbFrameActivated(this.frames[idx])
	}
}

// activateGoroutine switches the evaluation context to goroutine idx's top
// frame — every later locals fetch and eval runs there — then fires
// SigGoroutineActivated so the host opens its file:line. A no-op for an
// out-of-range index.
func (this *DebugPanel) activateGoroutine(idx int) {
	if idx < 0 || idx >= len(this.goroutines) {
		return
	}
	gr := this.goroutines[idx]
	this.setContext(int64(gr.ID), 0)
	if this.cbGoroutineActivated != nil {
		this.cbGoroutineActivated(gr)
	}
}

// beginEditVar opens the inline value editor on visible variables row idx,
// seeding it with the row's current Value. Only the Value is editable — Name
// and Type stay as shown. A no-op for an out-of-range index or a group
// header. Blurs the other text fields so only one holds keys at a time.
func (this *DebugPanel) beginEditVar(idx int) {
	rows := this.visRows()
	if idx < 0 || idx >= len(rows) || rows[idx].isHeader() {
		return
	}
	this.focusWatchInput(false)
	this.focusConsoleInput(false)
	this.editingVar = idx
	this.varInput = rows[idx].row.Value
	this.Self().Update()
}

// submitEditVar fires SigVariableEdited(path, newValue) with the edited value
// and leaves edit mode. Like adds in the watch section, the panel does NOT
// mutate its own rows: applying the change needs dlv SetVariable, which only
// the host can do — it then re-fetches the variables and pushes them via
// SetVariables / SetLocals. A blank value is ignored (the edit simply closes).
func (this *DebugPanel) submitEditVar() {
	idx := this.editingVar
	val := strings.TrimSpace(this.varInput)
	this.editingVar = -1
	this.varInput = ""
	this.Self().Update()
	rows := this.visRows()
	if idx < 0 || idx >= len(rows) || rows[idx].isHeader() || val == "" {
		return
	}
	if this.cbVariableEdited != nil {
		this.cbVariableEdited(rows[idx].path, val)
	}
}

// cancelEditVar leaves the inline value editor without firing (Esc, or a
// click elsewhere). A no-op when no row is being edited.
func (this *DebugPanel) cancelEditVar() {
	if this.editingVar < 0 {
		return
	}
	this.editingVar = -1
	this.varInput = ""
	this.Self().Update()
}

// beginEditCond opens the inline condition editor on breakpoint row idx,
// seeding it with the row's current Cond (empty for an unconditional
// breakpoint). A no-op for an out-of-range index.
func (this *DebugPanel) beginEditCond(idx int) {
	if idx < 0 || idx >= len(this.breakpoints) {
		return
	}
	this.focusWatchInput(false)
	this.focusConsoleInput(false)
	this.editingCond = idx
	this.condInput = this.breakpoints[idx].Cond
	this.Self().Update()
}

// submitEditCond fires SigEditCondition(file, line, cond) with the trimmed
// condition and leaves edit mode. An EMPTY condition still fires: clearing the
// condition is exactly what an empty submit means. The panel does not apply it
// locally — that needs a dlv Amend, so the host applies it and re-pushes via
// SetBreakpoints.
func (this *DebugPanel) submitEditCond() {
	idx := this.editingCond
	cond := strings.TrimSpace(this.condInput)
	this.editingCond = -1
	this.condInput = ""
	this.Self().Update()
	if idx < 0 || idx >= len(this.breakpoints) {
		return
	}
	if this.cbEditCondition != nil {
		bp := this.breakpoints[idx]
		this.cbEditCondition(bp.File, bp.Line, cond)
	}
}

// cancelEditCond leaves the inline condition editor without firing (Esc, or a
// click elsewhere). A no-op when no row is being edited.
func (this *DebugPanel) cancelEditCond() {
	if this.editingCond < 0 {
		return
	}
	this.editingCond = -1
	this.condInput = ""
	this.Self().Update()
}

// toggleBreakpointAt flips breakpoint idx's armed state and fires
// SigToggleBreakpoint(file, line, wanted). The row flips locally right away so
// the checkbox answers the click; the host applies it in dlv and may re-push
// the table.
func (this *DebugPanel) toggleBreakpointAt(idx int) {
	if idx < 0 || idx >= len(this.breakpoints) {
		return
	}
	bp := this.breakpoints[idx]
	want := !bp.Enabled
	this.breakpoints[idx].Enabled = want
	this.Self().Update()
	if this.cbToggleBreakpoint != nil {
		this.cbToggleBreakpoint(bp.File, bp.Line, want)
	}
}

// deleteBreakpointAt drops breakpoint idx and fires SigDeleteBreakpoint with
// its location. Like a watch remove this is a direct manipulation of the shown
// rows, so the panel updates its own list immediately for responsiveness; the
// host mirrors the change in dlv and in the editor gutter.
func (this *DebugPanel) deleteBreakpointAt(idx int) {
	if idx < 0 || idx >= len(this.breakpoints) {
		return
	}
	bp := this.breakpoints[idx]
	this.breakpoints = append(this.breakpoints[:idx:idx], this.breakpoints[idx+1:]...)
	this.hoverBP = -1
	this.editingCond = -1
	this.condInput = ""
	this.lastBPClickIdx = -1
	this.Self().Update()
	if this.cbDeleteBreakpoint != nil {
		this.cbDeleteBreakpoint(bp.File, bp.Line)
	}
}

// focusWatchInput sets whether the expression input line holds focus,
// repainting on a change so the caret / placeholder swap.
func (this *DebugPanel) focusWatchInput(on bool) {
	if this.watchFocused == on {
		return
	}
	this.watchFocused = on
	this.Self().Update()
}

// submitWatchInput fires SigWatchAdded with the trimmed expression and
// clears the input line. A blank expression is ignored. The panel does NOT
// append to this.watches itself: adding needs an evaluation only the host
// can do, so the host evaluates the new expression and pushes the full list
// back via SetWatches (host-driven, like the rest of the panel).
func (this *DebugPanel) submitWatchInput() {
	expr := strings.TrimSpace(this.watchInput)
	this.watchInput = ""
	if expr != "" && this.cbWatchAdded != nil {
		this.cbWatchAdded(expr)
	}
	this.Self().Update()
}

// removeWatchAt drops watch idx and fires SigWatchRemoved with its
// expression. Unlike adds, a remove is a direct manipulation of the shown
// rows, so the panel updates its own list immediately for responsiveness;
// the host mirrors the change (and may still re-push via SetWatches).
func (this *DebugPanel) removeWatchAt(idx int) {
	if idx < 0 || idx >= len(this.watches) {
		return
	}
	expr := this.watches[idx].Expr
	this.watches = append(this.watches[:idx:idx], this.watches[idx+1:]...)
	this.hoverWatch = -1
	this.Self().Update()
	if this.cbWatchRemoved != nil {
		this.cbWatchRemoved(expr)
	}
}

// focusConsoleInput sets whether the console prompt holds focus, repainting on
// a change so the caret / hint swap. Focusing it also shows the console band,
// so a host shortcut can drop the user straight at the prompt.
func (this *DebugPanel) focusConsoleInput(on bool) {
	if on && !this.consoleVisible {
		this.consoleVisible = true
		this.Self().Update()
	}
	if this.consoleFocused == on {
		return
	}
	this.consoleFocused = on
	this.Self().Update()
}

// submitConsoleInput fires SigEval with the trimmed expression and clears the
// prompt. A blank submit is ignored. The panel does not echo the command
// itself: the host owns the transcript and pushes the command + its result
// back via SetConsoleLines.
func (this *DebugPanel) submitConsoleInput() {
	expr := strings.TrimSpace(this.consoleInput)
	this.consoleInput = ""
	if expr != "" && this.cbEval != nil {
		this.cbEval(expr)
	}
	this.Self().Update()
}

// OnKeyDown routes keys to whichever inline editor is open — the breakpoint
// condition editor first, then a variable's value editor (Enter submits, Esc
// cancels, Backspace deletes a rune) — then to the watch input and the console
// prompt while one of them holds focus (Enter submits, Esc unfocuses,
// Backspace deletes a rune), otherwise it gives the call stack Qt-style
// keyboard control: Up/Down move the selection (re-firing SigFrameSelected and
// SigContextChanged so the host refreshes in the new scope), Enter activates
// the frame.
func (this *DebugPanel) OnKeyDown(key int, repeat bool) {
	// The breakpoint condition editor takes keys first while open.
	if this.editingCond >= 0 {
		switch key {
		case gui.KeyEnter:
			this.submitEditCond()
		case gui.KeyEsc:
			this.cancelEditCond()
		case gui.KeyBackSpace:
			if r := []rune(this.condInput); len(r) > 0 {
				this.condInput = string(r[:len(r)-1])
				this.Self().Update()
			}
		}
		return
	}

	// The inline value editor takes keys next while a variables row is edited.
	if this.editingVar >= 0 {
		switch key {
		case gui.KeyEnter:
			this.submitEditVar()
		case gui.KeyEsc:
			this.cancelEditVar()
		case gui.KeyBackSpace:
			if r := []rune(this.varInput); len(r) > 0 {
				this.varInput = string(r[:len(r)-1])
				this.Self().Update()
			}
		}
		return
	}

	// The watch expression input takes keys next while it holds focus.
	if this.watchFocused {
		switch key {
		case gui.KeyEnter:
			this.submitWatchInput()
		case gui.KeyEsc:
			this.focusWatchInput(false)
		case gui.KeyBackSpace:
			if r := []rune(this.watchInput); len(r) > 0 {
				this.watchInput = string(r[:len(r)-1])
				this.Self().Update()
			}
		}
		return
	}

	// Then the console prompt.
	if this.consoleFocused {
		switch key {
		case gui.KeyEnter:
			this.submitConsoleInput()
		case gui.KeyEsc:
			this.focusConsoleInput(false)
		case gui.KeyBackSpace:
			if r := []rune(this.consoleInput); len(r) > 0 {
				this.consoleInput = string(r[:len(r)-1])
				this.Self().Update()
			}
		}
		return
	}

	if len(this.frames) == 0 {
		return
	}
	switch key {
	case gui.KeyDown:
		if this.selected < len(this.frames)-1 {
			this.selectFrame(this.selected + 1)
		}
	case gui.KeyUp:
		if this.selected > 0 {
			this.selectFrame(this.selected - 1)
		}
	case gui.KeyEnter:
		this.activateFrame(this.selected)
	}
}

// OnTextInput feeds typed characters into whichever text field is active: the
// breakpoint condition editor, a variable's inline value editor, the watch
// expression input, or the console prompt, in that order. Enter and Backspace
// arrive via OnKeyDown, not here; with no active field, typing is ignored.
func (this *DebugPanel) OnTextInput(s string) {
	if s == "\r" || s == "\n" {
		return
	}
	if this.editingCond >= 0 {
		this.condInput += s
		this.Self().Update()
		return
	}
	if this.editingVar >= 0 {
		this.varInput += s
		this.Self().Update()
		return
	}
	if this.watchFocused {
		this.watchInput += s
		this.Self().Update()
		return
	}
	if this.consoleFocused {
		this.consoleInput += s
		this.Self().Update()
	}
}

// OnMouseMove tracks hover state for whichever section the cursor is over.
func (this *DebugPanel) OnMouseMove(x, y float64) {
	hb := this.breakpointRowAt(y)
	hs := this.stackRowAt(y)
	hv := this.varRowAt(y)
	hg := this.goroutineRowAt(y)
	hw := this.watchRowAt(y)
	if hb != this.hoverBP || hs != this.hoverStack || hv != this.hoverVar ||
		hg != this.hoverGoro || hw != this.hoverWatch {
		this.hoverBP = hb
		this.hoverStack = hs
		this.hoverVar = hv
		this.hoverGoro = hg
		this.hoverWatch = hw
		this.Self().Update()
	}
}

// OnMouseLeave clears every hover highlight.
func (this *DebugPanel) OnMouseLeave() {
	if this.hoverBP != -1 || this.hoverStack != -1 || this.hoverVar != -1 ||
		this.hoverGoro != -1 || this.hoverWatch != -1 {
		this.hoverBP = -1
		this.hoverStack = -1
		this.hoverVar = -1
		this.hoverGoro = -1
		this.hoverWatch = -1
		this.Self().Update()
	}
}

// OnMouseWheel scrolls whichever band the cursor is over. Each band clamps to
// its own content height; the watch view height excludes its header and input
// line, the console view height its header and prompt line.
func (this *DebugPanel) OnMouseWheel(x, y, z float64) {
	b := this.bands()
	step := z * 3 * this.rowHeight

	switch {
	case b.bpBottom > b.bpTop && y < b.bpBottom:
		this.bpScrollY = clampScroll(this.bpScrollY-step,
			float64(len(this.breakpoints))*this.rowHeight, (b.bpBottom-b.bpTop)-debugHeaderH)
	case y < b.stackBottom:
		this.stackScrollY = clampScroll(this.stackScrollY-step,
			float64(len(this.frames))*this.rowHeight, (b.stackBottom-b.stackTop)-debugHeaderH)
	case y < b.varBottom:
		this.varScrollY = clampScroll(this.varScrollY-step,
			float64(len(this.visRows()))*this.rowHeight, (b.varBottom-b.varTop)-debugHeaderH)
	case y < b.goroBottom:
		this.goroScrollY = clampScroll(this.goroScrollY-step,
			float64(len(this.goroutines))*this.rowHeight, (b.goroBottom-b.goroTop)-debugHeaderH)
	case y < b.watchBottom:
		this.watchScrollY = clampScroll(this.watchScrollY-step,
			float64(len(this.watches))*this.rowHeight, (b.watchBottom-b.watchTop)-debugHeaderH-this.rowHeight)
	default:
		this.consoleScrollY = clampScroll(this.consoleScrollY-step,
			float64(len(this.consoleLines))*this.rowHeight, this.consoleViewH(b))
	}
	this.Self().Update()
}

// clampScroll pins a scroll offset into [0, max(0, content-view)].
func clampScroll(scrollY, contentH, viewH float64) float64 {
	if scrollY < 0 {
		return 0
	}
	maxScroll := contentH - viewH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scrollY > maxScroll {
		return maxScroll
	}
	return scrollY
}

// breakpointRowAt maps a y coordinate to a breakpoint-table row index, or -1
// when y is outside that band's rows (its header, another band, or a collapsed
// band).
func (this *DebugPanel) breakpointRowAt(y float64) int {
	b := this.bands()
	if b.bpBottom <= b.bpTop || y < b.bpTop || y >= b.bpBottom {
		return -1
	}
	return frameRowAtY(y+this.bpScrollY, b.bpTop+debugHeaderH, this.rowHeight, len(this.breakpoints))
}

// stackRowAt maps a y coordinate to a call-stack frame index, or -1 when y
// is outside the stack section's rows (header, or another band).
func (this *DebugPanel) stackRowAt(y float64) int {
	b := this.bands()
	if y < b.stackTop || y >= b.stackBottom {
		return -1
	}
	return frameRowAtY(y+this.stackScrollY, b.stackTop+debugHeaderH, this.rowHeight, len(this.frames))
}

// varRowAt maps a y coordinate to an index into the flattened variables rows,
// or -1 when y is outside that section's rows (the stack band above or the
// goroutines band below).
func (this *DebugPanel) varRowAt(y float64) int {
	b := this.bands()
	if y < b.varTop || y >= b.varBottom {
		return -1
	}
	return frameRowAtY(y+this.varScrollY, b.varTop+debugHeaderH, this.rowHeight, len(this.visRows()))
}

// goroutineRowAt maps a y coordinate to a goroutine-section row index, or -1
// when y is outside that section's rows (the variables band above, its own
// header, or the watch band below).
func (this *DebugPanel) goroutineRowAt(y float64) int {
	b := this.bands()
	if y < b.goroTop || y >= b.goroBottom {
		return -1
	}
	return frameRowAtY(y+this.goroScrollY, b.goroTop+debugHeaderH, this.rowHeight, len(this.goroutines))
}

// watchInputAt reports whether y lands on the expression input line (the
// row directly under the watch header).
func (this *DebugPanel) watchInputAt(y float64) bool {
	b := this.bands()
	inputY := b.watchTop + debugHeaderH
	return y >= inputY && y < inputY+this.rowHeight
}

// watchRowAt maps a y coordinate to a watched-expression index, or -1 when
// y is outside the watch rows (header, input line, or below the list). The
// rows start one row below the header (past the input line).
func (this *DebugPanel) watchRowAt(y float64) int {
	b := this.bands()
	if y < b.watchTop || y >= b.watchBottom {
		return -1
	}
	return frameRowAtY(y+this.watchScrollY, b.watchTop+debugHeaderH+this.rowHeight, this.rowHeight, len(this.watches))
}

// consoleInputAt reports whether y lands on the console prompt line (the last
// row of the console band). False while the console is hidden.
func (this *DebugPanel) consoleInputAt(y float64) bool {
	b := this.bands()
	if b.consoleBottom <= b.consoleTop {
		return false
	}
	inputY := this.consoleInputY(b)
	return y >= inputY && y < b.consoleBottom
}

func (this *DebugPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{MinWidth: 200, MinHeight: 120}
}

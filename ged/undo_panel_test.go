package ged

import (
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

// probeCommand is a command that records the order in which the stack ran it,
// so a traversal can be checked command by command rather than only by the
// position it ended on.
type probeCommand struct {
	text string
	log  *[]string
}

func (this *probeCommand) Redo()        { *this.log = append(*this.log, "redo "+this.text) }
func (this *probeCommand) Undo()        { *this.log = append(*this.log, "undo "+this.text) }
func (this *probeCommand) Text() string { return this.text }

// pushProbes pushes one probeCommand per text and returns the shared call log,
// already holding the redo each Push performs.
func pushProbes(stack gui.IUndoStack, texts ...string) *[]string {
	log := new([]string)
	for _, s := range texts {
		stack.Push(&probeCommand{text: s, log: log})
	}
	return log
}

func rowTexts(rows []historyRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Text)
	}
	return out
}

func dimFlags(rows []historyRow) []bool {
	out := make([]bool, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Dimmed)
	}
	return out
}

func eqBools(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestHistoryRowsEmptyStack pins the base row: an untouched document still has
// one reachable position — the state before anything happened — and without a
// row for it nothing can undo back to an empty canvas.
func TestHistoryRowsEmptyStack(t *testing.T) {
	rows, current := historyRows(gui.NewUndoStack("test"))
	if want := []string{"初始状态"}; !eqStrings(rowTexts(rows), want) {
		t.Errorf("rows = %q, want %q", rowTexts(rows), want)
	}
	if current != 0 {
		t.Errorf("current = %d, want 0", current)
	}
	if rows[0].Dimmed {
		t.Error("the base row is the state we are standing on, it must not be dimmed")
	}

	if rows, current := historyRows(nil); rows != nil || current != 0 {
		t.Errorf("historyRows(nil) = %v, %d; want nil, 0", rows, current)
	}
}

// TestHistoryRowsNothingUndone covers the ordinary case: commands listed
// oldest-first under the base row, the position sitting on the newest, and
// nothing dimmed because nothing is redoable.
func TestHistoryRowsNothingUndone(t *testing.T) {
	stack := gui.NewUndoStack("test")
	pushProbes(stack, "放置按钮", "移动按钮", "调整大小")

	rows, current := historyRows(stack)
	want := []string{"初始状态", "放置按钮", "移动按钮", "调整大小"}
	if !eqStrings(rowTexts(rows), want) {
		t.Errorf("rows = %q, want %q", rowTexts(rows), want)
	}
	if current != 3 {
		t.Errorf("current = %d, want 3", current)
	}
	if !eqBools(dimFlags(rows), []bool{false, false, false, false}) {
		t.Errorf("dimmed = %v, want all false", dimFlags(rows))
	}
}

// TestHistoryRowsPartiallyUndone is the state the panel exists to show: two
// commands undone but still redoable. They stay on the list, dimmed, and the
// marked position steps back with them.
func TestHistoryRowsPartiallyUndone(t *testing.T) {
	stack := gui.NewUndoStack("test")
	pushProbes(stack, "放置按钮", "移动按钮", "调整大小")
	stack.Undo()
	stack.Undo()

	rows, current := historyRows(stack)
	want := []string{"初始状态", "放置按钮", "移动按钮", "调整大小"}
	if !eqStrings(rowTexts(rows), want) {
		t.Errorf("rows = %q, want %q", rowTexts(rows), want)
	}
	if current != 1 {
		t.Errorf("current = %d, want 1", current)
	}
	// Command i sits at row i+1, so "at or beyond Current()" is rows 2 and 3.
	if want := []bool{false, false, true, true}; !eqBools(dimFlags(rows), want) {
		t.Errorf("dimmed = %v, want %v", dimFlags(rows), want)
	}
}

// TestHistoryRowsAfterPushTruncatesRedoTail is why the rows are re-read rather
// than accumulated: pushing after an undo drops the whole redo tail, so rows
// the panel has already shown have to disappear.
func TestHistoryRowsAfterPushTruncatesRedoTail(t *testing.T) {
	stack := gui.NewUndoStack("test")
	pushProbes(stack, "放置按钮", "移动按钮", "调整大小")
	stack.Undo()
	stack.Undo()
	pushProbes(stack, "删除按钮")

	rows, current := historyRows(stack)
	want := []string{"初始状态", "放置按钮", "删除按钮"}
	if !eqStrings(rowTexts(rows), want) {
		t.Errorf("rows = %q, want %q", rowTexts(rows), want)
	}
	if current != 2 {
		t.Errorf("current = %d, want 2", current)
	}
	if want := []bool{false, false, false}; !eqBools(dimFlags(rows), want) {
		t.Errorf("dimmed = %v, want %v", dimFlags(rows), want)
	}
}

// TestHistoryRowsLabelsUnnamedCommands keeps a command with no Text() from
// drawing a blank, unclickable-looking row.
func TestHistoryRowsLabelsUnnamedCommands(t *testing.T) {
	stack := gui.NewUndoStack("test")
	pushProbes(stack, "")
	rows, _ := historyRows(stack)
	if want := []string{"初始状态", "(未命名操作)"}; !eqStrings(rowTexts(rows), want) {
		t.Errorf("rows = %q, want %q", rowTexts(rows), want)
	}
}

// TestUndoPanelGoToWalksEveryCommand checks the jump runs each command's own
// Undo/Redo in order instead of teleporting the position: a command that is
// skipped leaves the document out of step with the marked row.
func TestUndoPanelGoToWalksEveryCommand(t *testing.T) {
	p := NewUndoPanel()
	scene := NewGedScene()
	p.SetScene(scene)
	log := pushProbes(scene.UndoStack(), "一", "二", "三")
	*log = (*log)[:0]

	p.GoTo(1)
	if want := []string{"undo 三", "undo 二"}; !eqStrings(*log, want) {
		t.Errorf("GoTo(1) ran %q, want %q", *log, want)
	}
	if got := scene.UndoStack().Current(); got != 1 {
		t.Errorf("Current() = %d, want 1", got)
	}

	*log = (*log)[:0]
	p.GoTo(3)
	if want := []string{"redo 二", "redo 三"}; !eqStrings(*log, want) {
		t.Errorf("GoTo(3) ran %q, want %q", *log, want)
	}
	if got := scene.UndoStack().Current(); got != 3 {
		t.Errorf("Current() = %d, want 3", got)
	}

	// Standing still must not move the document.
	*log = (*log)[:0]
	p.GoTo(3)
	if len(*log) != 0 {
		t.Errorf("GoTo to the current position ran %q, want nothing", *log)
	}

	// Out-of-range clamps to the ends of the stack rather than spinning.
	p.GoTo(-5)
	if got := scene.UndoStack().Current(); got != 0 {
		t.Errorf("GoTo(-5) left Current() = %d, want 0", got)
	}
	p.GoTo(99)
	if got := scene.UndoStack().Current(); got != 3 {
		t.Errorf("GoTo(99) left Current() = %d, want 3", got)
	}
}

// TestUndoPanelClickGoesToThatRowsPosition ties the pixel the user hits to the
// stack position, which is the whole interaction: row i is position i.
func TestUndoPanelClickGoesToThatRowsPosition(t *testing.T) {
	p := NewUndoPanel()
	scene := NewGedScene()
	p.SetScene(scene)
	pushProbes(scene.UndoStack(), "一", "二", "三")
	p.Rebuild()

	// Row centres: the list starts under the 22px header, 24px per row.
	rowY := func(i int) float64 { return 22 + float64(i)*24 + 12 }

	p.OnLeftDown(10, rowY(0))
	if got := scene.UndoStack().Current(); got != 0 {
		t.Errorf("clicking the base row left Current() = %d, want 0", got)
	}
	p.OnLeftDown(10, rowY(2))
	if got := scene.UndoStack().Current(); got != 2 {
		t.Errorf("clicking row 2 left Current() = %d, want 2", got)
	}
	if p.current != 2 {
		t.Errorf("panel marks row %d as current, want 2", p.current)
	}

	// The header and the empty space past the last row are not rows.
	p.OnLeftDown(10, 5)
	p.OnLeftDown(10, rowY(9))
	if got := scene.UndoStack().Current(); got != 2 {
		t.Errorf("a click outside the list moved Current() to %d, want 2", got)
	}
}

// TestUndoPanelSyncFollowsCommandsPushedElsewhere guards the panel against
// going stale. Commands arrive from canvas tools, menus and shortcuts, none of
// which touch the panel, and a move or a resize raises no scene signal at all.
func TestUndoPanelSyncFollowsCommandsPushedElsewhere(t *testing.T) {
	p := NewUndoPanel()
	scene := NewGedScene()
	p.SetScene(scene)
	if len(p.rows) != 1 {
		t.Fatalf("fresh panel shows %d rows, want just the base row", len(p.rows))
	}

	pushProbes(scene.UndoStack(), "移动按钮")
	p.syncFromStack()
	if want := []string{"初始状态", "移动按钮"}; !eqStrings(rowTexts(p.rows), want) {
		t.Errorf("rows = %q, want %q", rowTexts(p.rows), want)
	}
	if p.current != 1 {
		t.Errorf("current = %d, want 1", p.current)
	}

	scene.UndoStack().Undo()
	p.syncFromStack()
	if p.current != 0 {
		t.Errorf("after an undo elsewhere current = %d, want 0", p.current)
	}
	if !p.rows[1].Dimmed {
		t.Error("the undone command's row should be dimmed")
	}
}

// TestUndoPanelDocksAsAToolView pins the registration the dock relies on:
// Dock.AddView claims a view as one of the frame's tool views by looking its
// factory name up in the tool-view registry. Break either and the frame cannot
// tell the panel from a document — CurrentDocView hands the history panel to
// Run, Preview and Save, which then silently do nothing.
func TestUndoPanelDocksAsAToolView(t *testing.T) {
	if _, ok := gui.GetToolViewDef("ged.UndoPanel"); !ok {
		t.Fatal(`no tool view registered under "ged.UndoPanel"`)
	}
	p := NewUndoPanel()
	if got := core.FactoryNameOf(p); got != "ged.UndoPanel" {
		t.Fatalf("factory name = %q, want %q — AddView looks the panel up by this", got, "ged.UndoPanel")
	}

	frame := gui.NewFrame()
	// createPanels syncs the registry into the frame this way before docking
	// any panel; without it AddView has nothing to match against.
	_ = frame.ToolViewActions()
	frame.SuggestDocDock().AddView(p)

	if !frame.IsToolView(p) {
		t.Error("the docked panel is not one of the frame's tool views")
	}
	if view, _ := frame.CurrentDocView(); view != nil {
		t.Errorf("CurrentDocView() = %T, want nil — the panel is not a document", view)
	}
}

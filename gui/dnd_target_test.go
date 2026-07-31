package gui

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// dndProbe is a drop target that records every drag callback it receives and
// answers with a fixed action, so a walk over a widget tree can be asserted as
// an exact call sequence.
type dndProbe struct {
	Widget
	name   string
	accept DndAction // answered from OnDragEnter and OnDragMove
	drop   DndAction // answered from OnDrop
	log    *[]string
}

func newDndProbe(log *[]string, name string, accept, drop DndAction) *dndProbe {
	p := &dndProbe{name: name, accept: accept, drop: drop, log: log}
	p.Init(p)
	return p
}

func (this *dndProbe) record(event string, x, y float64) {
	*this.log = append(*this.log, fmt.Sprintf("%s:%s(%g,%g)", this.name, event, x, y))
}

func (this *dndProbe) OnDragEnter(x, y float64, dnd IDndContext) {
	this.record("enter", x, y)
	dnd.SetAction(this.accept)
}

func (this *dndProbe) OnDragMove(x, y float64, dnd IDndContext) {
	this.record("move", x, y)
	dnd.SetAction(this.accept)
}

func (this *dndProbe) OnDrop(x, y float64, dnd IDndContext) {
	this.record("drop", x, y)
	dnd.SetAction(this.drop)
}

func (this *dndProbe) OnDragLeave() {
	*this.log = append(*this.log, this.name+":leave")
}

// dndPlain is a widget that is not a drop target at all.
type dndPlain struct {
	Widget
}

func newDndPlain() *dndPlain {
	p := new(dndPlain)
	p.Init(p)
	return p
}

// newDndTestContext builds a context whose possible actions cover everything a
// probe can answer. dndContext itself is per-backend, but both declare pa, so
// this compiles against either one.
func newDndTestContext() *dndContext {
	return &dndContext{pa: DndCopy | DndMove | DndLink}
}

func checkDndLog(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("callbacks = %v, want %v", got, want)
	}
}

// TestDndDragOverFallsThroughRejectingChild covers the case the whole walk
// exists for: Dock answers DndIgnore whenever the cursor is near its border so
// that the enclosing Frame can take the drag and dock the view to the frame
// edge. A backend that offers the drag only to the innermost IOnDrop under the
// cursor — which the GLFW loop used to do — makes edge docking impossible,
// because the Dock swallows the drag and then refuses it.
func TestDndDragOverFallsThroughRejectingChild(t *testing.T) {
	var log []string
	parent := newDndProbe(&log, "parent", DndMove, DndMove)
	child := newDndProbe(&log, "child", DndIgnore, DndIgnore)
	child.SetBounds(10, 20, 30, 40)
	child.SetParent(parent)

	ctx := newDndTestContext()
	got := dndDragOver(child, nil, 3, 4, ctx)

	if got != IWidget(parent) {
		t.Fatalf("drag holder = %v, want the parent", got)
	}
	if ctx.Action() != DndMove {
		t.Fatalf("action = %v, want DndMove from the parent", ctx.Action())
	}
	// The parent sees the point in its own coordinates: child origin + offset.
	checkDndLog(t, log, "child:enter(3,4)", "parent:enter(13,24)", "parent:move(13,24)")
}

// TestDndDragOverKeepsTargetWithoutReEnter checks that staying over the widget
// that already holds the drag produces OnDragMove only. Dock and TabBar reset
// their drop hint in OnDragEnter, so re-entering on every mouse position makes
// the split/insert marker flicker instead of tracking the cursor.
func TestDndDragOverKeepsTargetWithoutReEnter(t *testing.T) {
	var log []string
	target := newDndProbe(&log, "target", DndMove, DndMove)

	ctx := newDndTestContext()
	got := dndDragOver(target, target, 5, 6, ctx)

	if got != IWidget(target) {
		t.Fatalf("drag holder = %v, want the same target", got)
	}
	checkDndLog(t, log, "target:move(5,6)")
}

// TestDndDragOverLeavesPreviousTarget checks that the widget losing the drag is
// told exactly once. OnDragLeave is the only place Dock, Frame and TabBar clear
// their drop rectangle, so a missed leave leaves the highlight painted on a
// widget the cursor is no longer over.
func TestDndDragOverLeavesPreviousTarget(t *testing.T) {
	var log []string
	root := newDndPlain()
	old := newDndProbe(&log, "old", DndMove, DndMove)
	fresh := newDndProbe(&log, "fresh", DndCopy, DndCopy)
	old.SetParent(root)
	fresh.SetParent(root)

	ctx := newDndTestContext()
	got := dndDragOver(fresh, old, 1, 2, ctx)

	if got != IWidget(fresh) {
		t.Fatalf("drag holder = %v, want the new target", got)
	}
	if ctx.Action() != DndCopy {
		t.Fatalf("action = %v, want DndCopy", ctx.Action())
	}
	checkDndLog(t, log, "fresh:enter(1,2)", "old:leave", "fresh:move(1,2)")
}

// TestDndDragOverClearsActionWhenNothingAccepts pins the action reset that has
// to happen when the cursor moves off every drop target: the context is reused
// for the whole drag, so without it the feedback cursor keeps offering the
// action the last target accepted while hovering over dead space.
func TestDndDragOverClearsActionWhenNothingAccepts(t *testing.T) {
	var log []string
	old := newDndProbe(&log, "old", DndMove, DndMove)
	plain := newDndPlain()

	ctx := newDndTestContext()
	ctx.SetAction(DndMove)
	got := dndDragOver(plain, old, 1, 2, ctx)

	if got != nil {
		t.Fatalf("drag holder = %v, want nil", got)
	}
	if ctx.Action() != DndIgnore {
		t.Fatalf("action = %v, want DndIgnore once no widget accepts", ctx.Action())
	}
	checkDndLog(t, log, "old:leave")
}

// TestDndDropWalksToFirstWidgetThatTakesIt checks that a drop refused by the
// widget under the cursor is offered to its ancestors, in the coordinates of
// each. Frame and Dock both decline a drop whose payload is not a dock handle,
// and the outer one is what handles the frame-edge case.
func TestDndDropWalksToFirstWidgetThatTakesIt(t *testing.T) {
	var log []string
	parent := newDndProbe(&log, "parent", DndMove, DndMove)
	child := newDndProbe(&log, "child", DndMove, DndIgnore)
	child.SetBounds(7, 9, 30, 40)
	child.SetParent(parent)

	ctx := newDndTestContext()
	ctx.SetAction(DndCopy) // left over from the last OnDragMove
	dndDrop(child, 1, 1, ctx)

	if ctx.Action() != DndMove {
		t.Fatalf("action = %v, want the parent's DndMove", ctx.Action())
	}
	checkDndLog(t, log, "child:drop(1,1)", "parent:drop(8,10)")
}

// TestDndBackendsShareTargetingWalk keeps the two backends on one copy of the
// enter/move/leave/drop walk. They diverged once already — the Win32 side fell
// through to ancestors while the GLFW side stopped at the innermost target —
// and only one of them can be compiled, let alone run, on any given host.
func TestDndBackendsShareTargetingWalk(t *testing.T) {
	for _, file := range []string{"dnd_windows.go", "dnd_glfw.go"} {
		src := readDndSource(t, file)
		for _, call := range []string{"dndDragOver(", "dndDrop("} {
			if !strings.Contains(src, call) {
				t.Errorf("%s does not call %s; it carries its own copy of the drop targeting rules", file, call)
			}
		}
	}
}

// TestDndWindowsDropChecksContextBeforeUse pins the operand order in Drop. OLE
// calls Drop with no live context after a DragLeave, and the guard used to read
// this.dndContext.do before testing this.dndContext for nil, so the check meant
// to reject the call was itself what faulted the process.
func TestDndWindowsDropChecksContextBeforeUse(t *testing.T) {
	body := dndFuncBody(t, "dnd_windows.go", "func (this *Window) Drop(")
	nilTest := strings.Index(body, "this.dndContext == nil")
	deref := strings.Index(body, "this.dndContext.do")
	if nilTest < 0 {
		t.Fatal("dnd_windows.go: Drop does not test this.dndContext for nil")
	}
	if deref >= 0 && deref < nilTest {
		t.Error("dnd_windows.go: Drop dereferences this.dndContext before testing it for nil")
	}
}

// TestDndWindowsOffersLinkEffect covers a link drag arriving as a no-op on
// Windows only: DoDragDrop masked the caller's actions with 0x03, so DndLink
// was stripped and OLE was handed a drag with no allowed effect, while the GLFW
// backend passed the same actions through untouched.
func TestDndWindowsOffersLinkEffect(t *testing.T) {
	body := dndFuncBody(t, "dnd_windows.go", "func (this *Window) DoDragDrop(")
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, "acts :=") {
			continue
		}
		if !strings.Contains(line, "DndLink") {
			t.Errorf("dnd_windows.go: %q drops DndLink from the actions offered to OLE", strings.TrimSpace(line))
		}
		return
	}
	t.Fatal("dnd_windows.go: DoDragDrop no longer computes acts")
}

func readDndSource(t *testing.T, file string) string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("cannot read %s: %v", file, err)
	}
	return string(src)
}

// dndFuncBody returns the text of the function opened by header in file.
func dndFuncBody(t *testing.T, file, header string) string {
	t.Helper()
	src := readDndSource(t, file)
	start := strings.Index(src, header)
	if start < 0 {
		t.Fatalf("%s has no %s", file, header)
	}
	rest := src[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("%s: %s is not terminated", file, header)
	}
	return rest[:end]
}

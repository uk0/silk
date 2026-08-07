package ged

// Every design mutation goes through one door: Scene().PushCommand(cmd), which
// is what raises SigDesignChanged. Six sites used to push onto the undo stack
// directly — the edit landed, was undoable, and nothing said so, leaving the
// 生成代码 view showing the design the user had already replaced.
//
// These cases drive the real entry points (key handler, canvas gestures, the
// history panel) and assert the user-visible pair: the generated file moved AND
// a listener was told. Asserting only that a callback ran would not have caught
// the original defect, because the defect was the code changing in silence.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// doorProbe watches one design through a real CodePanel — the listener the
// stale-view defect was reported against — plus the generated file itself,
// computed independently of any notification.
type doorProbe struct {
	view  *GedView
	scene *GedScene
	panel *CodePanel

	code string // generated file as of arm()
	text string // what the panel was showing as of arm()
}

func newDoorProbe(t *testing.T) *doorProbe {
	t.Helper()
	view := NewGedView()
	scene := view.GedScene()
	scene.SetFormTitle("Door")
	scene.SetSize(200, 150)

	panel := NewCodePanel()
	panel.BindGedView(view)
	panel.ShowTab(codeTabGen)
	return &doorProbe{view: view, scene: scene, panel: panel}
}

func (p *doorProbe) generated() string {
	return p.scene.GenerateCode(CodeGenOptions{PackageName: "main"})
}

// arm records the state the next mutation has to move away from, and clears the
// panel's pending flag so the assertion reads only what that mutation reported.
func (p *doorProbe) arm(t *testing.T) {
	t.Helper()
	p.panel.RegenerateNow()
	p.code = p.generated()
	p.text = p.panel.genView.Text()
}

// expectReported asserts the whole user-visible chain for one mutation.
//
// genPending is read BEFORE RegenerateNow deliberately: regenerating first
// would refresh the view whether or not anything ever reported the change,
// which is precisely the blindness that let the stale-view defect ship.
func (p *doorProbe) expectReported(t *testing.T, what string) {
	t.Helper()
	after := p.generated()
	if after == p.code {
		t.Fatalf("%s left the generated file byte-identical; this case cannot see a missing report", what)
	}
	if !p.panel.genPending {
		t.Errorf("%s changed the generated file but reported nothing — 生成代码 stays on the pre-edit listing", what)
	}
	p.panel.RegenerateNow()
	if got := p.panel.genView.Text(); got == p.text {
		t.Errorf("%s: 生成代码 view still shows the pre-edit listing", what)
	}
}

// TestKeyboardNudgeReportsDesignChanged drives the arrow key through the real
// key handler, the way the reviewer reproduced the stale view.
func TestKeyboardNudgeReportsDesignChanged(t *testing.T) {
	p := newDoorProbe(t)
	btn := addFakeAt(t, p.scene, "btn", 40, 30, 10, 4)
	p.view.Selection().Add(btn)
	p.arm(t)

	p.view.OnKeyDown(gui.KeyRight, false)

	if x, _ := btn.Pos(); x != 41 {
		t.Fatalf("after one KeyRight: x = %g, want 41 — the nudge never ran", x)
	}
	p.expectReported(t, "keyboard nudge")
}

// TestCtrlArrowResizeReportsDesignChanged covers the second site the reviewer
// found. Ctrl cannot be driven through OnKeyDown here — the modifier comes from
// gui.IsKeyDown, which needs a live window — so this calls the beginGesture +
// resizeSelection pair OnKeyDown invokes once it has taken that branch, the
// same workaround ged_view_resize_test.go uses.
func TestCtrlArrowResizeReportsDesignChanged(t *testing.T) {
	p := newDoorProbe(t)
	btn := addFakeAt(t, p.scene, "btn", 40, 30, 10, 4)
	p.view.Selection().Add(btn)
	p.arm(t)

	p.view.beginGesture(false)
	p.view.resizeSelection(1, 0)

	if _, _, w, _ := btn.Bounds(); w != 11 {
		t.Fatalf("after Ctrl+Right: width = %g, want 11 — the resize never ran", w)
	}
	p.expectReported(t, "Ctrl+arrow resize")
}

// TestHandleDragResizeReportsDesignChanged is the mouse resize: press on the
// bottom-right handle and drag. The command comes from graph.ResizeDecor, which
// pushed onto the stack directly and so left the same stale view as the two
// keyboard cases.
func TestHandleDragResizeReportsDesignChanged(t *testing.T) {
	p := newDoorProbe(t)
	btn := addFakeAt(t, p.scene, "btn", 10, 10, 25, 7)
	p.view.Selection().Add(btn)
	p.arm(t)

	// The press point sits on the bottom-right handle disc; the drop lands on
	// bare canvas so no re-parent command can supply the report instead.
	if decor, handle := p.view.FindHandleAt(34.5, 16.5); decor == nil || handle == 0 {
		t.Fatalf("press point is not on a handle (decor=%v handle=%d)", decor, handle)
	}
	dragOnCanvas(p.view, 34.5, 16.5, 60, 40)

	if _, _, w, h := btn.Bounds(); w == 25 && h == 7 {
		t.Fatalf("after the handle drag: size unchanged (%g, %g) — the resize never ran", w, h)
	}
	p.expectReported(t, "resize-handle drag")
}

// TestCanvasDragMoveReportsDesignChanged is the mouse move: graph.MovePart
// commits the drag. The drop lands on empty canvas, so the re-parent command
// GedView.OnLeftUp can raise is not generated and MovePart's own push is the
// only thing that can report the change.
func TestCanvasDragMoveReportsDesignChanged(t *testing.T) {
	p := newDoorProbe(t)
	btn := addFakeAt(t, p.scene, "btn", 10, 10, 25, 7)
	p.view.Selection().Add(btn)
	p.arm(t)

	before := p.scene.UndoStack().Count()
	dragOnCanvas(p.view, 12, 12, 100, 60)

	if got := p.scene.UndoStack().Count() - before; got != 1 {
		t.Fatalf("drag pushed %d commands, want exactly 1 (MovePart's) — another push could be supplying the report", got)
	}
	if x, _ := btn.Pos(); x == 10 {
		t.Fatalf("after the drag: x = %g, want the widget moved", x)
	}
	p.expectReported(t, "canvas drag move")
}

// TestRectToolDragReportsDesignChanged covers graph.RectPart, reached with the
// 矩形工具 active. It adds a graph.RectItem, which the generator does not model,
// so the generated text is unchanged by design — the observable here is the new
// item in the design plus the report, and expectReported is deliberately not
// used because its first assertion would be false.
func TestRectToolDragReportsDesignChanged(t *testing.T) {
	p := newDoorProbe(t)
	var rectTool graph.ITool
	for _, tool := range p.view.Tools() {
		if tool.Name() == "rect-tool" {
			rectTool = tool
		}
	}
	if rectTool == nil {
		t.Fatal("no rect-tool on the view; this case is testing nothing")
	}
	p.view.SetActiveTool(rectTool)
	p.arm(t)

	before := len(p.scene.Children())
	dragOnCanvas(p.view, 20, 20, 80, 60)

	if got := len(p.scene.Children()) - before; got != 1 {
		t.Fatalf("rect drag added %d items, want 1 — the gesture never ran", got)
	}
	if !p.panel.genPending {
		t.Error("rect-tool drag added an item to the design but reported nothing")
	}
}

// TestUndoRedoReportDesignChanged pins the paths that were already correct:
// running a command backwards pushes nothing, so GedView.Undo/Redo report the
// change themselves.
func TestUndoRedoReportDesignChanged(t *testing.T) {
	p := newDoorProbe(t)
	btn := addFakeAt(t, p.scene, "btn", 40, 30, 10, 4)
	p.view.Selection().Add(btn)
	p.view.OnKeyDown(gui.KeyRight, false)
	p.arm(t)

	p.view.Undo()
	if x, _ := btn.Pos(); x != 40 {
		t.Fatalf("after undo: x = %g, want 40", x)
	}
	p.expectReported(t, "undo")

	p.arm(t)
	p.view.Redo()
	if x, _ := btn.Pos(); x != 41 {
		t.Fatalf("after redo: x = %g, want 41", x)
	}
	p.expectReported(t, "redo")
}

// TestUndoPanelGoToReportsDesignChanged: clicking a row in the 撤销历史 panel
// rewinds the stack without pushing anything, so the jump has to report the
// change the same way Ctrl+Z does. A click there can undo a dozen edits at once.
func TestUndoPanelGoToReportsDesignChanged(t *testing.T) {
	p := newDoorProbe(t)
	btn := addFakeAt(t, p.scene, "btn", 40, 30, 10, 4)
	p.view.Selection().Add(btn)
	p.view.OnKeyDown(gui.KeyRight, false)
	p.view.beginGesture(false)
	p.view.resizeSelection(2, 0)

	panel := NewUndoPanel()
	panel.SetScene(p.scene)
	p.arm(t)

	panel.GoTo(0)
	if x, _, w, _ := btn.Bounds(); x != 40 || w != 10 {
		t.Fatalf("after GoTo(0): bounds x=%g w=%g, want x=40 w=10", x, w)
	}
	p.expectReported(t, "undo-history jump")
}

// TestScenePushCommandNotifiesThroughIScene pins the seam the whole fix rests
// on. Every converted call site holds the scene as a graph.IScene, not as a
// *GedScene: a call typed as *graph.SceneItem would run the base method and
// skip the override, leaving the fix inert while still compiling.
func TestScenePushCommandNotifiesThroughIScene(t *testing.T) {
	view := NewGedView()
	scene := view.GedScene()

	fired := 0
	scene.SigDesignChanged(func() { fired++ })

	// The static type graph.MovePart, graph.RectPart, graph.ResizeDecor and
	// GedView all reach the scene through.
	var door graph.IScene = scene
	if _, ok := door.(*GedScene); !ok {
		t.Fatalf("Scene() hands out %T, not a *GedScene — the override cannot be reached", door)
	}

	fake, err := NewFakeWidgetFromFactory("gui.Button")
	if err != nil {
		t.Fatalf("create Button: %v", err)
	}
	fake.SetBounds(5, 5, 20, 6)
	cmd := graph.NewAddCommand()
	cmd.AddItem(fake, scene)
	door.PushCommand(cmd)

	if fired != 1 {
		t.Errorf("PushCommand through graph.IScene reported %d changes, want 1 — the *GedScene override is not on the interface path", fired)
	}
	if fake.Parent() != graph.IItem(scene) {
		t.Errorf("PushCommand through graph.IScene did not run the command: parent = %v", fake.Parent())
	}
}

// doorFile is the one file allowed to touch the undo stack's Push directly:
// graph.SceneItem.PushCommand is the door's implementation, and ged.GedScene
// overrides it to report the change.
const doorFile = "graph/scene-item.go"

// TestNoDirectUndoStackPushOutsideTheDoor walks the repository and fails naming
// any site that pushes onto a scene's undo stack without going through
// PushCommand. Six such sites accumulated before anyone noticed; the shape that
// has actually held here is a guard that reads the source, not a review habit.
func TestNoDirectUndoStackPushOutsideTheDoor(t *testing.T) {
	root := repoRoot(t)

	// Split so this file itself carries no contiguous occurrence of what it
	// searches for — otherwise the guard reports itself.
	needle := "UndoStack()" + ".Push("

	door, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doorFile)))
	if err != nil {
		t.Fatalf("read %s: %v", doorFile, err)
	}
	if !strings.Contains(string(door), needle) {
		t.Fatalf("%s no longer holds the sanctioned push; this guard is now checking nothing", doorFile)
	}

	// Ask git which files are ours rather than maintaining a skip list. A
	// hand-kept list of directories to ignore cannot keep up: this checkout
	// carries .claude/worktrees (51 sibling copies of the whole repo), a
	// Package/ tree and a local/ dir, and the guard reported 350 offenders from
	// them the moment it ran outside a pristine worktree. Version control
	// already answers "which files are silk's source", and it answers it the
	// same way on every machine.
	tracked, err := trackedGoFiles(root)
	if err != nil {
		t.Skipf("cannot list tracked files (%v); this guard needs a git checkout", err)
	}
	if len(tracked) == 0 {
		t.Fatal("git listed no Go files; the guard would pass vacuously")
	}

	var offenders []string
	for _, rel := range tracked {
		if rel == doorFile {
			continue
		}
		src, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if readErr != nil {
			continue
		}
		for i, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, needle) {
				offenders = append(offenders, fmt.Sprintf("  %s:%d: %s", rel, i+1, strings.TrimSpace(line)))
			}
		}
	}

	if len(offenders) > 0 {
		t.Errorf("%d site(s) push onto a scene's undo stack directly:\n%s\n\n"+
			"Scene().PushCommand(cmd) is the one door — it is what tells the 生成代码 view, the\n"+
			"undo panel and every other listener that the design moved. The stack announces nothing,\n"+
			"so a direct push lands an edit that is undoable and invisible.",
			len(offenders), strings.Join(offenders, "\n"))
	}
}

// repoRoot walks up from the package directory to the checkout holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above the package directory")
		}
		dir = parent
	}
}

// trackedGoFiles lists the repository's own Go sources, slash-separated and
// relative to root. Untracked and ignored trees are excluded by construction,
// which is the point: a walk of the working directory also finds worktree
// copies of this very file and reports each of them as a bypass.
func trackedGoFiles(root string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", "*.go").Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, name := range strings.Split(string(out), "\x00") {
		if name != "" {
			files = append(files, name)
		}
	}
	return files, nil
}

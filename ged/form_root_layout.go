package ged

import (
	"sort"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
)

// activeFormLayout ignores a detached former root retained for undo.
func (s *GedScene) activeFormLayout() *FakeWidget {
	if s.formLayout != nil && s.formLayout.Parent() == s {
		return managedLayoutTarget(s.formLayout)
	}
	return nil
}

// A resize triggers Layout through SetBounds1. Only explicitly reflow an
// unchanged rectangle after attaching or restoring children.
func (s *GedScene) syncFormLayout(force bool) {
	box := s.activeFormLayout()
	if box == nil {
		return
	}
	target := geom.Rect{Width: s.Width(), Height: s.Height()}
	if box.Bounds1() != target {
		box.SetBounds1(target)
	} else if force {
		box.Layout()
	}
}

func (v *GedView) selectedFormForLayout() *GedScene {
	s := v.GedScene()
	if s == nil {
		return nil
	}
	items := v.Selection().ItemList()
	if len(items) == 0 || (len(items) == 1 && items[0] == s) {
		return s
	}
	return nil
}

func (s *GedScene) canApplyFormLayout() bool {
	if s.activeFormLayout() != nil {
		return false
	}
	var unlocked func(graph.IItem) bool
	unlocked = func(parent graph.IItem) bool {
		for _, it := range parent.Children() {
			if _, ok := it.(*FakeWidget); !ok || it.IsLockPos() || it.IsLockSize() || !unlocked(it) {
				return false
			}
		}
		return true
	}
	return unlocked(s)
}

type formRootLayoutCommand struct {
	view     *GedView
	scene    *GedScene
	box      *FakeWidget
	oldRoot  *FakeWidget
	move     *graph.ReparentCommand
	before   []layoutSnapshot
	original []graph.IItem
	items    []*FakeWidget
	cells    []layoutPlacement
}

func (c *formRootLayoutCommand) Text() string { return "Lay Out Form" }

func (c *formRootLayoutCommand) Redo() {
	c.move.Redo()
	for i, w := range c.items {
		w.layoutCell = c.cells[i]
	}
	c.scene.formLayout = c.box
	c.box.layoutManaged = true
	c.scene.syncFormLayout(true)
	setLayoutSelection(c.view, []graph.IItem{c.box})
}

func (c *formRootLayoutCommand) Undo() {
	c.move.Undo()
	c.scene.formLayout = c.oldRoot
	for _, s := range c.before {
		s.item.layoutCell = s.placement
		s.item.SetBounds1(s.bounds)
	}
	setLayoutSelection(c.view, c.original)
}

func (v *GedView) applyFormLayout(scene *GedScene, kind LayoutKind) bool {
	if err := scene.CommitPendingEdits(); err != nil {
		core.Warn(err)
		return false
	}
	if v.selectedFormForLayout() != scene || !scene.canApplyFormLayout() {
		return false
	}
	box, err := NewFakeWidgetFromFactory(string(kind))
	if err != nil {
		return false
	}
	cmd := &formRootLayoutCommand{view: v, scene: scene, box: box, oldRoot: scene.formLayout,
		move: graph.NewReparentCommand("Lay Out Form"), original: append([]graph.IItem(nil), v.Selection().ItemList()...)}
	var capture func(graph.IItem)
	capture = func(parent graph.IItem) {
		for _, child := range parent.Children() {
			f := child.(*FakeWidget) // validated above
			cmd.before = append(cmd.before, layoutSnapshot{f, f.Bounds1(), f.layoutCell})
			capture(f)
		}
	}
	capture(scene)
	for _, item := range scene.Children() {
		cmd.items = append(cmd.items, item.(*FakeWidget))
	}
	sort.SliceStable(cmd.items, func(i, j int) bool {
		a, b := cmd.items[i], cmd.items[j]
		if kind == LayoutHorizontal || (kind != LayoutVertical && a.Y() == b.Y()) {
			return a.X() < b.X()
		}
		return a.Y() < b.Y()
	})
	cmd.cells = layoutPlacements(cmd.items, kind)
	cmd.move.Add(box, nil, scene)
	for _, w := range cmd.items {
		cmd.move.AddMoved(w, scene, box, w.X(), w.Y())
	}
	scene.PushCommand(cmd)
	return true
}

func (v *GedView) breakableLayout(item graph.IItem) *FakeWidget {
	if scene, ok := item.(*GedScene); ok {
		item = scene.activeFormLayout()
	}
	box, ok := item.(*FakeWidget)
	if !ok || box == nil || box.Scene() != v.Scene() || box.IsLockPos() ||
		!isDesignerLayoutFactory(box.factoryName) || box.Parent() == nil {
		return nil
	}
	if box.HasChildren() {
		return box
	}
	if scene, ok := box.Parent().(*GedScene); ok && scene.activeFormLayout() == box {
		return box
	}
	return nil
}

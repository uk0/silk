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
	return s.activeFormLayout() == nil && layoutChildrenUnlocked(s)
}

func layoutChildrenUnlocked(parent graph.IItem) bool {
	for _, it := range parent.Children() {
		if _, ok := it.(*FakeWidget); !ok || it.IsLockPos() || it.IsLockSize() || !layoutChildrenUnlocked(it) {
			return false
		}
	}
	return true
}

type ownedRootLayoutCommand struct {
	view     *GedView
	box      *FakeWidget
	move     *graph.ReparentCommand
	before   []layoutSnapshot
	original []graph.IItem
	items    []*FakeWidget
	cells    []layoutPlacement
	text     string
	bind     func(applied bool)
}

func (c *ownedRootLayoutCommand) Text() string { return c.text }

func (c *ownedRootLayoutCommand) Redo() {
	c.move.Redo()
	for i, w := range c.items {
		w.layoutCell = c.cells[i]
	}
	c.box.layoutManaged = true
	c.bind(true)
	setLayoutSelection(c.view, []graph.IItem{c.box})
}

func (c *ownedRootLayoutCommand) Undo() {
	c.move.Undo()
	c.bind(false)
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
	const title = "Lay Out Form"
	cmd := v.prepareOwnedRootLayout(scene, kind, title)
	if cmd == nil {
		return false
	}
	prior := scene.formLayout
	cmd.bind = func(applied bool) {
		scene.formLayout = prior
		if applied {
			scene.formLayout = cmd.box
			scene.syncFormLayout(true)
		}
	}
	scene.PushCommand(cmd)
	return true
}

// prepareOwnedRootLayout is shared by forms and titled containers. Callers
// validate the host/descendants and supply the owning geometry policy.
func (v *GedView) prepareOwnedRootLayout(owner graph.IItem, kind LayoutKind, title string) *ownedRootLayoutCommand {
	box, err := NewFakeWidgetFromFactory(string(kind))
	if err != nil {
		return nil
	}
	cmd := &ownedRootLayoutCommand{view: v, box: box, text: title,
		move: graph.NewReparentCommand(title), original: append([]graph.IItem(nil), v.Selection().ItemList()...)}
	var capture func(graph.IItem)
	capture = func(parent graph.IItem) {
		for _, child := range parent.Children() {
			f := child.(*FakeWidget) // validated above
			cmd.before = append(cmd.before, layoutSnapshot{f, f.Bounds1(), f.layoutCell})
			capture(f)
		}
	}
	capture(owner)
	for _, item := range owner.Children() {
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
	cmd.move.Add(box, nil, owner)
	for _, w := range cmd.items {
		cmd.move.AddMoved(w, owner, box, w.X(), w.Y())
	}
	return cmd
}

func (v *GedView) breakableLayout(item graph.IItem) *FakeWidget {
	if scene, ok := item.(*GedScene); ok {
		item = scene.activeFormLayout()
	} else if owner, ok := item.(*FakeWidget); ok && owner.activeContentLayout() != nil {
		item = owner.activeContentLayout()
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
	if owner, ok := box.Parent().(*FakeWidget); ok && owner.activeContentLayout() == box {
		return box
	}
	return nil
}

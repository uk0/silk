package ged

import (
	"maps"
	"sort"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// selectedManagedLayout also resolves the layout of a selected owner, so an
// empty canvas selection can change the form's root without finding its item.
func (v *GedView) selectedManagedLayout() *FakeWidget {
	if form := v.selectedFormForLayout(); form != nil {
		return form.activeFormLayout()
	}
	items := v.Selection().ItemList()
	if len(items) != 1 {
		return nil
	}
	f, ok := items[0].(*FakeWidget)
	if !ok || f.Scene() != v.Scene() {
		return nil
	}
	if root := f.activeContentLayout(); root != nil {
		return root
	}
	return managedLayoutTarget(f)
}

func canConvertLayout(box *FakeWidget, kind LayoutKind) bool {
	if box.Parent() == nil || box.factoryName == string(kind) || box.IsLockSize() ||
		!unlockedLayoutAncestors(box) || !layoutChildrenUnlocked(box) {
		return false
	}
	if box.factoryName == string(LayoutGrid) {
		for i, child := range box.Children() {
			cell := box.childPlacement(child.(*FakeWidget), i)
			// Other layout types cannot represent a spanning grid control.
			if cell.RowSpan != 1 || cell.ColumnSpan != 1 {
				return false
			}
		}
	}
	return true
}

type layoutConversionChild struct {
	widget *FakeWidget
	cell   layoutPlacement
}

func conversionChildren(box *FakeWidget) []layoutConversionChild {
	children := box.Children()
	items := make([]layoutConversionChild, 0, len(children))
	for i, child := range children {
		f := child.(*FakeWidget)
		items = append(items, layoutConversionChild{widget: f, cell: box.childPlacement(f, i)})
	}
	// A grid swap can change visual positions without changing tree order.
	// Follow actual rows and columns rather than historical insertion order.
	if isTabularLayout(box) {
		sort.SliceStable(items, func(i, j int) bool {
			a, b := items[i].cell, items[j].cell
			if a.Row == b.Row {
				return a.Column < b.Column
			}
			return a.Row < b.Row
		})
	}
	return items
}

func convertedLayoutCell(source, target LayoutKind, item layoutConversionChild, i int) layoutPlacement {
	p := layoutPlacement{RowSpan: 1, ColumnSpan: 1, Valid: true}
	switch target {
	case LayoutHorizontal:
		p.Column = i
	case LayoutVertical:
		p.Row = i
	case LayoutForm:
		p.Row, p.Column = i/2, i%2
	case LayoutGrid:
		switch source {
		case LayoutHorizontal:
			p.Column = i
		case LayoutVertical:
			p.Row = i
		case LayoutForm:
			p.Row, p.Column = item.cell.Row, item.cell.Column
		}
	}
	return p
}

func copySharedLayoutProperties(dst, src gui.IWidget) {
	type spacing interface {
		Spacing() float64
		SetSpacing(float64)
	}
	type padding interface {
		Padding() gui.Padding
		SetPadding(gui.Padding)
	}
	if from, ok := src.(spacing); ok {
		if to, ok := dst.(spacing); ok {
			to.SetSpacing(from.Spacing())
		}
	}
	if from, ok := src.(padding); ok {
		if to, ok := dst.(padding); ok {
			to.SetPadding(from.Padding())
		}
	}
}

type convertLayoutCommand struct {
	view     *GedView
	owner    graph.IItem
	from, to *FakeWidget
	formRoot *GedScene
	move     *graph.ReparentCommand
	items    []layoutConversionChild
	before   []layoutSnapshot
	original []graph.IItem
}

func (c *convertLayoutCommand) Text() string { return "Change Layout Type" }

func (c *convertLayoutCommand) reflow(box *FakeWidget) {
	if c.formRoot != nil {
		c.formRoot.formLayout = box
		c.formRoot.syncFormLayout(true)
		return
	}
	if owner, ok := c.owner.(*FakeWidget); ok && owner.ManagesChildGeometry() {
		owner.Layout()
	}
	box.Layout()
}

func (c *convertLayoutCommand) Redo() {
	c.move.Redo()
	for i, item := range c.items {
		item.widget.layoutCell = convertedLayoutCell(LayoutKind(c.from.factoryName), LayoutKind(c.to.factoryName), item, i)
	}
	c.reflow(c.to)
	setLayoutSelection(c.view, []graph.IItem{c.to})
}

func (c *convertLayoutCommand) Undo() {
	c.move.Undo()
	for _, s := range c.before {
		s.item.layoutCell = s.placement
	}
	c.reflow(c.from)
	// The scene owns absolute geometry for free descendants. Restore in
	// preorder after the original layout has been registered with its host.
	for _, s := range c.before {
		s.item.SetBounds1(s.bounds)
	}
	setLayoutSelection(c.view, c.original)
}

func (v *GedView) convertManagedLayout(old *FakeWidget, kind LayoutKind) bool {
	s := v.GedScene()
	if err := s.CommitPendingEdits(); err != nil {
		core.Warn(err)
		return false
	}
	if v.selectedManagedLayout() != old || !canConvertLayout(old, kind) {
		return false
	}
	next, err := NewFakeWidgetFromFactory(string(kind))
	if err != nil {
		return false
	}
	next.name, next.code, next.eventHandlers = old.name, old.code, maps.Clone(old.eventHandlers)
	next.layoutManaged, next.layoutCell = true, old.layoutCell
	next.SetLocalCoord(old.HasLocalCoord())
	next.SetBounds1(old.Bounds1())
	copySharedLayoutProperties(next.widget, old.widget)
	owner := old.Parent()
	c := &convertLayoutCommand{view: v, owner: owner, from: old, to: next,
		move:  graph.NewReparentCommand("Change Layout Type"),
		items: conversionChildren(old), original: append([]graph.IItem(nil), v.Selection().ItemList()...)}
	if form, ok := owner.(*GedScene); ok && form.activeFormLayout() == old {
		c.formRoot = form
	}
	var capture func(graph.IItem)
	capture = func(parent graph.IItem) {
		for _, child := range parent.Children() {
			f := child.(*FakeWidget)
			c.before = append(c.before, layoutSnapshot{f, f.Bounds1(), f.layoutCell})
			capture(f)
		}
	}
	capture(old)
	c.move.Add(old, owner, nil)
	c.move.AddAt(next, nil, owner, old.IndexInParent())
	for _, item := range c.items {
		c.move.Add(item.widget, old, next)
	}
	s.PushCommand(c)
	return true
}

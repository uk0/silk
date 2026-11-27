package ged

import (
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// GridTrackAxis identifies a track in the selected widget's managed grid.
type GridTrackAxis uint8

const (
	GridRow GridTrackAxis = iota
	GridColumn

	gridInsertRowTitle = "Insert Grid Row Above"
	gridInsertColTitle = "Insert Grid Column Before"
	gridInsertSpan     = "A grid widget spans this boundary"
	gridInsertLimit    = "The grid is at its track limit"
	gridInsertNoSpace  = "There is not enough visible space for a new grid track"
)

func init() {
	i18n.AddMany("zh-CN", map[string]string{
		gridInsertRowTitle: "在上方插入网格行",
		gridInsertColTitle: "在左侧插入网格列",
		gridInsertSpan:     "有控件跨越此插入边界",
		gridInsertLimit:    "网格已达到行列数上限",
		gridInsertNoSpace:  "没有足够的可见空间插入网格行列",
	})
}

type gridTrackChange struct {
	widget *FakeWidget
	before layoutPlacement
	after  layoutPlacement
}

type gridTrackInsertion struct {
	parent, selected *FakeWidget
	axis             GridTrackAxis
	index            int
	changes          []gridTrackChange
}

func (v *GedView) selectedGridTrackTarget() (parent, child *FakeWidget) {
	if v == nil || v.Scene() == nil {
		return nil, nil
	}
	selection := v.Selection().ItemList()
	if len(selection) != 1 {
		return nil, nil
	}
	child, ok := selection[0].(*FakeWidget)
	if !ok || child.Scene() != v.Scene() {
		return nil, nil
	}
	parent, ok = child.Parent().(*FakeWidget)
	if !ok || !parent.layoutManaged || parent.factoryName != string(LayoutGrid) || parent.Scene() != v.Scene() {
		return nil, nil
	}
	return parent, child
}

func (a GridTrackAxis) span(p layoutPlacement) (start, size int) {
	if a == GridColumn {
		return p.Column, p.ColumnSpan
	}
	return p.Row, p.RowSpan
}

func (a GridTrackAxis) title() string {
	if a == GridColumn {
		return gridInsertColTitle
	}
	return gridInsertRowTitle
}

func (v *GedView) gridTrackInsertion(axis GridTrackAxis) (gridTrackInsertion, string) {
	parent, selected := v.selectedGridTrackTarget()
	if parent == nil || (axis != GridRow && axis != GridColumn) {
		return gridTrackInsertion{}, dropCellGap
	}
	if selected.IsLockPos() || !unlockedLayoutAncestors(parent) {
		return gridTrackInsertion{}, dropLayoutLocked
	}
	index, _ := axis.span(placementForChild(selected))
	plan := gridTrackInsertion{parent: parent, selected: selected, axis: axis, index: index}
	for i, child := range parent.Children() {
		f, ok := child.(*FakeWidget)
		if !ok {
			return gridTrackInsertion{}, dropCellGap
		}
		p := parent.childPlacement(f, i)
		start, length := axis.span(p)
		if start < index {
			if start+length > index {
				return gridTrackInsertion{}, gridInsertSpan
			}
			continue
		}
		if start+length >= 4096 {
			return gridTrackInsertion{}, gridInsertLimit
		}
		if f.IsLockPos() {
			return gridTrackInsertion{}, dropLayoutLocked
		}
		if axis == GridColumn {
			p.Column++
		} else {
			p.Row++
		}
		plan.changes = append(plan.changes, gridTrackChange{f, f.layoutCell, p})
	}
	return plan, ""
}

// CanInsertGridTrack checks selection and cell metadata without building the
// runtime layout. InsertGridTrackBeforeSelection additionally validates the
// proposed reflow before committing it.
func (v *GedView) CanInsertGridTrack(axis GridTrackAxis) bool {
	_, failure := v.gridTrackInsertion(axis)
	return failure == ""
}

func (v *GedView) gridInsertionReflowFailure(plan gridTrackInsertion) string {
	scene := v.Scene()
	page := scene.MapRectToScene1(scene.Bounds1())
	clip := designDropArea(page, plan.parent)
	if clip.IsEmpty() {
		return gridInsertNoSpace
	}
	trial := &layoutTrial{
		children: make(map[*FakeWidget]gui.IWidget),
		orders:   make(map[*FakeWidget][]graph.IItem),
		cells:    make(map[*FakeWidget]layoutPlacement, len(plan.changes)),
	}
	for _, c := range plan.changes {
		trial.cells[c.widget] = c.after
	}
	runtime, ok := plan.parent.generate(trial).(*gui.GridLayout)
	if !ok {
		return gridInsertNoSpace
	}
	runtime.Layout()
	tracks := runtime.CellTracks()
	rect := tracks.Content
	if plan.axis == GridRow {
		t, ok := trackByIndex(tracks.Rows, plan.index)
		if !ok {
			return gridInsertNoSpace
		}
		rect.Y, rect.Height = t.Start, t.Size
	} else {
		t, ok := trackByIndex(tracks.Columns, plan.index)
		if !ok {
			return gridInsertNoSpace
		}
		rect.X, rect.Width = t.Start, t.Size
	}
	origin := plan.parent.MapRectToScene1(plan.parent.Bounds1())
	newTrack := geom.Rect{
		X: origin.X + gui.PixelToMm(rect.X), Y: origin.Y + gui.PixelToMm(rect.Y),
		Width: gui.PixelToMm(rect.Width), Height: gui.PixelToMm(rect.Height),
	}
	if newTrack.IntersectCopy(clip).IsEmpty() {
		return gridInsertNoSpace
	}
	return tabularTrialFailure(plan.parent, runtime, trial, page, clip, plan.selected, nil, false)
}

// InsertGridTrackBeforeSelection creates an unoccupied row above or column
// before the sole selected grid widget. Cells and spans at/after that index
// move together. Inserting through a span, reflowing protected geometry or
// creating a clipped track is rejected without a layout history entry.
func (v *GedView) InsertGridTrackBeforeSelection(axis GridTrackAxis) bool {
	plan, failure := v.gridTrackInsertion(axis)
	if failure != "" {
		return false
	}
	scene := v.GedScene()
	if !v.canCommitTabularDrop() || v.GedScene() != scene {
		return false
	}
	// A property draft can move/remove the selection or even replace the
	// document synchronously. Do not apply the menu intent to a new target.
	current, failure := v.gridTrackInsertion(axis)
	if failure != "" || current.parent != plan.parent || current.selected != plan.selected {
		return false
	}
	if failure = v.gridInsertionReflowFailure(current); failure != "" {
		v.showStatus(i18n.T(failure))
		return false
	}
	c := &gridTrackInsertionCommand{view: v, plan: current}
	var save func(graph.IItem)
	save = func(p graph.IItem) {
		for _, node := range p.Children() {
			if f, ok := node.(*FakeWidget); ok {
				c.bounds = append(c.bounds, layoutSnapshot{f, f.Bounds1(), f.layoutCell})
				save(f)
			}
		}
	}
	save(current.parent)
	scene.PushCommand(c)
	return true
}

type gridTrackInsertionCommand struct {
	view   *GedView
	plan   gridTrackInsertion
	bounds []layoutSnapshot
}

func (c *gridTrackInsertionCommand) Text() string { return i18n.T(c.plan.axis.title()) }
func (c *gridTrackInsertionCommand) Redo()        { c.apply(false) }
func (c *gridTrackInsertionCommand) Undo()        { c.apply(true) }
func (c *gridTrackInsertionCommand) apply(undo bool) {
	for _, change := range c.plan.changes {
		if undo {
			change.widget.layoutCell = change.before
		} else {
			change.widget.layoutCell = change.after
		}
	}
	c.plan.parent.Layout()
	if undo {
		// Restore authored geometry, including fractional coordinates of
		// free-positioned descendants, after restoring track registration.
		for _, b := range c.bounds {
			b.item.SetBounds1(b.bounds)
		}
	}
	c.view.refreshPropertyValues()
	c.view.Self().Update()
}

func (v *GedView) addGridInsertionMenu(menu *gui.Menu, anchor *FakeWidget) {
	parent, selected := v.selectedGridTrackTarget()
	if parent == nil || selected != anchor {
		return
	}
	menu.AddSeparator()
	for _, spec := range []struct {
		axis GridTrackAxis
		icon string
	}{{GridRow, "layout-row-insert"}, {GridColumn, "layout-column-insert"}} {
		action := menu.AddButton1(i18n.T(spec.axis.title()), paint.LoadIcon(spec.icon)).Action()
		action.SetEnabled(v.CanInsertGridTrack(spec.axis))
		action.BindFunc0(func() {
			if p, item := v.selectedGridTrackTarget(); item == anchor && p == parent {
				v.InsertGridTrackBeforeSelection(spec.axis)
			}
		})
	}
}

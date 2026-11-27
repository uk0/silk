package ged

import (
	"fmt"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/prop"
)

type layoutPlacement struct {
	Row, Column, RowSpan, ColumnSpan int
	Valid                            bool
}

func (p layoutPlacement) normalized() layoutPlacement {
	if p.Row < 0 {
		p.Row = 0
	}
	if p.Column < 0 {
		p.Column = 0
	}
	if p.Row > 4095 {
		p.Row = 4095
	}
	if p.Column > 4095 {
		p.Column = 4095
	}
	if p.RowSpan > 4096-p.Row {
		p.RowSpan = 4096 - p.Row
	}
	if p.ColumnSpan > 4096-p.Column {
		p.ColumnSpan = 4096 - p.Column
	}
	if p.RowSpan < 1 {
		p.RowSpan = 1
	}
	if p.ColumnSpan < 1 {
		p.ColumnSpan = 1
	}
	return p
}
func (f *FakeWidget) childPlacement(child *FakeWidget, index int) layoutPlacement {
	p := child.layoutCell
	if !p.Valid {
		p = layoutPlacement{Row: index / 2, Column: index % 2, RowSpan: 1, ColumnSpan: 1, Valid: true}
	}
	if f.factoryName == string(LayoutForm) {
		p.Column %= 2
		p.RowSpan = 1
		p.ColumnSpan = 1
	}
	return p.normalized()
}
func (f *FakeWidget) saveLayout(doc *core.TDoc) {
	if f.layoutManaged {
		doc.WriteAttr("layout_managed", true)
	}
	if f.layoutCell.Valid {
		p := f.layoutCell.normalized()
		doc.WriteAttr("layout_row", p.Row)
		doc.WriteAttr("layout_column", p.Column)
		doc.WriteAttr("layout_row_span", p.RowSpan)
		doc.WriteAttr("layout_column_span", p.ColumnSpan)
	}
}
func (f *FakeWidget) loadLayout(doc *core.TDoc) {
	doc.ReadAttr("layout_managed", &f.layoutManaged)
	if doc.ChildByKey("layout_row", false) != nil {
		p := layoutPlacement{Valid: true}
		doc.ReadAttr("layout_row", &p.Row)
		doc.ReadAttr("layout_column", &p.Column)
		doc.ReadAttr("layout_row_span", &p.RowSpan)
		doc.ReadAttr("layout_column_span", &p.ColumnSpan)
		f.layoutCell = p.normalized()
	}
}
func setDeclPlacement(n *decl.Node, p layoutPlacement) {
	p = p.normalized()
	n.SetProp("layout_row", decl.Lit{V: p.Row})
	n.SetProp("layout_column", decl.Lit{V: p.Column})
	n.SetProp("layout_row_span", decl.Lit{V: p.RowSpan})
	n.SetProp("layout_column_span", decl.Lit{V: p.ColumnSpan})
}

type layoutWidgetAdder interface {
	AddLayoutWidget(gui.IWidget, int, int, int, int)
}

// layoutDesignChildren mirrors the *same runtime layout* on the editable
// canvas; it never binds the real designer widgets to the temporary preview.
func (f *FakeWidget) layoutDesignChildren() {
	f.cellTracks, f.hasCellTracks = gui.LayoutCellTracks{}, false
	if !f.layoutManaged || !isDesignerLayoutFactory(f.factoryName) || (!f.HasChildren() && !isTabularLayout(f)) {
		return
	}
	runtime := f.Generate()
	if runtime == nil {
		return
	}
	if source, ok := runtime.(gui.LayoutCellTrackSource); ok {
		f.cellTracks, f.hasCellTracks = source.CellTracks(), true
	}
	children := runtime.Children()
	i := 0
	for _, it := range f.Children() {
		child, ok := it.(*FakeWidget)
		if !ok {
			continue
		}
		if i >= len(children) {
			break
		}
		x, y, w, h := children[i].Bounds()
		i++
		xmm, ymm := gui.PixelToMm(x), gui.PixelToMm(y)
		if !f.HasLocalCoord() {
			xmm += f.X()
			ymm += f.Y()
		}
		dx, dy := xmm-child.X(), ymm-child.Y()
		child.SetBounds(xmm, ymm, gui.PixelToMm(w), gui.PixelToMm(h))
		if !child.ManagesChildGeometry() {
			translateFreeChildren(child, dx, dy)
		}
	}
}

func isGridOrFormParent(f *FakeWidget) bool {
	p, ok := f.Parent().(*FakeWidget)
	return ok && (p.factoryName == string(LayoutGrid) || p.factoryName == string(LayoutForm))
}
func placementForChild(f *FakeWidget) layoutPlacement {
	p, ok := f.Parent().(*FakeWidget)
	if !ok {
		return layoutPlacement{RowSpan: 1, ColumnSpan: 1}
	}
	for i, c := range p.Children() {
		if c == f {
			return p.childPlacement(f, i)
		}
	}
	return p.childPlacement(f, 0)
}

func emitLayoutProperties(buf *strings.Builder, f fieldInfo) {
	if !isDesignerLayoutFactory(f.factoryName) {
		return
	}
	if v, ok := f.widget.(interface{ Spacing() float64 }); ok {
		fmt.Fprintf(buf, "\tui.%s.SetSpacing(%s)\n", f.name, fmtFloat(v.Spacing()))
	}
	if v, ok := f.widget.(interface{ Padding() gui.Padding }); ok {
		p := v.Padding()
		fmt.Fprintf(buf, "\tui.%s.SetPadding(gui.Padding{L:%s,R:%s,T:%s,B:%s})\n", f.name, fmtFloat(p.L), fmtFloat(p.R), fmtFloat(p.T), fmtFloat(p.B))
	}
	if v, ok := f.widget.(*gui.FormLayout); ok {
		fmt.Fprintf(buf, "\tui.%s.SetLabelWidth(%s)\n\tui.%s.SetRowHeight(%s)\n", f.name, fmtFloat(v.LabelWidth()), f.name, fmtFloat(v.RowHeight()))
	}
}
func (f *FakeWidget) designPropertyChanged() {
	f.Layout()
	if p, ok := f.Parent().(*FakeWidget); ok && p.layoutManaged {
		p.Layout()
	}
}

func (f *FakeWidget) enumLayoutPlacementProperties(list prop.IPropertyList) {
	if !isGridOrFormParent(f) {
		return
	}
	list.AddProperty("布局行", func() int { return placementForChild(f).Row }, func(v int) {
		p := placementForChild(f)
		p.Row = v
		f.layoutCell = p.normalized()
		f.designPropertyChanged()
	})
	list.AddProperty("布局列", func() int { return placementForChild(f).Column }, func(v int) {
		p := placementForChild(f)
		p.Column = v
		f.layoutCell = p.normalized()
		f.designPropertyChanged()
	})
	if p, ok := f.Parent().(*FakeWidget); ok && p.factoryName == string(LayoutGrid) {
		list.AddProperty("跨行数", func() int { return placementForChild(f).RowSpan }, func(v int) {
			p := placementForChild(f)
			p.RowSpan = v
			f.layoutCell = p.normalized()
			f.designPropertyChanged()
		})
		list.AddProperty("跨列数", func() int { return placementForChild(f).ColumnSpan }, func(v int) {
			p := placementForChild(f)
			p.ColumnSpan = v
			f.layoutCell = p.normalized()
			f.designPropertyChanged()
		})
	}
}

// ManagesChildGeometry distinguishes explicit layouts from free-positioned
// containers for graph gestures that must carry shared-coordinate descendants.
func (f *FakeWidget) ManagesChildGeometry() bool {
	return f.layoutManaged && isDesignerLayoutFactory(f.factoryName)
}

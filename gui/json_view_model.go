package gui

import (
	"fmt"
	"strconv"

	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/jsonx"
)

type jsonViewRow struct {
	node       jsonx.Node
	parent     *jsonViewRow
	row        int
	start, end int // a synthetic range when grouped is true
	grouped    bool
	children   []*jsonViewRow
	loaded     bool
}
type jsonViewModel struct {
	GuiModel
	root     *jsonViewRow
	pageSize int
	owner    *JSONView
}

func newJSONViewModel(owner *JSONView, root jsonx.Node) *jsonViewModel {
	m := &jsonViewModel{pageSize: owner.pageSize, owner: owner}
	m.GuiModel.Init(m)
	if root.Valid() {
		m.root = &jsonViewRow{node: root}
	}
	return m
}
func (m *jsonViewModel) row(mi ModelIndex) *jsonViewRow {
	if mi.Model != m {
		return nil
	}
	r, _ := mi.Param.(*jsonViewRow)
	return r
}
func (m *jsonViewModel) index(r *jsonViewRow, col int) ModelIndex {
	if r == nil || col < 0 || col >= 3 {
		return ModelIndex{}
	}
	return ModelIndex{Row: r.row, Col: col, Param: r, Model: m}
}
func (m *jsonViewModel) span(r *jsonViewRow) (int, int, int) {
	start, end := 0, r.node.Len()
	if r.grouped {
		start, end = r.start, r.end
	}
	chunk := 1
	for (end-start+chunk-1)/chunk > m.pageSize {
		chunk *= m.pageSize
	}
	return start, end, chunk
}
func (m *jsonViewModel) materialize(r *jsonViewRow) {
	if r == nil || r.loaded {
		return
	}
	r.loaded = true
	start, end, chunk := m.span(r)
	for i := start; i < end; i += chunk {
		child := &jsonViewRow{parent: r, row: len(r.children)}
		if chunk > 1 {
			child.node = r.node
			child.grouped = true
			child.start = i
			child.end = min(i+chunk, end)
		} else {
			child.node, _ = r.node.Child(i)
		}
		r.children = append(r.children, child)
	}
}
func (m *jsonViewModel) Index(row, col int, parent ModelIndex) ModelIndex {
	if row < 0 || col < 0 || col >= 3 {
		return ModelIndex{}
	}
	if parent.IsNil() {
		if row == 0 {
			return m.index(m.root, col)
		}
		return ModelIndex{}
	}
	r := m.row(parent)
	if r == nil || parent.Col != 0 {
		return ModelIndex{}
	}
	m.materialize(r)
	if row >= len(r.children) {
		return ModelIndex{}
	}
	return m.index(r.children[row], col)
}
func (m *jsonViewModel) Parent(mi ModelIndex) ModelIndex {
	r := m.row(mi)
	if r == nil {
		return ModelIndex{}
	}
	return m.index(r.parent, 0)
}
func (m *jsonViewModel) RowCount(parent ModelIndex) int {
	if parent.IsNil() {
		if m.root != nil {
			return 1
		}
		return 0
	}
	r := m.row(parent)
	if r == nil || parent.Col != 0 {
		return 0
	}
	start, end, chunk := m.span(r)
	return (end - start + chunk - 1) / chunk
}
func (m *jsonViewModel) ColCount() int { return 3 }
func (m *jsonViewModel) Flags(mi ModelIndex) ItemFlags {
	if m.row(mi) == nil {
		return NoItemFlags
	}
	return ItemIsEnabled | ItemIsSelectable
}
func (m *jsonViewModel) HeaderData(section int, vertical bool, role ItemDataRole) any {
	if section < 0 || section >= 3 || vertical {
		return nil
	}
	if role == DisplayRole {
		return []string{"Key / index", "Type", "Value"}[section]
	}
	if role == SizeHintRole {
		width := max(360, m.owner.Width()-m.owner.padding.L-m.owner.padding.R-Theme().ScrollWidth)
		sizes := []float64{max(140, width*.34), 85, max(140, width*.66-85)}
		return geom.Vec2{X: sizes[section], Y: Theme().ItemHeight}
	}
	return nil
}
func (m *jsonViewModel) Data(mi ModelIndex, role ItemDataRole) any {
	r := m.row(mi)
	if r == nil {
		return nil
	}
	if role != DisplayRole && role != ToolTipRole {
		return nil
	}
	if r.grouped {
		switch mi.Col {
		case 0:
			return fmt.Sprintf("[%d … %d]", r.start, r.end-1)
		case 1:
			return "group"
		case 2:
			return fmt.Sprintf("%d entries · expand to browse", r.end-r.start)
		}
	}
	if role == ToolTipRole {
		return r.node.Pointer()
	}
	switch mi.Col {
	case 0:
		if r.parent == nil && r.node.Pointer() == "" {
			return "$"
		}
		if r.parent != nil && r.parent.node.Kind() == jsonx.Array {
			return "[" + r.node.Key() + "]"
		}
		return strconv.Quote(jsonViewKeyPreview(r.node.Key()))
	case 1:
		return string(r.node.Kind())
	case 2:
		return r.node.Preview(160)
	}
	return nil
}
func jsonViewKeyPreview(key string) string {
	count := 0
	for i := range key {
		if count == 96 {
			return key[:i] + "…"
		}
		count++
	}
	return key
}

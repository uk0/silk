package data

import (
	//"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

const (
	colMain = iota
	_colMax
)

// 项目模型的结点
// 注: 因为同一个数据可以出现在多个树结点下, 所以我们不能直接用数据作为节点
type _Node struct {
	IData

	parent   *_Node
	children []*_Node
}

func (n *_Node) Parent() *_Node {
	return n.parent
}

func (n *_Node) Children() []*_Node {
	return n.children
}

func (n *_Node) IndexInParent() int {
	if n.parent == nil {
		return -1
	}
	for i, v := range n.parent.Children() {
		if v == n {
			return i
		}
	}
	return -1
}

// 数据树模型
type PrjModel struct {
	gui.GuiModel

	roots []*_Node
}

func (m *PrjModel) Init(self gui.IGuiModel) {
	m.GuiModel.Init(self)
}

func (m *PrjModel) isRoot(n *_Node) bool {
	return n.parent == nil
}

// 获取结点对应行第0列的索引
func (m *PrjModel) col0Index(n *_Node) gui.ModelIndex {
	// nil结点对应nil索引
	if n == nil {
		return gui.ModelIndex{}
	}

	if n.parent == nil {
		for i, v := range m.roots {
			if v == n {
				return m.Index(i, 0, gui.ModelIndex{})
			}
		}
		return gui.ModelIndex{}
	}

	return m.Index(n.IndexInParent(), 0, m.col0Index(n.Parent()))
}

// 获取parent下第row行col列的索引
func (m *PrjModel) Index(row, col int, parent gui.ModelIndex) gui.ModelIndex {
	if col < 0 || col >= _colMax {
		return gui.ModelIndex{}
	}

	// 根节点
	if parent.IsNil() {
		if row < len(m.roots) {
			return gui.ModelIndex{row, col, m.roots[row], m}
		}
		return gui.ModelIndex{}
	}

	// 非根节点
	n := parent.Param.(*_Node)
	return gui.ModelIndex{row, col, n.Children()[row], m}
}

// 获取单元格数据
func (m *PrjModel) Data(idx gui.ModelIndex, role gui.ItemDataRole) interface{} {
	if idx.IsNil() {
		return nil
	}
	n := idx.Param.(*_Node)
	switch role {
	case gui.DisplayRole:
		switch idx.Col {
		case colMain:
			return n.DataName()
		default:
			return nil
		}
	default:
		return nil
	}
	return nil
}

func (m *PrjModel) HeaderData(section int, vertical bool, role gui.ItemDataRole) interface{} {
	if vertical {
		return nil // unsupported
	}
	switch role {
	case gui.DisplayRole:
		switch section {
		case colMain:
			return "Name"
		default:
			return nil
		}
	default:
		return nil
	}
	return nil
}

// 获取父节点的索引
func (m *PrjModel) Parent(idx gui.ModelIndex) gui.ModelIndex {
	if idx.IsNil() {
		return gui.ModelIndex{}
	}

	n := idx.Param.(*_Node)

	// 根节点, 没有父节点
	if n.parent == nil {
		return gui.ModelIndex{}
	}

	// 其他结点
	p := n.Parent()
	return m.col0Index(p)
}

// 指定节点下子结点的行数
func (m *PrjModel) RowCount(parent gui.ModelIndex) int {
	if parent.IsNil() {
		return len(m.roots)
	}
	n := parent.Param.(*_Node)
	return len(n.Children())
}

// 指定节点下的子结点的列数
func (m *PrjModel) ColCount() int {
	return _colMax
}

// 子节点的属性标记, 例如是否可选
func (m *PrjModel) Flags(idx gui.ModelIndex) gui.ItemFlags {
	return 0
}

func (m *PrjModel) Refresh() {
	m.BeginReset()
	m.roots = nil
	for _, v := range DataMan().Roots() {
		n := new(_Node)
		n.IData = v
		m.roots = append(m.roots, n)
	}
	m.EndReset()
}

func (this *PrjModel) Sort(column int, desc bool) {
}

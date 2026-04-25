package graph

import (
	"silk/core"
	"silk/gui"
)

func init() {
	core.RegisterFactory("graph.DbgTreeView", gui.TypeOf(DbgTreeView{}))
}

type DbgTreeView struct {
	gui.TreeView
}

const (
	dtMainCol = iota
	dtBoundsCol
	dtFlagsCol
	_dtMax
)

func NewDbgTreeView() *DbgTreeView {
	p := new(DbgTreeView)
	p.Init(p)
	return p
}

func (this *DbgTreeView) Init(iw gui.IWidget) {
	this.TreeView.Init(iw)
	this.TreeView.SetModel(NewDbgTreeModel())
	this.SetIcon(gui.LoadIcon("tree-view"))
	this.SetTitle("Graph Tree")
}

func (this *DbgTreeView) SetRootItems(items []IItem) {
	if this.DbgTreeModel() == nil {
		this.TreeView.SetModel(NewDbgTreeModel())
	}
	this.DbgTreeModel().SetRootItems(items)
}

func (this *DbgTreeView) DbgTreeModel() *DbgTreeModel {
	if p, ok := this.Model().(*DbgTreeModel); ok {
		return p
	}
	return nil
}

///////////////////////////////////////////////////////////
type DbgTreeModel struct {
	gui.GuiModel
	roots   []IItem
	rootSet map[IItem]int
}

func NewDbgTreeModel() *DbgTreeModel {
	p := new(DbgTreeModel)
	p.Init(p)
	return p
}

func (m *DbgTreeModel) isRoot(node IItem) bool {
	_, ok := m.rootSet[node]
	return ok
}

func (m *DbgTreeModel) rowOfRoot(node IItem) int {
	i, ok := m.rootSet[node]
	if ok {
		return i
	}
	return -1
}

// 获取结点对应行第0列的索引
func (m *DbgTreeModel) col0Index(node IItem) gui.ModelIndex {
	// nil结点对应nil索引
	if node == nil {
		return gui.ModelIndex{}
	}
	row := m.rowOfRoot(node)
	if row == -1 {
		return m.Index(node.IndexInParent(), 0, m.col0Index(node.Parent()))
	}
	return m.Index(row, 0, gui.ModelIndex{})
}

// 获取parent下第row行col列的索引
func (m *DbgTreeModel) Index(row, col int, parent gui.ModelIndex) gui.ModelIndex {
	if row < 0 || col < 0 {
		return gui.ModelIndex{}
	}

	// 根节点
	if parent.IsNil() {
		if row >= len(m.roots) {
			return gui.ModelIndex{}
		}
		return gui.ModelIndex{row, col, m.roots[row], m}
	}

	// 非根节点
	item, ok := parent.Param.(IItem)
	if !ok || item == nil {
		return gui.ModelIndex{}
	}
	children := item.Children()
	if row >= len(children) {
		return gui.ModelIndex{}
	}
	return gui.ModelIndex{row, col, children[row], m}
}

// 获取单元格数据
func (m *DbgTreeModel) Data(idx gui.ModelIndex, role gui.ItemDataRole) interface{} {
	if idx.IsNil() {
		return nil
	}
	// 暂时都返回图标或名字, 无论哪列
	node := idx.Param.(IItem)
	switch role {
	case gui.DisplayRole:
		switch idx.Col {
		case dtMainCol:
			return node.DebugLabel()
		case dtFlagsCol:
			return node.DebugFlagsString()
		case dtBoundsCol:
			return node.Bounds1()
		default:
			return nil
		}
		//	case gui.DecorationRole:
	default:
		return nil
	}
	return nil
}

func (m *DbgTreeModel) HeaderData(section int, vertical bool, role gui.ItemDataRole) interface{} {
	if vertical {
		return nil // unsupported
	}
	switch role {
	case gui.DisplayRole:
		switch section {
		case dtMainCol:
			return "Name"
		case dtFlagsCol:
			return "Flags"
		case dtBoundsCol:
			return "Bounds"
		default:
			return nil
		}
		//	case gui.DecorationRole:
	default:
		return nil
	}
	return nil
}

// 获取父节点的索引
func (m *DbgTreeModel) Parent(idx gui.ModelIndex) gui.ModelIndex {
	if idx.IsNil() {
		return gui.ModelIndex{}
	}

	// 根节点, 没有父节点
	if idx.Row < len(m.roots) && m.roots[idx.Row] == idx.Param {
		return gui.ModelIndex{}
	}

	// 其他结点
	node := idx.Param.(IItem)
	p := node.Parent()
	return m.col0Index(p)
}

// 指定节点下子结点的行数
func (m *DbgTreeModel) RowCount(parent gui.ModelIndex) int {
	if parent.IsNil() {
		return len(m.roots)
	}
	node := parent.Param.(IItem)
	return len(node.Children())
}

// 指定节点下的子结点的列数
func (m *DbgTreeModel) ColCount() int {
	return _dtMax
}

// 子节点的属性标记, 例如是否可选
func (m *DbgTreeModel) Flags(idx gui.ModelIndex) gui.ItemFlags {
	return 0
}

func (m *DbgTreeModel) SetRootItems(items []IItem) {
	m.BeginReset()
	m.roots = items
	m.rootSet = make(map[IItem]int)
	for i, v := range m.roots {
		m.rootSet[v] = i
	}
	m.EndReset()
}

//func (m *DbgTreeModel) SetRootPath(path string) error {
//	info, err := os.Stat(path)
//	if err != nil {
//		return err
//	}
//	m.rootPath = path
//	m.root = new(fsNode)
//	m.root.model = m
//	m.root.info = info
//	m.root.row = 0
//	m.root.List()
//	return nil
//}

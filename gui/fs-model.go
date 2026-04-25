package gui

/*

import (
	"silk/core"
	"silk/paint"
	"io/ioutil"
	"os"
)

type fsNode struct {
	model    *FsModel
	parent   *fsNode
	children []*fsNode
	info     os.FileInfo
	listed   bool
	row      int
}

func (p *fsNode) Model() *FsModel {
	if p.model == nil {
		p.model = p.parent.Model()
	}
	return p.model
}

func (p *fsNode) FullPath() string {
	if p.parent != nil {
		return p.parent.FullPath() + "/" + p.info.Name()
	}
	return p.model.rootPath
}

// 读本节点的信息, 如果是目录则探测底下是否有子节点
func (p *fsNode) List() error {
	if p.listed {
		return nil
	}

	if !p.info.IsDir() {
		p.listed = true
		p.children = nil
		return nil
	}

	entries, err := ioutil.ReadDir(p.FullPath())
	if err != nil {
		return err
	}

	for i, info := range entries {
		c := new(fsNode)
		c.parent = p
		c.info = info
		c.model = p.model
		c.row = i
		//if info.IsDir() {
		//	c.hasChildren = true
		//} else {
		//	c.hasChildren = false
		//}
		p.children = append(p.children, c)

		core.Debug(c.FullPath())
	}

	p.listed = true
	return nil
}

func (p *fsNode) Count() int {
	p.List()
	return len(p.children)
}

func (p *fsNode) Icon() paint.Icon {
	if p.info.IsDir() {
		return LoadIcon("folder")
	}
	return LoadIcon("file")
}

type FsModel struct {
	rootPath string
	root     *fsNode
}

func (m *FsModel) col0Index(node *fsNode) *ModelIndex {
	if node == nil {
		return nil
	}
	if node.parent != nil {
		return m.Index(node.row, 0, m.col0Index(node.parent))
	}
	return m.Index(node.row, 0, nil)
}

func (m *FsModel) Index(row, col int, parent *ModelIndex) *ModelIndex {
	if parent == nil && row == 0 {
		idx := new(ModelIndex)
		idx.row = row
		idx.col = col
		idx.model = m
		idx.internal = m.root
		return idx
	}

	if parent == nil {
		return nil
	}

	p := parent.internal.(*fsNode)
	idx := new(ModelIndex)
	idx.row = row
	idx.col = col
	idx.model = m
	idx.internal = p.children[row]

	return idx
}

func (m *FsModel) Data(idx *ModelIndex, role ItemDataRole) interface{} {
	if idx == nil {
		return nil
	}
	node := idx.internal.(*fsNode)
	if role == DecorationRole {
		return node.Icon()
	}
	return node.info.Name()
}

func (m *FsModel) Parent(idx *ModelIndex) *ModelIndex {
	if idx == nil {
		return nil
	}
	node := idx.Internal().(*fsNode)
	p := node.parent
	return m.col0Index(p)
}

func (m *FsModel) RowCount(parent *ModelIndex) int {
	if parent == nil {
		return 0
	}
	node := parent.Internal().(*fsNode)
	return node.Count()
}

func (m *FsModel) ColCount() int {

	return 1
}

func (m *FsModel) Flags(idx *ModelIndex) ItemFlags {

	return 0
}

func (m *FsModel) SetRootPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	m.rootPath = path
	m.root = new(fsNode)
	m.root.model = m
	m.root.info = info
	m.root.row = 0
	m.root.List()
	return nil
}
*/

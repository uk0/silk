package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/gv"
	"math"
)

// Brick是Frame里的分格方块
// (原为Tile, 但Tile和Title容易搞混)
type IBrick interface {
	SelfBrick() IBrick
	LeftContainMainDock() bool
	RightContainMainDock() bool
	ContainMainDock() bool
	IsLeftVisible() bool
	IsRightVisible() bool
	IsVisible() bool
	SetBounds(x, y, w, h float64)
	Bounds() (x, y, w, h float64)
	Layout()
	DropRect(split float64, left, vert, merge bool) (xd, yd, wd, hd float64)
	DropSplitHint(xp, yp float64) (split float64, left, vert, merge bool)

	SetSplit(split float64)
	SetVertical(vert bool)
	IsVertical() bool
	Left() IBrick
	SetLeft(t IBrick)
	Right() IBrick
	SetRight(t IBrick)

	ParentBrick() IBrick
	setParentBrick(a IBrick)
	Frame() *Frame
	setFrame(f *Frame)

	Split(t IBrick, split float64, left, vert bool)
	SplitNewDock(left, vert bool) IDock

	Sibling() IBrick
	FindSplitter(x, y float64) IBrick
	SetSplitPoint(x, y float64)

	SaveTDoc() *core.TDoc
	LoadTDoc(doc *core.TDoc)

	//FallowDockPath(path []int) IDock
	check()
}

// Brick是Frame里的分格方块
// (原为Tile, 但Tile和Title容易搞混)
type Brick struct {
	ot          IBrick
	parentBrick IBrick
	frame       *Frame
	left        IBrick
	right       IBrick
	split       float64
	rc          geom.Rect
	vert        bool
}

func newBrick() *Brick {
	p := new(Brick)
	p.ot = p
	return p
}

func (this *Brick) SelfBrick() IBrick {
	return this.ot
}

func (this *Brick) Frame() *Frame {
	//p := this.parentBrick
	//for {
	//	if i, ok := p.(*Frame); ok {
	//		return i
	//	}
	//	if t, ok := p.(*Brick); ok {
	//		p = t.parentBrick
	//	} else {
	//		break
	//	}
	//}
	//return nil
	return this.frame
}

func (this *Brick) setFrame(f *Frame) {
	if this.frame == f {
		return
	}
	this.frame = f
	if this.left != nil {
		this.left.setFrame(f)
	}
	if this.right != nil {
		this.right.setFrame(f)
	}
}

func (this *Brick) ParentBrick() IBrick {
	return this.parentBrick
}

func (this *Brick) setParentBrick(a IBrick) {
	if a == nil {
		this.parentBrick = nil
	} else {
		this.parentBrick = a.SelfBrick()
	}
}

func (this *Brick) SetBounds(x, y, w, h float64) {
	this.rc.X, this.rc.Y, this.rc.Width, this.rc.Height = x, y, w, h
	this.Layout()
}

func (this *Brick) Bounds() (x, y, w, h float64) {
	return this.rc.X, this.rc.Y, this.rc.Width, this.rc.Height
}

func (this *Brick) SetBounds1(rc geom.Rect) {
	this.rc = rc
	this.Layout()
}

func (this *Brick) Bounds1() geom.Rect {
	return this.rc
}

func (this *Brick) IsLeftVisible() bool {
	return this.left != nil && this.left.IsVisible()
}

func (this *Brick) IsRightVisible() bool {
	return this.right != nil && this.right.IsVisible()
}

func (this *Brick) IsVisible() bool {
	return this.IsLeftVisible() || this.IsRightVisible()
}

func (this *Brick) LeftContainMainDock() bool {
	return this.left != nil && this.left.ContainMainDock()
}

func (this *Brick) RightContainMainDock() bool {
	return this.right != nil && this.right.ContainMainDock()
}

func (this *Brick) ContainMainDock() bool {
	return this.LeftContainMainDock() || this.RightContainMainDock()
}

//func (this *Brick) IsMinimized() bool {
//	return (this.left == nil || this.left.IsMinimized()) &&
//		(this.right == nil || this.right.IsMinimized())
//}

func (this *Brick) DropRect(split float64, left, vert, merge bool) (xd, yd, wd, hd float64) {
	if merge {
		xd = this.rc.X + 0
		yd = this.rc.Y + 0
		wd = this.rc.Width - 0
		hd = this.rc.Height - 0
		return
	}
	if vert {
		xd, wd = this.rc.X, this.rc.Width
		if left {
			yd = this.rc.Y
			hd = this.rc.Height * split
		} else {
			yd = this.rc.Y + this.rc.Height*split
			hd = this.rc.Height * (1 - split)
		}
	} else {
		yd, hd = this.rc.Y, this.rc.Height
		if left {
			xd = this.rc.X
			wd = this.rc.Width * split
		} else {
			xd = this.rc.X + this.rc.Width*split
			wd = this.rc.Width * (1 - split)
		}
	}
	return
}

// 假设在xp,yp位置drop一个dock, 判断是否分割, 以及分割的方向和位置
func (this *Brick) DropSplitHint(xp, yp float64) (split float64, left, vert, merge bool) {
	hasMain := this.SelfBrick().ContainMainDock()

	dx := xp - this.rc.X
	dy := yp - this.rc.Y

	if (this.left == nil || this.right == nil) &&
		dx > this.rc.Width*0.382 && dx < this.rc.Width*0.618 &&
		dy > this.rc.Height*0.382 && dy < this.rc.Height*0.618 {
		merge = true
		return
	}

	bottomleft := dy*this.rc.Width > dx*this.rc.Height
	bottomright := dy*this.rc.Width > (this.rc.Width-dx)*this.rc.Height

	if bottomleft {
		if bottomright {
			vert = true
			left = false
			if hasMain {
				split = 0.618
			} else {
				split = 0.5
			}
		} else {
			vert = false
			left = true
			if hasMain {
				split = 0.382
			} else {
				split = 0.5
			}

		}
	} else {
		if bottomright {
			vert = false
			left = false
			if hasMain {
				split = 0.618
			} else {
				split = 0.5
			}

		} else {
			vert = true
			left = true
			if hasMain {
				split = 0.382
			} else {
				split = 0.5
			}
		}
	}
	return
}

//func (this *Brick) doDrop(t IBrick, xp, yp float64) {
//	split, left, vert, merge := this.SplitHint(xp, yp)
//	if merge {
//		core.Warn("unexpected merge operation")
//	}
//	this.Split(t, split, left, vert)
//}

//

func (_this *Brick) Split(t IBrick, split float64, left, vert bool) {
	this := _this.SelfBrick()
	frame := this.Frame()
	//	var p, a IBrick

	newParent := newBrick()

	oldParent := this.ParentBrick()
	if oldParent != nil {
		newParent.setParentBrick(oldParent)
		newParent.setFrame(oldParent.Frame())
		if oldParent.Left() == this {
			oldParent.SetLeft(newParent)
		} else {
			oldParent.SetRight(newParent)
		}
	} else {
		frame.setRootBrick(newParent)
	}
	newParent.setFrame(frame)

	newParent.SetSplit(split)
	core.Debug("newParent.SetVertical(vert): ", vert)
	newParent.SetVertical(vert)
	if left {
		newParent.SetLeft(t)
		newParent.SetRight(this)
	} else {
		newParent.SetLeft(this)
		newParent.SetRight(t)
	}
}

func (this *Brick) SetSplit(split float64) {
	if split < 0.1 {
		this.split = 0.1
	} else if split > 0.9 {
		this.split = 0.9
	} else {
		this.split = split
	}
}

func (this *Brick) IsVertical() bool {
	return this.vert
}
func (this *Brick) SetVertical(vert bool) {
	this.vert = vert
}

func (this *Brick) SetLeft(t IBrick) {
	if this.left != nil {
		this.left.setParentBrick(nil)
		this.left = nil
	}
	if t == nil {
		return
	}
	this.left = t.SelfBrick()
	this.left.setParentBrick(this.SelfBrick())
	this.left.setFrame(this.SelfBrick().Frame())
}

func (this *Brick) SetRight(t IBrick) {
	if this.right != nil {
		this.right.setParentBrick(nil)
		this.right = nil
	}
	if t == nil {
		return
	}
	this.right = t.SelfBrick()
	this.right.setParentBrick(this.SelfBrick())
	this.right.setFrame(this.SelfBrick().Frame())
}

func (this *Brick) Left() IBrick {
	return this.left
}

func (this *Brick) Right() IBrick {
	return this.right
}

func (this *Brick) SizeHints() SizeHints {
	return SizeHints{}
}

func (this *Brick) splitRange() (min, max float64) {
	return
}

func (_this *Brick) Sibling() IBrick {
	this := _this.SelfBrick()
	parent := this.ParentBrick()
	if parent == nil {
		return nil
	}
	this.check()
	if parent.Left() == this {
		parent.Right().check()
		return parent.Right()
	} else {
		parent.Left().check()
		return parent.Left()
	}
}

func (_this *Brick) check() {
	this := _this.SelfBrick()
	if this.ParentBrick() != nil {
		if this.ParentBrick().Left() != this && this.ParentBrick().Right() != this {
			DbgExportGuiGv(true, this.ParentBrick(), this, this.ParentBrick().Right())
			panic("parent.left != this && parent.right != this")
		}
		if this.ParentBrick().Left() == this && this.ParentBrick().Right() == this {
			DbgExportGuiGv(true, this.ParentBrick(), this, this.ParentBrick().Right())
			panic("parent.left == this && parent.right == this")
		}
	}
	if this.Left() != nil && this.Left().ParentBrick() != this {
		DbgExportGuiGv(true, this, this.Left(), this.Left().ParentBrick())
		panic("this.left.parent != this")
	}
	if this.Right() != nil && this.Right().ParentBrick() != this {
		DbgExportGuiGv(true, this, this.Right(), this.Right().ParentBrick())
		panic("this.right.parent != this")
	}
}

func (_this *Brick) Detach() {
	_this.check()
	this := _this.SelfBrick()
	parent := this.ParentBrick()
	if parent == nil {
		return
	}
	parent.check()
	s := this.Sibling()
	s.check()
	this.setParentBrick(nil)
	pp := parent.ParentBrick()
	if pp == nil {
		frame := this.Frame()
		if frame != nil {
			frame.setRootBrick(s)
		}
		return
	}

	pp.check()

	if pp.Left() == parent {
		pp.SetLeft(s)
	} else if pp.Right() == parent {
		pp.SetRight(s)
	} else {
		panic("")
	}

}

func (this *Brick) Layout() {
	lv := this.IsLeftVisible()
	rv := this.IsRightVisible()
	if !lv && !rv {
		return
	}

	if lv && rv {
		//split := math.Floor(this.split + 0.5)
		hsw := Theme().SplitterSize * 0.5
		if this.vert {
			pos := this.rc.Height * this.split
			pos = math.Floor(pos + 0.5)
			if pos < hsw {
				pos = hsw
			} else if pos >= this.rc.Height-hsw {
				pos = this.rc.Height - hsw
			}
			h0 := pos - hsw
			if h0 < 1 {
				h0 = 1
			}
			h1 := this.rc.Height - pos - hsw
			if h1 < 1 {
				h1 = 1
			}
			this.left.SetBounds(this.rc.X, this.rc.Y, this.rc.Width, h0)
			this.right.SetBounds(this.rc.X, this.rc.Y+pos+hsw, this.rc.Width, h1)
		} else {
			pos := this.rc.Width * this.split
			pos = math.Floor(pos + 0.5)
			if pos < hsw {
				pos = hsw
			} else if pos >= this.rc.Width-hsw {
				pos = this.rc.Width - hsw
			}
			w0 := pos - hsw
			if w0 < 1 {
				w0 = 1
			}
			w1 := this.rc.Width - pos - hsw
			if w1 < 1 {
				w1 = 1
			}
			this.left.SetBounds(this.rc.X, this.rc.Y, w0, this.rc.Height)
			this.right.SetBounds(this.rc.X+pos+hsw, this.rc.Y, w1, this.rc.Height)
		}
	} else if lv {
		this.left.SetBounds(this.rc.X, this.rc.Y, this.rc.Width, this.rc.Height)
	} else {
		this.right.SetBounds(this.rc.X, this.rc.Y, this.rc.Width, this.rc.Height)
	}
}

func (this *Brick) FindSplitter(x, y float64) IBrick {
	if x < this.rc.X || x >= this.rc.X+this.rc.Width || y < this.rc.Y || y >= this.rc.Y+this.rc.Height {
		return nil
	}
	hsw := Theme().SplitterSize * 0.5
	if this.vert {
		ys := this.rc.Y + this.rc.Height*this.split
		if y <= ys-hsw {
			if this.left != nil {
				return this.left.FindSplitter(x, y)
			}
		} else if y > ys+hsw {
			if this.right != nil {
				return this.right.FindSplitter(x, y)
			}
		} else {
			return this.ot
		}
	} else {
		xs := this.rc.X + this.rc.Width*this.split
		if x <= xs-hsw {
			if this.left != nil {
				return this.left.FindSplitter(x, y)
			}
		} else if x > xs+hsw {
			if this.right != nil {
				return this.right.FindSplitter(x, y)
			}
		} else {
			return this.ot
		}
	}
	return nil
}

func (this *Brick) SetSplitPoint(x, y float64) {
	if this.vert {
		this.SetSplit((y - this.rc.Y) / this.rc.Height)
	} else {
		this.SetSplit((x - this.rc.X) / this.rc.Width)
	}
}

func (this *Brick) SaveTDoc() *core.TDoc {
	doc := core.NewTDoc()
	doc.SetValue("brick")
	doc.WriteAttr("split", this.split)
	doc.WriteAttr("rect", this.rc)
	doc.WriteAttr("vert", this.vert)
	if this.left != nil {
		p := this.left.SaveTDoc()
		p.SetKey("left")
		doc.AddChild(p)
	}
	if this.right != nil {
		p := this.right.SaveTDoc()
		p.SetKey("right")
		doc.AddChild(p)
	}
	return doc
}

func (this *Brick) LoadTDoc(doc *core.TDoc) {

	doc.ReadAttr("split", &this.split)
	doc.ReadAttr("rect", &this.rc)
	doc.ReadAttr("vert", &this.vert)

	p := doc.ChildByKey("left", false)
	if p != nil {
		var tpy string
		p.Value(&tpy)

		var c IBrick
		if tpy == "brick" {
			c = newBrick()
		} else {
			d := NewDock()
			d.SetParent(this.Frame())
			c = d
		}
		this.SetLeft(c)
		c.LoadTDoc(p)
	}

	p = doc.ChildByKey("right", false)
	if p != nil {
		var tpy string
		p.Value(&tpy)

		var c IBrick
		if tpy == "brick" {
			c = newBrick()
		} else {
			d := NewDock()
			d.SetParent(this.Frame())
			c = d
		}
		this.SetRight(c)
		c.LoadTDoc(p)
	}
}

// 获取当前的路径, 以'1','2','3','4'表示上下左右
// 注: 和普通的二叉树结点路径不同
func DockPath(dock IBrick) (ret []int) {
	for p := dock; p.ParentBrick() != nil; p = p.ParentBrick() {
		if p.ParentBrick().Left() == p {
			if p.ParentBrick().IsVertical() {
				// 上
				ret = append([]int{1}, ret...)
			} else {
				// 左
				ret = append([]int{3}, ret...)
			}
		} else if p.ParentBrick().Right() == p {
			if p.ParentBrick().IsVertical() {
				// 下
				ret = append([]int{2}, ret...)
			} else {
				// 右
				ret = append([]int{4}, ret...)
			}
		} else {
			panic("")
		}
	}
	return
}

func (this *Brick) SplitNewDock(left, vert bool) IDock {
	dock := NewDock()
	dock.SetParent(this.frame)
	// 根据maindock的方向, 确定分割比例
	split := 0.5
	if this.SelfBrick().ContainMainDock() {
		if left {
			split = 0.382
		} else {
			split = 0.618
		}
	}
	this.Split(dock, split, left, vert)
	this.frame.Layout()
	return dock
}

func FallowDockPath(root IBrick, path []int) IDock {
	//core.Debug("root = ", root, "path=", path)
	//if root == nil {
	//	if len(path) == 0 {
	//		return root.Frame().SuggestToolDock()
	//	}
	//	return nil
	//}
	p := root
	for _, n := range path {
		switch n {
		default:
			fallthrough
		case 1:
			// 上
			if p.IsVertical() && p.Left() != nil {
				core.Debug("上")
				p = p.Left()
			} else {
				core.Debug(`SplitNewDock("上")`)
				return p.SplitNewDock(true, true)
			}
		case 2:
			// 下
			if p.IsVertical() && p.Right() != nil {
				core.Debug("下")
				p = p.Right()
			} else {
				core.Debug(`SplitNewDock("下")`)
				return p.SplitNewDock(false, true)
			}
		case 3:
			// 左
			if !p.IsVertical() && p.Left() != nil {
				core.Debug("左")
				p = p.Left()
			} else {
				core.Debug(`SplitNewDock("左")`)
				return p.SplitNewDock(true, false)
			}
		case 4:
			// 右
			if !p.IsVertical() && p.Right() != nil {
				core.Debug("右")
				p = p.Right()
			} else {
				core.Debug(`SplitNewDock("右")`)
				return p.SplitNewDock(false, false)
			}
		}
	}
	core.Debug("find largest")
	return findLargestDock(p)
}

func findLargestDock(brick IBrick) (ret IDock) {
	if dock, ok := brick.(IDock); ok {
		//if dock.IsMainDock() {
		//	return nil
		//}
		return dock
	}

	a := findLargestDock(brick.Left())
	b := findLargestDock(brick.Right())

	if a == nil && b == nil {
		return nil
	}

	if a != nil && b != nil {
		aa := a.Bounds1().Area()
		ba := b.Bounds1().Area()
		if aa < ba {
			return b
		} else {
			return a
		}
	}

	if b != nil {
		return b
	} else {
		return a
	}
}

func (this *Brick) ExportGv(g *gv.Graph) {
	self := this.SelfBrick()
	node := g.Node(self)
	node.Text = core.VisualString(this.Bounds1())
	//node.Text += "\n" + GetDbgText(self)

	node.Color = "lightgreen"
	node.TextColor = node.Color

	{
		edge := g.Edge(node, this.Left())
		edge.Color = node.Color
		edge.TextColor = edge.Color
		edge.Text = "L"
		edge.Weight = 10
	}
	{
		edge := g.Edge(node, this.Right())
		edge.Color = node.Color
		edge.TextColor = edge.Color
		edge.Text = "R"
		edge.Weight = 10
	}

	{
		edge := g.Edge(node, this.ParentBrick())
		edge.Color = "lightblue"
		edge.TextColor = edge.Color
		edge.Text = "P"
		edge.Weight = 0
	}
}

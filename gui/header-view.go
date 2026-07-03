package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/paint"
	//	"errors"
	"sort"
)

func init() {
	core.RegisterFactory("gui.HeaderView", core.TypeOf((*HeaderView)(nil))) //((*HeaderView)(nil)))
}

// 表头视图的一节
type HeaderViewSection struct {
	Offset      float64
	Size        float64
	LogicIndex  int // 和Model里的列号对应
	VisualIndex int // 用户看到的列序号, 隐藏的也编号
	Hidden      bool
}

// 表头视图
type HeaderView struct {
	GuiView

	model IGuiModel

	sections []*HeaderViewSection

	ini string

	scrollOffset float64
}

func NewHeaderView() *HeaderView {
	p := new(HeaderView)
	p.Init(p)
	return p
}

func (this *HeaderView) Init(iw IWidget) {
	this.GuiView.Init(iw)
}

func (this *HeaderView) Model() IGuiModel {
	return this.model
}

func (this *HeaderView) SetModel(m IGuiModel) {
	this.model = m
}

func (this *HeaderView) OnBeginReset() {
	this.sections = nil
}

func (this *HeaderView) OnEndReset() {

}

func (this *HeaderView) getSectionText(sec *HeaderViewSection) string {
	d := this.model.HeaderData(sec.LogicIndex, false, DisplayRole)
	return core.VisualString(d)
}

func (this *HeaderView) calcSectionSize(sid int) (sz float64, ok bool) {
	x := this.model.HeaderData(sid, false, SizeHintRole)
	if vec, b := x.(geom.Vec2); b {
		sz = vec.X
		if sz < 8 {
			sz = 8
		}
		ok = true
		return
	}
	sz = 128
	ok = false
	return
}

func (this *HeaderView) TotalSectionSize() (sz float64) {
	for _, p := range this.sections {
		sz += p.Size
	}
	return sz
}

func (this *HeaderView) AutoSectionSize(visualIndex int) {
}

func (this *HeaderView) SectionCount() int {
	return len(this.sections)
}

func (this *HeaderView) VisualSection(vid int) HeaderViewSection {
	return *(this.sections[vid])
}

// LogicSection returns the section whose LogicIndex matches sid.
// If no section matches (e.g. a stale or out-of-range logic index) it returns
// the zero-value HeaderViewSection sentinel instead of panicking, so a bad
// index can never crash the host application. A zero-value result has
// Size == 0 and Hidden == false; callers that must distinguish a real match
// can compare the returned LogicIndex against the requested sid.
func (this *HeaderView) LogicSection(sid int) HeaderViewSection {
	for i := 0; i < this.SectionCount(); i++ {
		if this.sections[i].LogicIndex == sid {
			return *(this.sections[i])
		}
	}
	return HeaderViewSection{}
}

func (this *HeaderView) SetScrollOffset(offset float64) {
	this.scrollOffset = offset
	this.Layout()
}

func (this *HeaderView) ScrollOffset() (offset float64) {
	offset = this.scrollOffset
	return
}

func (this *HeaderView) Layout() {
	if this.model == nil {
		return
	}
	cc := this.model.ColCount()
	tsz := this.TotalSectionSize()
	for i := len(this.sections); i < cc; i++ {
		sec := new(HeaderViewSection)
		sec.LogicIndex = i
		sec.VisualIndex = i
		sec.Offset = tsz
		sec.Size, _ = this.calcSectionSize(sec.LogicIndex)
		tsz += sec.Size
		this.sections = append(this.sections, sec)
	}

	if len(this.sections) > cc {
		var secs []*HeaderViewSection
		var pos float64
		for _, p := range this.sections {
			if p.LogicIndex >= cc {
				continue
			}
			secs = append(secs, p)
			p.Offset = pos
			pos += p.Size
		}
		this.sections = secs
	}

	//core.Debug(this.sections)
}

func (this *HeaderView) SetIniPath(path string) {
	this.ini = path
}

func (this *HeaderView) IniPath() string {
	return this.ini
}

func (this *HeaderView) Draw(g paint.Painter) {
	g.Save()
	defer g.Restore()

	g.Translate(-this.ScrollX(), -this.ScrollY())
	g.SetFont(Theme().Font)
	fe := Theme().Font.FontExtents()
	y := fe.Ascent + 0.5*(this.h-fe.Height)
	for _, p := range this.sections {
		if p.Hidden {
			continue
		}
		x := p.Offset
		g.Translate(x, 0)
		Theme().ButtonPushedFace.Draw(g, p.Size, this.h)
		g.Translate(-x, 0)
		g.SetBrush1(Theme().TextColor)
		g.DrawText1(x, y, this.getSectionText(p))
	}
}

func (this *HeaderView) logicIndexAtScrolled(pos float64) int {
	return sort.Search(this.SectionCount(), func(i int) bool {
		return this.sections[i].Offset+this.sections[i].Size >= pos
	})
}

func (this *HeaderView) LogicIndexAt(pos float64) int {
	x := pos + this.ScrollX()
	return this.logicIndexAtScrolled(x)
}

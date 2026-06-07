package ged

import (
	"silk/core"
	"silk/gui"
	"silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("ged.ShortcutsPanel", gui.TypeOf(ShortcutsPanel{}))
	gui.RegisterToolView(gui.ToolViewDef{
		Id:   "ged.ShortcutsPanel",
		Name: "快捷键",
		Icon: "edit",
		Desc: "快捷键参考面板",
	})
}

// shortcutEntry holds one key binding and its description.
type shortcutEntry struct {
	keys        string
	description string
}

// shortcutCategory groups related shortcuts under a heading.
type shortcutCategory struct {
	name      string
	shortcuts []shortcutEntry
}

// ShortcutsPanel displays a keyboard shortcut reference card,
// organized by category, similar to Qt Creator's shortcut viewer.
type ShortcutsPanel struct {
	gui.Widget
	categories []shortcutCategory
	scrollY    float64
	hoverIdx   int
}

func NewShortcutsPanel() *ShortcutsPanel {
	p := new(ShortcutsPanel)
	p.Init(p)
	return p
}

func (this *ShortcutsPanel) Init(self gui.IWidget) {
	this.Widget.Init(self)
	this.hoverIdx = -1
	this.scrollY = 0

	this.categories = []shortcutCategory{
		{
			name: "文件",
			shortcuts: []shortcutEntry{
				{"Ctrl+N", "新建"},
				{"Ctrl+O", "打开"},
				{"Ctrl+S", "保存"},
				{"Ctrl+Shift+S", "另存为"},
			},
		},
		{
			name: "编辑",
			shortcuts: []shortcutEntry{
				{"Ctrl+Z", "撤销"},
				{"Ctrl+Y", "重做"},
				{"Ctrl+C", "复制"},
				{"Ctrl+V", "粘贴"},
				{"Ctrl+X", "剪切"},
				{"Delete", "删除"},
			},
		},
		{
			name: "视图",
			shortcuts: []shortcutEntry{
				{"Ctrl+1", "设计模式"},
				{"Ctrl+2", "代码模式"},
				{"Ctrl+=", "放大"},
				{"Ctrl+-", "缩小"},
			},
		},
		{
			name: "设计",
			shortcuts: []shortcutEntry{
				{"Tab", "下一个控件"},
				{"Shift+Tab", "上一个控件"},
				{"Alt+L/R/T/B", "对齐"},
				{"Alt+H/V", "分布"},
			},
		},
		{
			name: "代码",
			shortcuts: []shortcutEntry{
				{"Ctrl+F", "查找"},
				{"Ctrl+G", "跳转行"},
				{"Ctrl+Shift+O", "符号"},
				{"Ctrl+/", "注释"},
				{"Ctrl+D", "复制行"},
			},
		},
		{
			name: "运行",
			shortcuts: []shortcutEntry{
				{"F5", "编译运行"},
				{"Ctrl+R", "预览"},
				{"Ctrl+Shift+F", "格式化"},
			},
		},
	}
}

func (this *ShortcutsPanel) Draw(g paint.Painter) {
	t := gui.Theme()
	w, h := this.Size()

	// Background
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(t.ViewBGColor)
	g.Fill()

	titleFont := paint.NewFont(t.Font.Family(), 13, true, false)
	keyFont := paint.NewFont(t.Font.Family(), 11, true, false)
	descFont := paint.NewFont(t.Font.Family(), 11, false, false)

	y := 8.0 - this.scrollY
	const (
		leftPad      = 12.0
		keyColWidth  = 130.0
		rowH         = 20.0
		catGap       = 10.0
		catHeaderH   = 24.0
	)

	for _, cat := range this.categories {
		// Category header
		g.SetFont(titleFont)
		g.SetBrush1(t.HighLightColor)
		g.Translate(leftPad, y+16)
		g.DrawText(cat.name)
		g.Translate(-leftPad, -(y + 16))

		// Underline
		g.SetPen1(t.HighLightColor, 1)
		g.Line(leftPad, y+catHeaderH, w-leftPad, y+catHeaderH)
		g.Stroke()

		y += catHeaderH + 4

		for _, sc := range cat.shortcuts {
			if y+rowH > 0 && y < h {
				// Key badge background
				keyExt := keyFont.TextExtents(sc.keys)
				badgeW := keyExt.Width + 10
				badgeH := rowH - 4
				badgeX := leftPad
				badgeY := y + 2

				// Rounded-rect key badge
				g.Rectangle(badgeX, badgeY, badgeW, badgeH)
				g.SetBrush1(paint.Color{t.FormColor.R, t.FormColor.G, t.FormColor.B, 255})
				g.FillPreserve()
				g.SetPen1(t.BorderColor, 1)
				g.Stroke()

				// Key text
				g.SetFont(keyFont)
				g.SetBrush1(t.TextColor)
				kx := badgeX + 5
				ky := badgeY + badgeH*0.5 + keyExt.Height*0.5
				g.Translate(kx, ky)
				g.DrawText(sc.keys)
				g.Translate(-kx, -ky)

				// Description
				g.SetFont(descFont)
				g.SetBrush1(t.TextColor)
				dx := leftPad + keyColWidth
				dy := badgeY + badgeH*0.5 + keyExt.Height*0.5
				g.Translate(dx, dy)
				g.DrawText(sc.description)
				g.Translate(-dx, -dy)
			}
			y += rowH
		}
		y += catGap
	}
}

func (this *ShortcutsPanel) OnMouseWheel(x, y, delta float64) {
	this.scrollY -= delta * 20
	this.scrollY = math.Max(0, this.scrollY)
	// Clamp to content height
	maxScroll := this.contentHeight() - this.Height()
	if maxScroll < 0 {
		maxScroll = 0
	}
	this.scrollY = math.Min(this.scrollY, maxScroll)
	this.Self().Update()
}

func (this *ShortcutsPanel) contentHeight() float64 {
	const (
		rowH       = 20.0
		catGap     = 10.0
		catHeaderH = 24.0
	)
	h := 8.0
	for _, cat := range this.categories {
		h += catHeaderH + 4
		h += float64(len(cat.shortcuts)) * rowH
		h += catGap
	}
	return h
}

func (this *ShortcutsPanel) SizeHints() gui.SizeHints {
	return gui.SizeHints{Width: 300, Height: 400}
}

package prop

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

// Translate display names, not persisted property IDs or explicit user labels.
var legacyPropertyLabels = map[string]string{
	"控件名称": "Object Name", "控件类型": "Widget Type", "类型": "Type", "标题": "Title",
	"X 位置 (mm)": "X Position (mm)", "Y 位置 (mm)": "Y Position (mm)",
	"宽度 (mm)": "Width (mm)", "高度 (mm)": "Height (mm)",
	"锁定": "Locked", "可用": "Enabled", "可见": "Visible", "图标": "Icon", "文本": "Text",
	"上边距": "Top Margin", "下边距": "Bottom Margin", "左边距": "Left Margin", "右边距": "Right Margin", "间距": "Spacing",
}

func init() {
	translations := map[string]string{multiValueText: "多值"}
	for id, label := range legacyPropertyLabels {
		translations[label] = id
	}
	i18n.AddMany("zh-CN", translations)
}

func propertyDisplayName(id string) string {
	if label, ok := legacyPropertyLabels[id]; ok {
		return i18n.T(label)
	}
	return i18n.T(id)
}

// Elision communicates truncation; clipping also contains overhanging glyphs.
// Shared by labels, compact numbers and mixed-value hints in narrow panels.
func drawPropertyText(g paint.Painter, font paint.Font, color paint.Color, x, y, width, height float64, text string) {
	if font == nil || width <= 0 || height <= 0 {
		return
	}
	fitted := gui.ElideText(font, text, width)
	g.Save()
	defer g.Restore()
	g.Rectangle(x, y, width, height)
	g.Clip()
	g.SetFont(font)
	g.SetBrush1(color)
	fe := font.FontExtents()
	g.DrawText1(x, y+(height+fe.Ascent-fe.Descent)/2, fitted)
}

func (sheet *PropertySheet) drawPropertyLabel(g paint.Painter, p *PropertyItem) {
	r := sheet.getItemRect(p, pitemLabel)
	width := r.Width - 4
	if isColorProperty(p) {
		width -= 18
	}
	drawPropertyText(g, gui.Theme().Font, gui.Theme().TextColor, r.X, r.Y, width, r.Height, p.Label())
}

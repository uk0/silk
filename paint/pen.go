package paint

type LineStyle interface {
	Width() float64
}

// 考虑到兼容性, 我们的画笔只支持纯色, 不支持图案填充
type Pen interface {
	Width() float64
	Color() Color
}

type pen struct {
	color Color
	width float64
}

func (p *pen) Color() Color {
	return p.color
}

func (p *pen) Width() float64 {
	return p.width
}

func NewPen(cr Color, width float64) *pen {
	return &pen{cr, width}
}

//func NewPen4(width float64, r, g, b uint8) *pen {
//	return &pen{width, Color{r, g, b, 255}}
//}

//func NewPen5(width float64, r, g, b, a uint8) *pen {
//	return &pen{width, Color{r, g, b, a}}
//}

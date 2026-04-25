package gui

import (
	//	"silk/diag"
	"silk/paint"
	//"strings"
	"unicode"
)

// 抽象的多行文本块
// 支持自动折行等操作
type TextBlock struct {
	text []rune
	rows []sRow
	font paint.Font
	fe   *paint.FontExtents
	//pos  int
	pw   float64
	ml   bool
	wrap bool
}

type sRow struct {
	text   []rune
	begin  int
	end    int
	glyphs []paint.Glyph
}

func (this *TextBlock) RunesCount() int {
	if this.text == nil {
		return 0
	}
	return len(this.text) - 1
}

func (this *TextBlock) Replace(begin, end int, s string) (caret int, old string) {
	if this.text == nil {
		this.SetText(s)
		return len([]rune(s)), ""
	}
	rc := this.RunesCount()
	if end < begin {
		t := begin
		begin = end
		end = t
	}
	if begin < 0 {
		begin = 0
	} else if begin > rc {
		begin = rc
	}
	if end < begin {
		end = begin
	} else if end > rc {
		end = rc
	}
	//core.Debug(begin, end)
	old = string(this.text[begin:end])
	a := this.text[:begin]
	b := []rune(s)
	c := make([]rune, len(this.text)-end)
	copy(c, this.text[end:])
	caret = begin + len(b)
	this.text = append(a, b...)
	this.text = append(this.text, c...)
	//core.Debug("this.text: ", string(this.text))
	//this.layout()
	return
}

func (this *TextBlock) selText(begin, end int) string {
	if this.text == nil {
		return ""
	}
	rc := this.RunesCount()
	if end < begin {
		t := begin
		begin = end
		end = t
	}
	if begin < 0 {
		begin = 0
	} else if begin > rc {
		begin = rc
	}
	if end < begin {
		end = begin
	} else if end > rc {
		end = rc
	}
	return string(this.text[begin:end])
}

func (this *TextBlock) SetText(s string) {
	this.text = []rune(s + "\n")
}

func (this *TextBlock) Text() string {
	if this.text == nil {
		return ""
	}
	return string(this.text[:len(this.text)-1])
}

func (this *TextBlock) String() string {
	return this.Text()
}

//func (this *TextBlock) layout() {
//	this.Layout(this.pw)
//}

//func (this *TextBlock) needPrepare() {
//	this.rows = nil
//	//	this.Ow().Update()
//}

func (this *TextBlock) Font() paint.Font {
	if this.font == nil {
		return Theme().Font
	}
	return this.font
}

func (this *TextBlock) SetFont(font paint.Font) {
	this.font = font
	this.fe = nil
}

func (this *TextBlock) MultiLine() bool {
	return this.ml
}

func (this *TextBlock) SetMultiLine(b bool) {
	this.ml = b
}

func (this *TextBlock) Warp() bool {
	return this.wrap
}

func (this *TextBlock) SetWrap(b bool) {
	if b {
		this.SetMultiLine(true)
	}
	this.wrap = b
}

func isLetter(r rune) bool {
	return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func canBreak(a, b rune) bool {
	la := isLetter(a)
	if la && isLetter(b) {
		return false
	}
	if la && (b == '-' || unicode.IsSpace(b)) {
		return false
	}
	return true
}

func findLineEnd(s []rune) int {
	for i, v := range s {
		if v == '\r' || v == '\n' {
			return i
		}
	}
	return -1
}

func (this *TextBlock) Layout(w float64) {

	wrap := this.wrap && this.ml
	//if wrap && w != this.pw {
	this.rows = nil
	//}
	this.pw = w
	//if this.rows == nil {
	//	this.pos = 0
	//}
	ln := 0
	//if ln >= toLn {
	//	return
	//}

	total := len(this.text) - 1
	//if this.pos >= total {
	//	return
	//}

	sf := this.Font().ScaledFont(nil)
	fe := this.fontExtents()

	begin := 0
	s := this.text[begin:]

	tmp := sf.TextToGlyphs(0, 0, " ")
	blank := tmp[0]

	y := float64(ln) * fe.Height
	for len(s) > 0 {
		var pos int
		if this.ml {
			pos = findLineEnd(s)
		} else {
			pos = total
		}
		if pos == -1 {
			// 这是额外附加的空行
			break
		}
		var n = pos + 1
		if s[pos] == '\n' {
			if pos+1 < len(s) && s[pos+1] == '\r' {
				n = pos + 2
			}
		} else {
			if pos+1 < len(s) && s[pos+1] == '\n' {
				n = pos + 2
			}
		}
		wraped := false
		text := s[:pos+1]
		glyphs := sf.TextToGlyphs(0, y+fe.Ascent, string(text))
		if wrap {
			i := len(glyphs) - 1
			for i > 1 && glyphs[i].X >= w {
				i--
			}
			if i < len(glyphs)-1 {
				j := i
				for j > 1 && !canBreak(text[j-1], text[j]) {
					j--
				}
				if j > 1 {
					i = j
				}
				pos = pos - len(glyphs) + 1 + i
				n = pos
				wraped = true
			}
		}
		row := sRow{
			s[:pos],
			begin, begin + n,
			make([]paint.Glyph, 0, pos+1)} // 用新的slice节省内存, 因为旧的有冗余
		for j := 0; j < pos; j++ {
			g := glyphs[j]
			g.A = uint32(glyphs[j+1].X - g.X)
			row.glyphs = append(row.glyphs, g)
		}
		if !wraped {
			// 在行尾加空格做换行符, 因回车字符可能可见, 所以不能直接用回车
			b1 := blank
			b1.X = glyphs[pos].X
			b1.Y = glyphs[pos].Y
			b1.A = 165536 // 超宽的换行符, 保证点行尾时不跑到另一行
			row.glyphs = append(row.glyphs, b1)
		}
		this.rows = append(this.rows, row)
		begin += n
		s = s[n:]
		ln++
		y += fe.Height
	}

	//	this.pos = begin
}

func (this *TextBlock) fontExtents() *paint.FontExtents {
	if this.fe == nil {
		this.fe = this.Font().FontExtents()
	}
	return this.fe
}

func (this *TextBlock) RowHeight() float64 {
	return this.fontExtents().Height
}

func (this *TextBlock) SoftRowsCount() int {
	//	this.prepare(this.pw, 1<<30)
	return len(this.rows)
}

func (this *TextBlock) PosToPoint(pos int) (x, y float64) {
	if this.text == nil || pos <= 0 {
		return 0, 0
	}
	rh := this.RowHeight()
	rc := this.SoftRowsCount()
	for r, row := range this.rows {
		if pos >= row.begin && pos < row.end {
			y = float64(r)*rh + rh*0.5
			c := pos - row.begin
			if c >= len(row.glyphs) {
				c = len(row.glyphs) - 1
				x = row.glyphs[c].X + float64(row.glyphs[c].A)
			} else {
				x = row.glyphs[c].X
			}

			return
		}
	}
	r := rc - 1
	y = float64(r)*rh + rh*0.5
	row := this.rows[r]
	c := row.end - row.begin
	if c >= len(row.glyphs) {
		c = len(row.glyphs) - 1
		x = row.glyphs[c].X + float64(row.glyphs[c].A)
	} else {
		x = row.glyphs[c].X
	}
	return

}

func (this *TextBlock) PointToPos(x, y float64) int {
	if this.text == nil || y < 0 {
		return 0
	}

	rh := this.RowHeight()
	var r int
	if this.ml {
		r = int(y / rh)
	} else {
		r = 0
	}
	rc := this.SoftRowsCount()
	if r >= rc {
		return this.RunesCount()
	}
	row := this.rows[r]
	x1 := 0.0
	for i, g := range row.glyphs {
		if x < x1+float64(g.A)*0.5 {
			return row.begin + i
		}
		x1 += float64(g.A)
	}
	return row.end
}

func (this *TextBlock) RowColToPos(r, c int) (pos int) {
	if this.text == nil || r < 0 {
		return 0
	}
	rc := this.SoftRowsCount()
	if r >= rc {
		return this.RunesCount()
	}
	row := this.rows[r]
	if c >= row.end-row.begin {
		c = row.end - row.begin - 1
	}
	if c < 0 {
		c = 0
	}
	pos = row.begin + c
	//if pos < 0 {
	//	pos = 0
	//} else if pos > this.RunesCount() {
	//	pos = this.RunesCount()
	//}
	return
}

func (this *TextBlock) PosToRowCol(pos int) (r, c int) {
	if this.text == nil || pos <= 0 {
		return 0, 0
	}
	rc := this.SoftRowsCount()
	for r, row := range this.rows {
		if pos >= row.begin && pos < row.end {
			return r, pos - row.begin
		}
	}
	lasRow := rc - 1
	return lasRow, this.rows[lasRow].end - this.rows[lasRow].begin
}

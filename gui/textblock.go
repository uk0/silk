package gui

import (
	//	"github.com/uk0/silk/diag"
	"github.com/uk0/silk/paint"
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
	text       []rune
	begin, end int
	// Positions share the logical line origin; slices survive wrapping without
	// copies. Native glyph padding must never carry GUI advance metadata.
	carets   []float64
	runs     []paint.GlyphRun
	origin   float64
	clusters []paint.TextCluster
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

// Layout shapes each logical line once, retaining each glyph's owning font.
// Wrapping walks the retained runs monotonically: neither suffix shaping nor
// a full run scan is repeated for every visual row.
func (this *TextBlock) Layout(w float64) {
	this.rows = nil
	this.pw = w
	if len(this.text) == 0 {
		return
	}
	wrap := this.wrap && this.ml && w > 0
	sf := this.Font().ScaledFont(nil)
	for begin := 0; begin < len(this.text); {
		pos := len(this.text) - begin - 1
		if this.ml {
			pos = findLineEnd(this.text[begin:])
		}
		if pos < 0 {
			break
		}
		end := begin + pos
		next := end + 1
		if next < len(this.text)-1 && ((this.text[end] == '\r' && this.text[next] == '\n') || (this.text[end] == '\n' && this.text[next] == '\r')) {
			next++
		}
		text := this.text[begin:end]
		layout := paint.LayoutText(sf, string(text))
		runIndex, clusterIndex := 0, 0
		for start := 0; ; {
			stop := pos
			origin := layout.Carets[start]
			if wrap && start < pos {
				stop = start + 1
				for stop < pos && layout.Carets[stop+1]-origin <= w {
					stop++
				}
				if stop < pos {
					j := stop
					for j > start && !canBreak(text[j-1], text[j]) {
						j--
					}
					if j > start {
						stop = j
					}
				}
			}
			stop = layout.WrapEnd(start, stop)
			row := sRow{text: text[start:stop], begin: begin + start, end: begin + stop,
				carets: layout.Carets[start : stop+1], origin: origin}
			for clusterIndex < len(layout.Clusters) && layout.Clusters[clusterIndex].End <= start {
				clusterIndex++
			}
			for j := clusterIndex; j < len(layout.Clusters) && layout.Clusters[j].Begin < stop; j++ {
				c := layout.Clusters[j]
				row.clusters = append(row.clusters, paint.TextCluster{Begin: c.Begin - start, End: c.End - start})
			}
			for runIndex < len(layout.Runs) && layout.Runs[runIndex].End <= start {
				runIndex++
			}
			for j := runIndex; j < len(layout.Runs) && layout.Runs[j].Begin < stop; j++ {
				run := layout.Runs[j]
				if len(run.Glyphs) == run.End-run.Begin {
					run.Glyphs = run.Glyphs[max(start-run.Begin, 0):min(stop-run.Begin, len(run.Glyphs))]
				}
				row.runs = append(row.runs, run)
			}
			if stop == pos {
				row.end = next
			}
			this.rows = append(this.rows, row)
			if stop == pos {
				break
			}
			start = stop
		}
		begin = next
	}
}

func (row *sRow) caretX(col int) float64 {
	if len(row.carets) == 0 {
		return 0
	}
	return row.carets[max(0, min(col, len(row.carets)-1))] - row.origin
}

func (row *sRow) draw(g paint.Painter, baseline float64) {
	g.Save()
	defer g.Restore()
	g.Translate(-row.origin, baseline)
	for _, run := range row.runs {
		g.SetFont(run.Font)
		g.DrawGlyphs(run.Glyphs)
	}
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
	if len(this.rows) == 0 || pos <= 0 {
		return 0, 0
	}
	pos = min(pos, this.RunesCount())
	r, c := this.PosToRowCol(pos)
	return this.rows[r].caretX(c), (float64(r) + .5) * this.RowHeight()
}

func (this *TextBlock) PointToPos(x, y float64) int {
	if len(this.rows) == 0 || y < 0 {
		return 0
	}
	r := 0
	if this.ml && this.RowHeight() > 0 {
		r = int(y / this.RowHeight())
	}
	if r >= len(this.rows) {
		return this.RunesCount()
	}
	row := &this.rows[r]
	cluster := 0
	for i := 0; i < len(row.text); {
		next := i + 1
		if cluster < len(row.clusters) && row.clusters[cluster].Begin == i {
			next = row.clusters[cluster].End
			cluster++
		}
		if x < (row.caretX(i)+row.caretX(next))*.5 {
			return row.begin + i
		}
		i = next
	}
	// Newline/CRLF and the final sentinel have zero visible width. Clicking
	// beyond the ink belongs to this line, never to an invisible giant glyph.
	return min(row.begin+len(row.text), this.RunesCount())
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
	if len(this.rows) == 0 || pos <= 0 {
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

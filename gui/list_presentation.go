package gui

import (
	"github.com/uk0/silk/paint"
	"math"
)

// ListItemPresentation opts a ListWidget into two-line rows without changing
// ListItem or its selection, activation, icon and checkbox contracts. Text
// remains the primary label; SecondaryText describes it; TrailingText reserves
// a separate right-aligned slot. Muted is visual only, NOT an activation guard.
type ListItemPresentation struct {
	SecondaryText string
	TrailingText  string
	Muted         bool
}

type listPresentationKey struct {
	width        float64
	font         paint.Font
	family       string
	size         int
	bold, italic bool
	theme        uint64
}
type listPresentationRow struct {
	title, secondary, trailing                      string
	mainWidth, trailingWidth                        float64
	muted                                           bool
	titleBearing, secondaryBearing, trailingBearing float64
}

// SetItemPresenter opts into measured, clipped two-line rows (nil restores the
// legacy single-line presentation). Called only for visible cache misses on the
// UI thread. Keep callbacks pure; call InvalidateItemPresentation if external
// presentation data changes. List mutations, width, font and theme invalidate
// automatically. The cache is bounded, independently of the item count.
func (l *ListWidget) SetItemPresenter(fn func(int, ListItem) ListItemPresentation) {
	l.itemPresenter = fn
	l.InvalidateItemPresentation()
	l.Layout()
	l.InvalidateParentLayout()
}

func (l *ListWidget) InvalidateItemPresentation() {
	l.presentationRows = nil
	l.Update()
}

func (l *ListWidget) presentedRow(index int, width float64) listPresentationRow {
	f := l.Font()
	key := listPresentationKey{width, f, f.Family(), f.Size(), f.Bold(), f.Italic(), themeRev}
	if key != l.presentationKey {
		l.presentationRows = nil
		l.presentationKey = key
	}
	if row, ok := l.presentationRows[index]; ok {
		return row
	}
	data := l.itemPresenter(index, l.items[index])
	row := listPresentationRow{mainWidth: width, muted: data.Muted}
	if data.TrailingText != "" {
		row.trailing = ElideText(f, data.TrailingText, math.Max(0, math.Floor(width*.45)-2))
		if row.trailing != "" {
			row.trailingWidth = math.Ceil(f.TextExtents(row.trailing).Width) + 2
			row.mainWidth = math.Max(0, width-row.trailingWidth-8)
		}
	}
	row.title = ElideText(f, l.items[index].Text, row.mainWidth-2)
	row.secondary = ElideText(f, data.SecondaryText, width-2)
	// Reserve a pixel of rasterization slack on each side. Ink extents do
	// not include antialiasing coverage, and fractional negative origins can
	// bleed on the native font backend. Round the bearing down so labels use
	// integer origins while their ink remains inside the padded slot.
	row.titleBearing = f.TextExtents(row.title).XBearing
	row.secondaryBearing = f.TextExtents(row.secondary).XBearing
	row.trailingBearing = f.TextExtents(row.trailing).XBearing
	if l.presentationRows == nil || len(l.presentationRows) >= 128 {
		l.presentationRows = make(map[int]listPresentationRow)
	}
	l.presentationRows[index] = row
	return row
}

func (l *ListWidget) drawPresentedRow(g paint.Painter, index int, x, y, width, height float64, selected bool) {
	if width <= 0 || height <= 0 {
		return
	}
	row := l.presentedRow(index, width)
	f := l.Font()
	fe := f.FontExtents()
	line := math.Max(1, fe.Height)
	top := y + math.Max(0, (height-2*line)*.5)
	ink := selectionTextColor(Theme(), selected)
	titleInk := ink
	if row.muted && !selected {
		titleInk = Theme().MenuGrayTextColor
	}
	draw := func(text string, tx, ty, w, bearing float64, color paint.Color) {
		if text == "" || w <= 0 {
			return
		}
		g.Save()
		g.Rectangle(tx, ty, w, line)
		g.Clip()
		g.SetBrush1(color)
		g.DrawText1(tx+1-math.Floor(bearing), ty+fe.Ascent, text)
		g.Restore()
	}
	draw(row.title, x, top, row.mainWidth, row.titleBearing, titleInk)
	draw(row.trailing, x+width-row.trailingWidth, top, row.trailingWidth, row.trailingBearing, ink)
	draw(row.secondary, x, top+line, width, row.secondaryBearing, ink)
}

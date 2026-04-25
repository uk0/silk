package geom

import "math"
import "fmt"
import "errors"

//import "strconv"

type Rect struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

//func (a Rect) Pos() (x, y float64) {
//	return a.X, a.Y
//}

//func (a Rect) Size() (width, height float64) {
//	return a.Width, a.Height
//}

func (a Rect) IsIntersects(b Rect) bool {
	return a.IntersectCopy(b).IsEmpty()
}

func (a Rect) IntersectCopy(b Rect) (ret Rect) {
	a.Normalize()
	b.Normalize()

	x0 := math.Max(a.X, b.X)
	x1 := math.Min(a.Right(), b.Right())
	if x1 < x0 {
		return
	}

	y0 := math.Max(a.Y, b.Y)
	y1 := math.Min(a.Bottom(), b.Bottom())
	if y1 < y0 {
		return
	}

	ret.X = x0
	ret.Y = y0
	ret.Width = x1 - x0
	ret.Height = y1 - y0
	return
}

func (a Rect) UniteCopy(b Rect) (ret Rect) {
	a.Normalize()
	b.Normalize()

	x0 := math.Min(a.X, b.X)
	x1 := math.Max(a.Right(), b.Right())
	y0 := math.Min(a.Y, b.Y)
	y1 := math.Max(a.Bottom(), b.Bottom())

	ret.X = x0
	ret.Y = y0
	ret.Width = x1 - x0
	ret.Height = y1 - y0

	return
}

func (a Rect) IsNormal() bool {
	return a.Width >= 0 && a.Height >= 0
}

func (a *Rect) Normalize() {
	if a.Width < 0 {
		a.X += a.Width
		a.Width = -a.Width
	}

	if a.Height < 0 {
		a.Y += a.Height
		a.Height = -a.Height
	}
}

func (a Rect) NormalizeCopy() (r Rect) {
	r = a
	r.Normalize()
	return
}

func (a Rect) ShrinkCopy(offset float64) Rect {
	return a.AdjustCopy(offset, -offset, offset, -offset)
}

func (a Rect) ExpandCopy(offset float64) Rect {
	return a.AdjustCopy(-offset, offset, -offset, offset)
}

func (a Rect) AdjustCopy(dx1, dx2, dy1, dy2 float64) (r Rect) {
	r.X = a.X + dx1
	r.Y = a.Y + dy1
	r.Width = a.Width - dx1 + dx2
	r.Height = a.Height - dy1 + dy2
	return
}

func (a Rect) Center() (x, y float64) {
	return a.X + a.Width*0.5, a.Y + a.Height*0.5
}

func (a Rect) Center1() Vec2 {
	return Vec2{a.X + a.Width*0.5, a.Y + a.Height*0.5}
}

func (a Rect) Contains(x, y float64) bool {
	return x >= a.X && y >= a.Y && x <= a.Right() && y <= a.Bottom()
}

func (a Rect) Left() float64 {
	return a.X
}

func (a Rect) Top() float64 {
	return a.Y
}

func (a Rect) Bottom() float64 {
	return a.Y + a.Height
}

func (a Rect) Right() float64 {
	return a.X + a.Width
}

func (a Rect) Area() float64 {
	return a.Width * a.Height
}

func (a Rect) IsEmpty() bool {
	return a.Width == 0 || a.Height == 0
}

func (a Rect) IsZero() bool {
	return a.X == 0 && a.Y == 0 && a.Width == 0 && a.Height == 0
}

func (a Rect) String() string {
	return fmt.Sprint("[", a.X, a.Y, a.Width, a.Height, "]")
}

func (this *Rect) Scan(state fmt.ScanState, verb rune) error {
	this.X, this.Y, this.Width, this.Height = 0, 0, 0, 0

	state.SkipSpace()
	r, _, _ := state.ReadRune()

	if r != '[' {
		return errors.New("Error in scaning geom.Rect: '[' expected.")
	}

	tok, err := state.Token(true, func(r rune) bool { return r != ']' })
	if err != nil {
		return errors.New("Error in scaning geom.Rect: " + err.Error())
	}

	_, err = fmt.Sscan(string(tok), &this.X, &this.Y, &this.Width, &this.Height)
	if err != nil {
		return errors.New("Error in scaning geom.Rect: " + err.Error())
	}

	state.SkipSpace()
	r, _, _ = state.ReadRune()

	if r != ']' {
		return errors.New("Error in scaning geom.Rect: ']' expected.")
	}

	return nil
}

func (this *Rect) Size() (float64, float64) {
	return this.Width, this.Height
}

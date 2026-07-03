package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.Rating", core.TypeOf((*Rating)(nil)))
}

// Rating is a star rating control that displays filled/empty circles
// and allows the user to select a rating by clicking.
type Rating struct {
	Widget
	value           int
	maxStars        int
	readonly        bool
	hoverValue      int
	cbRatingChanged func(int)
}

// NewRating creates a new Rating widget with a default of 5 stars.
func NewRating() *Rating {
	p := new(Rating)
	p.Init(p)
	p.maxStars = 5
	p.hoverValue = -1
	return p
}

// Value returns the current rating (0 to maxStars).
func (this *Rating) Value() int { return this.value }

// SetValue sets the current rating.
func (this *Rating) SetValue(v int) {
	if v < 0 {
		v = 0
	}
	if v > this.maxStars {
		v = this.maxStars
	}
	if v != this.value {
		this.value = v
		if this.cbRatingChanged != nil {
			this.cbRatingChanged(v)
		}
		this.Self().Update()
	}
}

// MaxStars returns the maximum number of stars.
func (this *Rating) MaxStars() int { return this.maxStars }

// SetMaxStars sets the maximum number of stars.
func (this *Rating) SetMaxStars(n int) {
	if n < 1 {
		n = 1
	}
	if n > 10 {
		n = 10
	}
	this.maxStars = n
	if this.value > n {
		this.value = n
	}
	this.Self().Update()
}

// IsReadOnly returns whether the control is read-only.
func (this *Rating) IsReadOnly() bool { return this.readonly }

// SetReadOnly sets the read-only state.
func (this *Rating) SetReadOnly(b bool) {
	this.readonly = b
}

// SigRatingChanged sets the callback for when the rating changes.
func (this *Rating) SigRatingChanged(fn func(int)) {
	this.cbRatingChanged = fn
}

// --- Drawing ---

const (
	ratingRadius  = 8.0
	ratingSpacing = 4.0
)

func (this *Rating) Draw(g paint.Painter) {
	w, h := this.Size()
	cy := h * 0.5

	displayValue := this.value
	if this.hoverValue >= 0 && !this.readonly {
		displayValue = this.hoverValue
	}

	filledColor := paint.Color{255, 193, 7, 255}  // amber/gold
	emptyColor := paint.Color{200, 200, 200, 255} // light gray
	hoverColor := paint.Color{255, 215, 0, 180}   // gold with alpha

	// Dynamic radius based on widget bounds so stars fit
	dynRadius := ratingRadius
	if this.maxStars > 0 {
		maxR := w / (float64(this.maxStars)*2 + float64(this.maxStars-1)*(ratingSpacing/(ratingRadius*2))*2)
		if maxR < dynRadius {
			dynRadius = maxR
		}
		maxRH := (h - 4) / 2
		if maxRH < dynRadius {
			dynRadius = maxRH
		}
	}
	dynSpacing := ratingSpacing
	if dynRadius < ratingRadius {
		dynSpacing = dynSpacing * dynRadius / ratingRadius
	}

	for i := 0; i < this.maxStars; i++ {
		cx := dynRadius + float64(i)*(dynRadius*2+dynSpacing)

		if i < displayValue {
			if this.hoverValue >= 0 && !this.readonly {
				g.Arc(cx, cy, dynRadius, 0, 2*math.Pi)
				g.SetBrush1(hoverColor)
				g.Fill()
			} else {
				g.Arc(cx, cy, dynRadius, 0, 2*math.Pi)
				g.SetBrush1(filledColor)
				g.Fill()
			}
		} else {
			g.Arc(cx, cy, dynRadius, 0, 2*math.Pi)
			g.SetBrush1(emptyColor)
			g.FillPreserve()
			g.SetPen1(paint.Color{180, 180, 180, 255}, 1)
			g.Stroke()
		}
	}
}

// --- Events ---

func (this *Rating) OnMouseEnter() { this.Self().Update() }

func (this *Rating) OnMouseLeave() {
	this.hoverValue = -1
	this.Self().Update()
}

func (this *Rating) OnMouseMove(x, y float64) {
	if this.readonly {
		return
	}
	star := this.hitTestStar(x)
	if star != this.hoverValue {
		this.hoverValue = star
		this.Self().Update()
	}
}

func (this *Rating) OnLeftDown(x, y float64) {
	if this.readonly {
		return
	}
	this.SetFocus()
	star := this.hitTestStar(x)
	if star > 0 {
		this.SetValue(star)
	} else {
		this.SetValue(0)
	}
}

func (this *Rating) hitTestStar(x float64) int {
	for i := 0; i < this.maxStars; i++ {
		cx := ratingRadius + float64(i)*(ratingRadius*2+ratingSpacing)
		if x >= cx-ratingRadius && x <= cx+ratingRadius {
			return i + 1
		}
	}
	return 0
}

// --- SizeHints ---

func (this *Rating) SizeHints() SizeHints {
	w := float64(this.maxStars)*(ratingRadius*2+ratingSpacing) - ratingSpacing
	h := ratingRadius*2 + 4
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}

func (this *Rating) EnumProperties(list core.IPropertyList) {
	list.AddProperty("Value", this.Value, this.SetValue)
	list.AddProperty("MaxStars", this.MaxStars, this.SetMaxStars)
	list.AddProperty("ReadOnly", this.IsReadOnly, this.SetReadOnly)
}

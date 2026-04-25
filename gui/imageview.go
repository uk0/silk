package gui

import (
	"silk/core"
	"silk/paint"
	"math"
)

// ImageScaleMode 图片缩放模式
type ImageScaleMode int

const (
	ImageContain ImageScaleMode = iota // 保持比例，完整显示
	ImageCover                         // 保持比例，填满区域
	ImageStretch                       // 拉伸填满
	ImageCenter                        // 原始大小居中显示
)

// ImageView 图片显示控件
type ImageView struct {
	Widget
	pixmap    paint.Pixmap
	scaleMode ImageScaleMode
	bgColor   paint.Color
}

func init() {
	core.RegisterFactory("gui.ImageView", core.TypeOf((*ImageView)(nil)))
}

func NewImageView() *ImageView {
	p := new(ImageView)
	p.Init(p)
	p.scaleMode = ImageContain
	p.bgColor = paint.Color{240, 240, 240, 255}
	return p
}

func (this *ImageView) SetPixmap(pm paint.Pixmap) {
	this.pixmap = pm
	this.Self().Update()
}

func (this *ImageView) Pixmap() paint.Pixmap {
	return this.pixmap
}

func (this *ImageView) ScaleMode() ImageScaleMode {
	return this.scaleMode
}

func (this *ImageView) SetScaleMode(mode ImageScaleMode) {
	this.scaleMode = mode
	this.Self().Update()
}

func (this *ImageView) SetBgColor(c paint.Color) {
	this.bgColor = c
	this.Self().Update()
}

// --- Drawing ---

func (this *ImageView) Draw(g paint.Painter) {
	w, h := this.Size()

	// background
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(this.bgColor)
	g.Fill()

	if this.pixmap == nil {
		// placeholder icon
		t := Theme()
		g.SetBrush1(paint.Color{200, 200, 200, 255})
		g.SetFont(t.Font)
		text := "No Image"
		ext := t.Font.TextExtents(text)
		tx := (w - ext.Width) / 2
		ty := 0.5*(h+ext.YBearing) - ext.YBearing
		g.Translate(tx, ty)
		g.DrawText(text)
		g.Translate(-tx, -ty)
		return
	}

	imgWi := this.pixmap.Width()
	imgHi := this.pixmap.Height()
	if imgWi <= 0 || imgHi <= 0 {
		return
	}

	imgW := float64(imgWi)
	imgH := float64(imgHi)

	var sx, sy, sw, sh float64

	switch this.scaleMode {
	case ImageContain:
		scale := math.Min(w/imgW, h/imgH)
		sw = imgW * scale
		sh = imgH * scale
		sx = (w - sw) / 2
		sy = (h - sh) / 2

	case ImageCover:
		scale := math.Max(w/imgW, h/imgH)
		sw = imgW * scale
		sh = imgH * scale
		sx = (w - sw) / 2
		sy = (h - sh) / 2

	case ImageStretch:
		sx, sy = 0, 0
		sw, sh = w, h

	case ImageCenter:
		sx = (w - imgW) / 2
		sy = (h - imgH) / 2
		sw, sh = imgW, imgH
	}

	g.Save()
	g.Translate(sx, sy)
	scaleX := sw / imgW
	scaleY := sh / imgH
	g.Scale(scaleX, scaleY)
	g.DrawPixmap(this.pixmap)
	g.Restore()
}

func (this *ImageView) SizeHints() SizeHints {
	w, h := 120.0, 90.0
	if this.pixmap != nil {
		w = float64(this.pixmap.Width())
		h = float64(this.pixmap.Height())
		if w > 200 {
			scale := 200 / w
			w = 200
			h *= scale
		}
	}
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}

func (this *ImageView) EnumProperties(list core.IPropertyList) {
	list.AddProperty("缩放模式", this.ScaleMode, this.SetScaleMode)
}

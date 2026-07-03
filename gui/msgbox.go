package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// MessageBox is a styled modal message dialog.
type MessageBox struct {
	Form
	btnBox *ButtonBox
	edit   *Edit
	ret    string
}

func (this *MessageBox) Init(iw IWidget) {
	this.Form.Init(iw)

	this.btnBox = NewButtonBox()
	this.btnBox.SetParent(iw)
	this.btnBox.SigSubmit(this.onBtnClick)

	this.edit = NewEdit()
	this.edit.SetParent(iw)
	this.edit.SetMultiLine(true)
	this.edit.SetReadOnly(true)
	this.edit.SetNoFrame(true)
}

// NewMessageBox creates a new MessageBox instance.
func NewMessageBox() *MessageBox {
	p := new(MessageBox)
	p.Init(p)
	return p
}

func (this *MessageBox) SetMessage(s string) {
	this.edit.SetText(s)
	this.Layout()
}

func (this *MessageBox) ShowModal() string {
	this.SetVisible(false)
	this.AttachWindow(WtForm)
	this.SetBounds(this.x, this.y, 380, 180)
	if this.Icon() != nil {
		this.Window().SetIcon(this.Icon())
	}
	this.Window().MoveToCenter()
	a := this.Window().ShowModal(nil)
	if s, ok := a.(string); ok {
		this.ret = s
		return s
	}

	this.ret = core.VisualString(a)
	return this.ret
}

func (this *MessageBox) onBtnClick(s string) {
	this.Window().EndModal(s)
}

func (this *MessageBox) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Width(), this.Height()
	btnBarH := float64(t.ItemHeight) + 16.0

	// Content area background
	g.Rectangle(0, 0, w, h-btnBarH)
	g.SetBrush1(t.ViewBGColor)
	g.Fill()

	// Button bar background (slightly different tone)
	g.Rectangle(0, h-btnBarH, w, btnBarH)
	g.SetBrush1(t.FormColor)
	g.Fill()

	// Separator line above button bar
	g.Line(0, h-btnBarH, w, h-btnBarH)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()

	// Draw icon if present (left side, vertically centered in content area)
	ico := this.Icon()
	if ico != nil {
		icoSize := 32.0
		ix := 16.0
		iy := (h - btnBarH - icoSize) * 0.5
		g.Translate(ix, iy)
		g.DrawIcon(ico, icoSize, false)
		g.Translate(-ix, -iy)
	}
}

func (this *MessageBox) Layout() {
	t := Theme()
	btnBarH := float64(t.ItemHeight) + 16.0
	padSide := 16.0
	padTop := 12.0
	iconSpace := 0.0

	if this.Icon() != nil {
		iconSpace = 48.0
	}

	// Center the button bar: add left padding to center buttons
	btnW := this.w
	this.btnBox.SetBounds(0, this.h-btnBarH, btnW, btnBarH)

	// Edit area: right of icon, above button bar
	editX := padSide + iconSpace
	editY := padTop
	editW := this.w - editX - padSide
	editH := this.h - btnBarH - padTop - 8
	this.edit.SetBounds(editX, editY, editW, editH)
}

func (this *MessageBox) SetButtons(btns []string) {
	this.btnBox.SetButtons(btns)
}

// ShowMessageBox displays a styled message box dialog.
func ShowMessageBox(iw IWidget, ico paint.Icon, title, content string, btns []string) string {
	box := NewMessageBox()
	box.SetTitle(title)
	box.SetMessage(content)
	box.SetButtons(btns)
	box.SetIcon(ico)
	box.SetParent(iw)
	return box.ShowModal()
}

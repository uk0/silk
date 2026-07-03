package gui

import (
	"github.com/uk0/silk/paint"
)

// InputBox is a styled dialog for text input with a prompt label.
type InputBox struct {
	Form
	btnBox *ButtonBox
	edit   *Edit
	prompt *Label
	title  string
	ico    paint.Icon
}

func (this *InputBox) Init(iw IWidget) {
	this.Form.Init(iw)

	this.prompt = NewLabel("")
	this.prompt.SetParent(iw)

	this.btnBox = NewButtonBox()
	this.btnBox.SetParent(iw)
	this.btnBox.SigSubmit(this.onBtnClick)
	this.btnBox.SetButtons([]string{"@ok", "@cancel"})

	this.edit = NewEdit()
	this.edit.SetParent(iw)
	this.edit.SetMultiLine(false)
	this.edit.SetNoFrame(false)
}

func NewInputBox() *InputBox {
	p := new(InputBox)
	p.Init(p)
	return p
}

func (this *InputBox) SetIcon(ico paint.Icon) {
	this.ico = ico
	this.Layout()
}

func (this *InputBox) SetTitle(s string) {
	this.title = s
	this.Layout()
}

func (this *InputBox) Title() string {
	return this.title
}

func (this *InputBox) SetLabel(s string) {
	this.prompt.SetText(s)
	this.Layout()
}

func (this *InputBox) SetMessage(s string) {
	this.edit.SetText(s)
	this.Layout()
}

func (this *InputBox) ShowModal() bool {
	this.SetVisible(false)
	this.AttachWindow(WtForm)
	this.SetBounds(this.x, this.y, 380, 170)
	if this.ico != nil {
		this.Window().SetIcon(this.ico)
	}
	this.Window().MoveToCenter()
	a := this.Window().ShowModal(nil)
	b, _ := a.(bool)
	return b
}

func (this *InputBox) onBtnClick(s string) {
	if s == "@ok" {
		this.Window().EndModal(true)
	} else {
		this.Window().EndModal(false)
	}
}

func (this *InputBox) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Width(), this.Height()
	btnBarH := float64(t.ItemHeight) + 16.0

	// Content area background
	g.Rectangle(0, 0, w, h-btnBarH)
	g.SetBrush1(t.ViewBGColor)
	g.Fill()

	// Button bar background
	g.Rectangle(0, h-btnBarH, w, btnBarH)
	g.SetBrush1(t.FormColor)
	g.Fill()

	// Separator line above button bar
	g.Line(0, h-btnBarH, w, h-btnBarH)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()
}

func (this *InputBox) Layout() {
	t := Theme()
	btnBarH := float64(t.ItemHeight) + 16.0
	padSide := 20.0
	padTop := 16.0

	// Button bar at bottom
	this.btnBox.SetBounds(0, this.h-btnBarH, this.w, btnBarH)

	// Prompt label above the edit field
	labelH := 22.0
	if this.prompt.Text() != "" {
		this.prompt.SetBounds(padSide, padTop, this.w-padSide*2, labelH)
	} else {
		this.prompt.SetBounds(0, 0, 0, 0)
		labelH = 0
	}

	// Edit field below the label, centered vertically in remaining space
	editH := float64(t.ItemHeight) + 4
	editY := padTop + labelH + 8
	this.edit.SetBounds(padSide, editY, this.w-padSide*2, editH)
}

// ShowInputBox displays a styled input dialog.
func ShowInputBox(parent IWidget, icon paint.Icon, title, label, defText string) (string, bool) {
	p := NewInputBox()
	p.SetParent(parent)
	p.SetIcon(icon)
	p.SetTitle(title)
	p.SetLabel(label)
	p.edit.SetText(defText)
	p.edit.SelectAll()
	if p.ShowModal() {
		return p.edit.Text(), true
	}
	return "", false
}

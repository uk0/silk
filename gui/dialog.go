package gui

import (
	"math"
	"silk/core"
	"silk/paint"
)

func init() {
	core.RegisterFactory("gui.Dialog", core.TypeOf((*Dialog)(nil)))
}

// DialogResult represents the result of a dialog action
type DialogResult int

const (
	DialogOK     DialogResult = iota
	DialogCancel
	DialogYes
	DialogNo
)

// Dialog is a modal window widget built on top of Form.
// It provides a content area and a bottom button bar for user interaction.
type Dialog struct {
	Form
	contentWidget IWidget
	btnBox        *ButtonBox
	result        DialogResult
	buttons       []*Button
	resultMap     map[string]DialogResult
	closed        bool
}

// NewDialog creates a new Dialog with the given title and parent widget.
func NewDialog(title string, parent IWidget) *Dialog {
	p := new(Dialog)
	p.Init(p)
	p.SetTitle(title)
	p.resultMap = make(map[string]DialogResult)
	if parent != nil {
		p.SetParent(parent)
	}
	p.btnBox = NewButtonBox()
	p.btnBox.SetParent(p)
	p.btnBox.SigSubmit(p.onBtnClick)
	return p
}

// SetContent sets the main content widget displayed in the dialog body.
func (this *Dialog) SetContent(w IWidget) {
	if this.contentWidget != nil {
		this.contentWidget.SetParent(nil)
	}
	this.contentWidget = w
	if w != nil {
		w.SetParent(this)
	}
	this.Layout()
}

// Content returns the current content widget.
func (this *Dialog) Content() IWidget {
	return this.contentWidget
}

// AddButton adds a button with the given text and associated DialogResult.
// Returns the created Button for further customization.
func (this *Dialog) AddButton(text string, result DialogResult) *Button {
	key := dialogResultKey(result)
	this.resultMap[key] = result
	btn := this.btnBox.AddButton1(text, nil)
	btn.SetExtraData(key)
	btn.Action().BindFunc(func(a IAction, sender interface{}) {
		this.onBtnClick(key)
	})
	this.buttons = append(this.buttons, btn)
	this.Layout()
	return btn
}

// ShowModal displays the dialog modally and returns the result.
func (this *Dialog) ShowModal() DialogResult {
	this.closed = false
	this.result = DialogCancel

	this.SetVisible(false)
	this.AttachWindow(WtForm)

	w, h := this.dialogSize()
	this.SetBounds(this.x, this.y, w, h)

	win := this.Window()
	if win == nil {
		return this.result
	}
	if this.Icon() != nil {
		win.SetIcon(this.Icon())
	}
	win.MoveToCenter()

	a := win.ShowModal(nil)
	if r, ok := a.(DialogResult); ok {
		this.result = r
	}
	return this.result
}

// Result returns the dialog result after ShowModal has returned.
func (this *Dialog) Result() DialogResult {
	return this.result
}

func (this *Dialog) dialogSize() (w, h float64) {
	w, h = 380, 160
	btnBarH := 44.0
	padSide := 24.0
	padTopBot := 20.0

	if this.contentWidget != nil {
		hints := this.contentWidget.SizeHints()
		cw := hints.Width + padSide*2
		ch := hints.Height + padTopBot*2 + btnBarH
		w = math.Max(w, cw)
		h = math.Max(h, ch)
	}
	// Clamp to reasonable max
	if w > 600 {
		w = 600
	}
	if h > 500 {
		h = 500
	}
	return
}

func (this *Dialog) onBtnClick(key string) {
	result, ok := this.resultMap[key]
	if !ok {
		result = DialogCancel
	}
	this.result = result
	this.closed = true
	if this.Window() != nil {
		this.Window().EndModal(result)
	}
}

func (this *Dialog) EnumProperties(list core.IPropertyList) {
	list.AddProperty("标题", this.Title, this.SetTitle)
}

// Layout arranges the content area on top and the button bar at the bottom.
func (this *Dialog) Layout() {
	btnBarH := 44.0
	padSide := 24.0
	padTop := 20.0

	if this.btnBox != nil {
		this.btnBox.SetBounds(0, this.h-btnBarH, this.w, btnBarH)
	}
	if this.contentWidget != nil {
		this.contentWidget.SetBounds(padSide, padTop, this.w-padSide*2, this.h-padTop-btnBarH)
	}
}

// Draw renders a modern dialog background with clean separation.
func (this *Dialog) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Width(), this.Height()
	btnBarH := 44.0

	// Content background (theme-aware for dark mode)
	g.Rectangle(0, 0, w, h)
	g.SetBrush1(t.ViewBGColor)
	g.Fill()

	// Button bar background (slightly darker than content)
	g.Rectangle(0, h-btnBarH, w, btnBarH)
	g.SetBrush1(t.FormColor)
	g.Fill()

	// Separator line above button bar
	g.Line(0, h-btnBarH, w, h-btnBarH)
	g.SetPen1(t.BorderColor, 1)
	g.Stroke()
}

// SizeHints returns the preferred size for the dialog.
func (this *Dialog) SizeHints() SizeHints {
	w, h := this.dialogSize()
	return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
}

func dialogResultKey(r DialogResult) string {
	switch r {
	case DialogOK:
		return "@ok"
	case DialogCancel:
		return "@cancel"
	case DialogYes:
		return "@yes"
	case DialogNo:
		return "@no"
	default:
		return "@cancel"
	}
}

// ShowMessageDialog displays a simple message dialog with an OK button.
func ShowMessageDialog(parent IWidget, title, message string) DialogResult {
	dlg := NewDialog(title, parent)

	content := NewVBox()
	content.SetSpacing(12)

	msgLabel := NewLabel(message)
	msgLabel.SetWrap(true)
	content.AddWidget(msgLabel)

	dlg.SetContent(content)
	dlg.AddButton("OK", DialogOK)
	return dlg.ShowModal()
}

// ShowConfirmDialog displays a confirmation dialog with Yes/No buttons.
// Returns true if the user clicked Yes.
func ShowConfirmDialog(parent IWidget, title, message string) bool {
	dlg := NewDialog(title, parent)

	content := NewVBox()
	content.SetSpacing(12)

	msgLabel := NewLabel(message)
	msgLabel.SetWrap(true)
	content.AddWidget(msgLabel)

	dlg.SetContent(content)
	dlg.AddButton("Yes", DialogYes)
	dlg.AddButton("No", DialogNo)
	result := dlg.ShowModal()
	return result == DialogYes
}

// ShowInputDialog displays an input dialog with a text field.
// Returns the entered text and true if the user clicked OK, or empty string
// and false if cancelled.
func ShowInputDialog(parent IWidget, title, prompt, defaultVal string) (string, bool) {
	box := NewInputBox()
	box.SetParent(parent)
	box.SetTitle(title)
	box.edit.SetText(defaultVal)
	box.edit.SelectAll()
	if box.ShowModal() {
		return box.edit.Text(), true
	}
	return "", false
}

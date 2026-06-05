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
	defaultBtn    *Button      // 默认按钮(回车触发), nil 时按隐式规则推断
	cancelResult  DialogResult // Esc 关闭对话框返回的结果, 默认 DialogCancel
}

// NewDialog creates a new Dialog with the given title and parent widget.
func NewDialog(title string, parent IWidget) *Dialog {
	p := new(Dialog)
	p.Init(p)
	p.SetTitle(title)
	p.resultMap = make(map[string]DialogResult)
	p.cancelResult = DialogCancel
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

// SetDefaultButton marks btn (one returned from AddButton) as the dialog's
// default button — the one Enter/Return activates. Passing nil clears it and
// reverts to the implicit heuristic (see defaultButton). Matches Qt
// QPushButton::setDefault on a QDialog's button.
func (this *Dialog) SetDefaultButton(btn *Button) {
	this.defaultBtn = btn
}

// SetCancelResult overrides the result returned when Esc closes the dialog
// (DialogCancel by default), mirroring Qt's reject role.
func (this *Dialog) SetCancelResult(r DialogResult) {
	this.cancelResult = r
}

// defaultButton returns the button Enter should activate. An explicitly set
// default wins. Otherwise we follow Qt QDialogButtonBox's "first AcceptRole"
// heuristic: the first affirmative button (OK, then Yes); failing that, the
// last-added button (Qt makes the sole/last button default when no accept
// role exists). Hidden or disabled buttons are skipped. Returns nil only when
// there are no usable buttons.
func (this *Dialog) defaultButton() *Button {
	if this.defaultBtn != nil {
		return this.defaultBtn
	}
	for _, want := range []DialogResult{DialogOK, DialogYes} {
		for _, b := range this.buttons {
			if b.IsVisible() && b.IsEnabled() && this.resultMap[btnKey(b)] == want {
				return b
			}
		}
	}
	for i := len(this.buttons) - 1; i >= 0; i-- {
		if b := this.buttons[i]; b.IsVisible() && b.IsEnabled() {
			return b
		}
	}
	return nil
}

// resolveDefault ends the modal with the default button's result, taking the
// exact same path a click on that button takes (onBtnClick). Reports whether a
// default button was found. Exposed for headless testing of the Enter path.
func (this *Dialog) resolveDefault() bool {
	b := this.defaultButton()
	if b == nil {
		return false
	}
	this.onBtnClick(btnKey(b))
	return true
}

// resolveCancel ends the modal with the cancel result, as if a cancel button
// were clicked. Exposed for headless testing of the Esc path.
func (this *Dialog) resolveCancel() {
	this.onBtnClick(dialogResultKey(this.cancelResult))
}

// OnKeyDown implements IEventKeyDown so the dialog itself handles Enter/Esc
// (Qt QDialog behavior). Enter/Return activates the default button; Esc
// cancels/rejects. Key events route here only when no child widget holds focus
// (window_glfw dispatches to focusWidget first), so a focused multi-line editor
// keeps its own Enter — the dialog only catches these as the focus fallback.
func (this *Dialog) OnKeyDown(key int, repeat bool) {
	switch key {
	case KeyEnter:
		this.resolveDefault()
	case KeyEsc:
		this.resolveCancel()
	}
}

// btnKey returns the result key a Dialog button was tagged with in AddButton.
func btnKey(b *Button) string {
	if k, ok := b.ExtraData().(string); ok {
		return k
	}
	return "@cancel"
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

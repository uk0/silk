package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"math"
)

func init() {
	core.RegisterFactory("gui.Dialog", core.TypeOf((*Dialog)(nil)))
}

// DialogResult represents the result of a dialog action
type DialogResult int

const (
	DialogOK DialogResult = iota
	DialogCancel
	DialogYes
	DialogNo
)

// Dialog is a modal window widget built on top of Form.
// It provides a content area and a bottom button bar for user interaction.
type Dialog struct {
	Form
	contentWidget IWidget
	btnBox        *dialogButtonBar
	result        DialogResult
	buttons       []*Button
	resultMap     map[string]DialogResult
	closed        bool
	defaultBtn    *Button      // 默认按钮(回车触发), nil 时按隐式规则推断
	cancelResult  DialogResult // Esc 关闭对话框返回的结果, 默认 DialogCancel
	preferredW    float64
	preferredH    float64
	initialFocus  IWidget
	lastError     error
}

// NewDialog creates a new Dialog with the given title and parent widget.
func NewDialog(title string, parent IWidget) *Dialog {
	p := new(Dialog)
	p.Init(p)
	p.SetTitle(title)
	if parent != nil {
		p.SetParent(parent)
	}
	return p
}

// Init carries the state every dialog needs regardless of how it was built. A
// reflect-built dialog (designer palette, form loader) only ever gets Init, and
// without this AddButton dies on a nil resultMap and a nil button bar, while
// cancelResult left at 0 makes Esc return DialogOK instead of rejecting.
func (this *Dialog) Init(iw IWidget) {
	this.Form.Init(iw)

	this.resultMap = make(map[string]DialogResult)
	this.cancelResult = DialogCancel

	this.btnBox = new(dialogButtonBar)
	this.btnBox.Init(this.btnBox)
	this.btnBox.SetFocusPolicy(NoFocus)
	this.btnBox.SetParent(iw)
	this.btnBox.SigSubmit(this.onBtnClick)
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
	this.markDefaultButton()
	return btn
}

// markDefaultButton moves the isDefault flag onto the button defaultButton()
// currently resolves to, clearing it everywhere else. It runs after every
// change to the button set or the explicit default because the implicit
// heuristic can pick a different button each time (the last-added button is
// default until an OK/Yes shows up), and two ringed buttons would be worse
// than none.
func (this *Dialog) markDefaultButton() {
	def := this.defaultButton()
	for _, b := range this.buttons {
		b.SetDefault(b == def)
	}
}

// ShowModal displays the dialog modally and returns the result.
func (this *Dialog) ShowModal() DialogResult {
	this.closed = false
	this.result = DialogCancel
	this.lastError = nil

	this.SetVisible(false)
	this.AttachWindow(WtForm)

	w, h := this.dialogSize()
	// Applying the computed size is not a new caller preference.
	this.Form.SetBounds(this.x, this.y, w, h)

	win := this.Window()
	if win == nil {
		return this.result
	}
	if this.Icon() != nil {
		win.SetIcon(this.Icon())
	}
	win.MoveToCenter()

	a := win.ShowModal(this.activateInitialFocus)
	this.lastError = win.ModalError()
	if r, ok := a.(DialogResult); ok {
		this.result = r
	}
	return this.result
}

// LastError returns a recovered modal failure, or nil for normal acceptance/cancellation.
// It is reset before each ShowModal call. Read it on the UI thread after return.
func (this *Dialog) LastError() error { return this.lastError }

// SetInitialFocus selects the child that receives keyboard input after native
// activation. Unlike calling child.SetFocus before ShowModal, this preserves
// the owner's current focus so cancellation can return to the original editor.
// The target must be a visible/enabled descendant when the dialog is shown.
func (this *Dialog) SetInitialFocus(target IWidget) { this.initialFocus = target }

func (this *Dialog) activateInitialFocus() {
	for p := this.initialFocus; p != nil; p = p.Parent() {
		if !p.IsVisible() || !p.IsEnabled() {
			return
		}
		if p == this.Self() {
			this.initialFocus.SetFocus()
			return
		}
	}
}

// Result returns the dialog result after ShowModal has returned.
func (this *Dialog) Result() DialogResult {
	return this.result
}

// SetSize records the requested modal size. ShowModal must not silently replace
// a caller's explicit size with the content's (often much smaller) natural size.
func (this *Dialog) SetSize(w, h float64) {
	this.preferredW, this.preferredH = w, h
	this.Form.SetSize(w, h)
}

func (this *Dialog) SetBounds(x, y, w, h float64) {
	this.preferredW, this.preferredH = w, h
	this.Form.SetBounds(x, y, w, h)
}

func (this *Dialog) dialogSize() (w, h float64) {
	w, h = 380, 160
	btnBarH := dialogButtonBarHeight
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
	if this.preferredW > 0 {
		w = math.Max(380, this.preferredW)
	}
	if this.preferredH > 0 {
		h = math.Max(160, this.preferredH)
	}
	if this.btnBox != nil {
		w = math.Max(w, this.btnBox.requiredWidth())
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
	this.markDefaultButton()
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

// resolveDefault invokes the default button action, including custom validation
// that may deliberately leave the dialog open. Reports whether a
// default button was found. Exposed for headless testing of the Enter path.
func (this *Dialog) resolveDefault() bool {
	b := this.defaultButton()
	if b == nil {
		return false
	}
	b.Action().Trigger(b)
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
	if repeat {
		return
	}
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
	btnBarH := dialogButtonBarHeight
	padSide := 24.0
	padTop := 20.0

	if this.btnBox != nil {
		this.btnBox.SetBounds(0, math.Max(0, this.h-btnBarH), math.Max(0, this.w), math.Min(this.h, btnBarH))
	}
	if this.contentWidget != nil {
		this.contentWidget.SetBounds(padSide, padTop, math.Max(0, this.w-padSide*2), math.Max(0, this.h-padTop-btnBarH-16))
	}
}

// Draw renders a modern dialog background with clean separation.
func (this *Dialog) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Width(), this.Height()
	btnBarH := dialogButtonBarHeight

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

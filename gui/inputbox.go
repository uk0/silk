package gui

import (
	"github.com/uk0/silk/paint"
)

// InputBox is a styled dialog for text input with a prompt label.
type InputBox struct {
	Form
	btnBox     *ButtonBox
	edit       *Edit
	prompt     *Label
	validation *Label
	title      string
}

func (this *InputBox) Init(iw IWidget) {
	this.Form.Init(iw)

	this.prompt = NewLabel("")
	this.prompt.SetParent(iw)
	this.prompt.SetWrap(true)
	this.validation = NewLabel("")
	this.validation.SetParent(iw)
	this.validation.SetWrap(true)
	this.validation.SetFocusPolicy(NoFocus)

	this.btnBox = NewButtonBox()
	this.btnBox.SetParent(iw)
	this.btnBox.SigSubmit(this.onBtnClick)
	this.btnBox.SetButtons([]string{"@ok", "@cancel"})

	this.edit = NewEdit()
	this.edit.SetParent(iw)
	this.edit.SetMultiLine(false)
	this.edit.SetNoFrame(false)
	this.edit.SigSubmit(func(_ interface{}, _ string) { this.onBtnClick("@ok") })
	this.edit.SigTextChanged(func(_ interface{}, _ string) { this.syncValidation() })
}

func NewInputBox() *InputBox {
	p := new(InputBox)
	p.Init(p)
	return p
}

func (this *InputBox) SetIcon(ico paint.Icon) {
	this.Form.SetIcon(ico)
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
	win := this.Window()
	if win == nil {
		return false
	}
	win.MoveToCenter()
	a := win.ShowModal(this.focusInput)
	b, _ := a.(bool)
	return b
}

// Native activation must finish before assigning logical input focus.
func (this *InputBox) focusInput() {
	this.edit.SetFocus()
	this.edit.SelectAll()
}
func (this *InputBox) onBtnClick(s string) {
	if s == "@ok" {
		this.syncValidation()
		if !this.edit.IsValid() {
			this.edit.SetFocus()
			return
		}
	}
	if win := this.Window(); win != nil {
		win.EndModal(s == "@ok")
	}
}

// Escape cancels this dialog regardless of which child owns the focus.
func (this *InputBox) resolveCancel() { this.onBtnClick("@cancel") }

func (this *InputBox) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Width(), this.Height()
	btnBarH := this.btnBox.Height()

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

// Text returns the draft, including an invalid/intermediate value. UI-thread only.
func (this *InputBox) Text() string { return this.edit.Text() }

// SetValidator gates both mouse and keyboard submission. Nil retains the
// historical free-form behavior. Validators must be fast, side-effect-free and
// UI-thread-only; use ValidatorFunc for drafts that may be temporarily invalid.
func (this *InputBox) SetValidator(v Validator) {
	this.edit.SetValidator(v)
	this.syncValidation()
}
func (this *InputBox) HasAcceptableInput() bool { return this.edit.HasAcceptableInput() }
func (this *InputBox) ValidationError() string  { return this.edit.ValidationError() }

func (this *InputBox) syncValidation() {
	// Edit callbacks can run before its final revalidation step.
	this.edit.revalidate()
	valid := this.edit.IsValid()
	this.validation.SetText(this.edit.ValidationError())
	ok := this.btnBox.btnMap["@ok"]
	ok.SetEnabled(valid)
	ok.Action().SetEnabled(valid)
	this.Layout()
}

func (this *InputBox) Layout() {
	w, h := max(0, this.w), max(0, this.h)
	footer := min(h, max(Theme().ItemHeight+16, this.btnBox.SizeHints().Height+12))
	topLimit := h - footer
	this.btnBox.SetBounds(0, topLimit, w, footer)
	pad := min(20, w/2)
	width := max(0, w-2*pad)
	y := min(16, topLimit)
	place := func(v IWidget, height float64) {
		height = min(max(0, height), max(0, topLimit-y))
		v.SetBounds(pad, y, width, height)
		y += height
	}
	if this.prompt.Text() != "" {
		this.prompt.SetBounds(pad, y, width, 0)
		place(this.prompt, this.prompt.SizeHints().Height)
		y = min(y+8, topLimit)
	} else {
		this.prompt.SetBounds(pad, y, width, 0)
	}
	place(this.edit, max(Theme().ItemHeight+4, this.edit.SizeHints().Height))
	y = min(y+6, topLimit)
	// Reserve a stable, wrapping status area: validity changes do not move input
	// or buttons. Tiny hosts clip within the content area, never over the footer.
	this.validation.SetBounds(pad, y, width, max(0, topLimit-y-6))
	if CurrentThemeMode() == ThemeDark {
		this.validation.SetTextColor(paint.Color{R: 252, G: 165, B: 165, A: 255})
	} else {
		this.validation.SetTextColor(paint.Color{R: 185, G: 28, B: 28, A: 255})
	}
}

// ShowInputBox displays a styled input dialog.
func ShowInputBox(parent IWidget, icon paint.Icon, title, label, defText string) (string, bool) {
	return ShowValidatedInputBox(parent, icon, title, label, defText, nil)
}

// ShowValidatedInputBox keeps invalid drafts open with a reason and disabled OK.
// Escape/Cancel remain available. Nil validator is identical to ShowInputBox.
func ShowValidatedInputBox(parent IWidget, icon paint.Icon, title, label, defText string, validator Validator) (string, bool) {
	p := NewInputBox()
	p.SetParent(parent)
	p.SetIcon(icon)
	p.SetTitle(title)
	p.SetLabel(label)
	p.SetMessage(defText)
	p.SetValidator(validator)
	p.edit.SelectAll()
	if p.ShowModal() {
		return p.Text(), true
	}
	return "", false
}

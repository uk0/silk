package gui

import (
	"math"

	"github.com/uk0/silk/paint"
)

// MessageBox is a styled modal message dialog.
type MessageBox struct {
	Form
	btnBox      *dialogButtonBar
	edit        *Edit
	ret         string
	buttonNames []string
	defaultName string
	lastError   error
}

func (this *MessageBox) Init(iw IWidget) {
	this.Form.Init(iw)
	this.SetFocusPolicy(NoFocus)

	this.btnBox = new(dialogButtonBar)
	this.btnBox.Init(this.btnBox)
	this.btnBox.SetFocusPolicy(NoFocus)
	this.btnBox.SetParent(iw)
	this.btnBox.SigSubmit(this.onBtnClick)

	this.edit = NewEdit()
	this.edit.SetParent(iw)
	this.edit.SetMultiLine(true)
	this.edit.SetWrap(true)
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
	this.edit.SetCaretPos(0)
	this.Layout()
}

func (this *MessageBox) ShowModal() string {
	this.ret, this.lastError = "", nil
	this.SetVisible(false)
	this.AttachWindow(WtForm)
	this.SetBounds(this.x, this.y, math.Max(620, this.btnBox.requiredWidth()), 280)
	win := this.Window()
	if win == nil {
		return ""
	}
	win.MoveToCenter()
	result := win.ShowModal(this.focusMessage)
	this.lastError = win.ModalError()
	this.ret, _ = result.(string)
	return this.ret
}

// LastError preserves a recovered modal failure without treating it as a choice.
func (this *MessageBox) LastError() error { return this.lastError }

// SetDefaultButton selects an explicit button name. Unknown/empty names use the
// safe implicit choice: Cancel/No, then OK/Save/Yes, then a sole custom button.
// A destructive multi-button custom dialog has no implicit affirmative action.
func (this *MessageBox) SetDefaultButton(name string) {
	this.defaultName = name
	this.markDefaultButton()
}
func (this *MessageBox) defaultButton() *Button {
	usable := func(name string) *Button {
		b := this.btnBox.btnMap[name]
		if b != nil && b.IsVisible() && b.IsEnabled() {
			return b
		}
		return nil
	}
	if b := usable(this.defaultName); b != nil {
		return b
	}
	for _, name := range []string{"@cancel", "取消", "Cancel", "@no", "否", "No", "@ok", "确定", "OK", "@save", "@save-all", "@yes", "Yes"} {
		if b := usable(name); b != nil {
			return b
		}
	}
	if len(this.buttonNames) == 1 {
		return usable(this.buttonNames[0])
	}
	return nil
}
func (this *MessageBox) markDefaultButton() {
	def := this.defaultButton()
	for _, button := range this.btnBox.btnMap {
		button.SetDefault(button == def)
	}
}
func (this *MessageBox) focusMessage() { this.edit.SetFocus() }
func (this *MessageBox) resolveDefault() bool {
	// Tab-selected buttons retain their own action; selecting/copying read-only
	// error text must not make Enter silently stop acknowledging the message.
	if b, ok := focusWidget.(*Button); ok && b.Parent() == this.btnBox && b.IsVisible() && b.IsEnabled() {
		b.Action().Trigger(b)
		return true
	}
	if b := this.defaultButton(); b != nil {
		b.Action().Trigger(b)
		return true
	}
	return false
}
func (this *MessageBox) resolveCancel() {
	for _, name := range []string{"@cancel", "取消", "Cancel"} {
		if b := this.btnBox.btnMap[name]; b != nil && b.IsVisible() && b.IsEnabled() {
			this.onBtnClick(name)
			return
		}
	}
	this.onBtnClick("")
}
func (this *MessageBox) OnKeyDown(key int, repeat bool) {
	if repeat {
		return
	}
	if key == KeyEnter {
		this.resolveDefault()
	}
	if key == KeyEsc {
		this.resolveCancel()
	}
}
func (this *MessageBox) onBtnClick(s string) {
	this.ret = s
	if win := this.Window(); win != nil {
		win.EndModal(s)
	}
}

func (this *MessageBox) Draw(g paint.Painter) {
	t := Theme()
	w, h := this.Width(), this.Height()
	btnBarH := dialogButtonBarHeight

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
	btnBarH := dialogButtonBarHeight
	padSide := 16.0
	padTop := 12.0
	iconSpace := 0.0

	if this.Icon() != nil {
		iconSpace = 48.0
	}

	// Match Dialog: right-aligned, consistently sized buttons with an inset.
	btnW := this.w
	this.btnBox.SetBounds(0, math.Max(0, this.h-btnBarH), math.Max(0, btnW), math.Min(this.h, btnBarH))

	// Edit area: right of icon, above button bar
	editX := padSide + iconSpace
	editY := padTop
	editW := this.w - editX - padSide
	editH := this.h - btnBarH - padTop - 8
	this.edit.SetBounds(editX, editY, math.Max(0, editW), math.Max(0, editH))
}

func (this *MessageBox) SetButtons(btns []string) {
	this.btnBox.SetButtons(btns)
	this.buttonNames = append([]string(nil), btns...)
	this.markDefaultButton()
	this.Layout()
}

// ShowMessageBox displays a styled message box dialog.
func ShowMessageBox(iw IWidget, ico paint.Icon, title, content string, btns []string) string {
	box := NewMessageBox()
	box.SetTitle(title)
	box.SetMessage(content)
	box.SetButtons(btns)
	box.SetIcon(ico)
	// Center on the owning form, not the narrow file-tree widget that raised it.
	if iw != nil {
		if owner := iw.OwnerWindow(); owner != nil && owner.widget != nil {
			iw = owner.widget
		}
	}
	box.SetParent(iw)
	return box.ShowModal()
}

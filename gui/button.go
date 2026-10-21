package gui

import (
	"math"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gv"
	"github.com/uk0/silk/paint"
)

func (this *Button) EnumProperties(list core.IPropertyList) {
	list.AddProperty("文本", this.Text, this.SetText)
	list.AddProperty("图标", this.IconName, this.SetIconName)
	list.AddProperty("可见", this.IsVisible, this.SetVisible)
	list.AddProperty("可用", this.IsEnabled, this.SetEnabled)
}

type IButton interface {
	asButton()
	HideSubPopup()
	ShowSubPopup()
	IsInPopupMenu() bool

	SetTextVisible(bool)
	SubPopup() IWidget
}

// 按钮, 含普通按钮, 菜单项, 下拉按钮等
type Button struct {
	face                *paint.LinearGradient
	faceHeight          float64
	faceTop, faceBottom paint.Color
	Widget
	pushed      bool
	textVisible bool
	isDefault   bool
	//iconVisible bool
	action       IAction
	iconName     string // resource name the icon came from, "" for a directly set icon
	subPopup     IWidget
	syncRev      uint64
	cbSubPopup   func(IButton)
	shortcutText string // informational popup column; shortcut dispatch is registered separately

	// SizeHints cache. Validity is determined by comparing the captured key
	// fields below against the current widget/theme state on each call. The
	// cache eliminates ~4 cairo allocations per Button.SizeHints() (the two
	// font extents calls plus their backing scaledFont machinery), which
	// dominates layout cost when many buttons are children of an HBox/VBox.
	//
	// Cache key inputs:
	//   hintActionRev — covers SetText/SetIcon/SetEnabled/SetChecked
	//   hintTextVis     — covers SetTextVisible (does not flow through Action)
	//   hintParent      — covers IsInPopupMenu transitions on reparent
	//   hintThemeRev    — covers SetThemeMode font/margin changes
	cachedHints   SizeHints
	hintActionRev uint64
	hintParent    IWidget
	hintThemeRev  uint64
	hintTextVis   bool
	hintsValid    bool
}

func init() {
	core.RegisterFactory("gui.Button", core.TypeOf((*Button)(nil)))
}

func NewButton() *Button {
	return NewButton1("", nil)
}

func NewButton1(s string, icon paint.Icon) *Button {
	btn := new(Button)
	btn.Init(btn)
	btn.SetText(s)
	btn.SetIcon(icon)
	return btn
}

func NewActionButton(a IAction) *Button {
	btn := new(Button)
	btn.Init(btn)
	btn.SetAction(a)
	return btn
}

func (this *Button) Draw(g paint.Painter) {
	Theme().DrawButton(g, this)
}

func (this *Button) OnMouseEnter() {
	//core.Debug("(this *Button) OnMouseEnter()")
	this.Self().Update()
}

func (this *Button) OnMouseLeave() {
	//core.Debug("(this *Button) OnMouseLeave()")
	this.Self().Update()
}

func (this *Button) OnLeftDown(x, y float64) {
	//core.Debug("(this *Button) OnLeftDown()")
	if this.IsEnabled() && this.IsAllAncentorsVisible() {
		this.pushed = true
		this.SetFocus()
		this.Self().Update()
	}
}

func (this *Button) OnLeftUp(x, y float64) {
	//core.Debug("(this *Button) OnLeftUp()")
	pushed := this.pushed
	this.pushed = false
	if core.IsDebugOn() {
		core.Debug("button release: pushed=", pushed, ", hover=", this.IsHover(),
			", enabled=", this.IsEnabled(), ", visible=", this.IsAllAncentorsVisible())
	}
	this.Self().Update()
	this.PopCapture()
	if this.action != nil && pushed && this.IsHover() && this.IsEnabled() {
		this.emit()
	}
}

func (this *Button) emit() {
	if !this.IsEnabled() || !this.IsAllAncentorsVisible() {
		return
	}
	action := this.action
	if this.subPopup != nil {
		_, menuHeader := this.subPopup.(*Menu)
		if this.subPopup.IsVisible() {
			this.HideSubPopup()
		} else {
			this.ShowSubPopup()
		}
		if menuHeader {
			return // A submenu header opens its menu, it is not a leaf command.
		}
	}
	if this.subPopup == nil && this.IsInPopupMenu() {
		root := findRootPopup(this.Self())
		if root != nil {
			root.Hide()
		}
	}
	// Hide callbacks can disable or remove the action. Re-check availability,
	// but not visibility after our own root.Hide; retain the clicked command.
	if this.IsEnabled() && action.IsEnabled() {
		action.Trigger(this.Self())
	}
}

// Compile-time check: Button must satisfy IEventKeyDown so the window routes
// keys to a focused button, and focus.go's AutoFocus heuristic places it in
// the Tab chain.
var _ IEventKeyDown = (*Button)(nil)

// OnKeyDown implements IEventKeyDown so a focused button can be activated from
// the keyboard: Enter or Space run the same path as a mouse click (emit ->
// Action.Trigger / sub-popup toggle). Implementing this interface also opts
// the button into the Tab focus chain (see focus.go AutoFocus). Guarded on
// IsEnabled — which for a Button also implies a non-nil action — so a disabled
// button never activates. Escape still reaches its popup/dialog if focus was
// left on a button that became disabled.
func (this *Button) OnKeyDown(key int, repeat bool) {
	if !this.IsAllAncentorsVisible() {
		return
	}
	if key == KeyEsc && !repeat {
		for p := this.Parent(); p != nil; p = p.Parent() {
			if menu, ok := p.(IMenu); ok && p.IsPopup() {
				if handler, ok := menu.(IEventKeyDown); ok {
					handler.OnKeyDown(key, false)
				}
				return // Esc closes the popup, never its owning dialog too.
			}
			if dialog, ok := p.(interface{ resolveCancel() }); ok {
				dialog.resolveCancel()
				return
			}
		}
	}
	if !this.IsEnabled() {
		return
	}
	switch key {
	case KeyEnter, KeySpace:
		if !repeat {
			this.emit()
		}
	}
}

func (this *Button) OnHide() {
	this.pushed = false
	this.HideSubPopup()
}

func (this *Button) OnMouseStop(x, y float64) {
	//this.Ow().Update()
	//x, y = this.MapToGlobal(x, y)
	if this.subPopup != nil && this.IsInPopupMenu() && !this.IsSubPopupVisible() {
		this.ShowSubPopup()
	}

}

func (this *Button) Text() string {
	if this.action != nil {
		return this.action.Text()
	}
	return "<nil>"
}

func (this *Button) Icon() paint.Icon {
	var ico paint.Icon
	if this.action != nil {
		ico = this.action.Icon()
	}
	return ico
}

func (this *Button) IsEnabled() bool {
	if !this.Widget.IsEnabled() {
		return false
	}
	if this.action != nil {
		return this.action.IsEnabled()
	}
	return false
}

func (this *Button) SizeHints() SizeHints {
	// Fast path: check cache. SizeHints depends on
	//   1. Action.MTime (text/icon/enabled/checked changes)
	//   2. textVisible local flag
	//   3. parent (controls IsInPopupMenu via IMenu type assertion)
	//   4. theme revision (font, margins, icon size)
	// Resolved IsTextVisible / IsIconVisible / IsInPopupMenu collapse to
	// (action state, textVisible, parent), so the cache key above is closed.
	if this.hintsValid {
		var rev uint64
		if this.action != nil {
			rev = this.action.Rev()
		}
		if rev == this.hintActionRev &&
			this.hintThemeRev == themeRev &&
			this.hintParent == this.parent &&
			this.hintTextVis == this.textVisible {
			return this.cachedHints
		}
	}

	hints := this.computeSizeHints()

	if this.action != nil {
		this.hintActionRev = this.action.Rev()
	} else {
		this.hintActionRev = 0
	}
	this.hintThemeRev = themeRev
	this.hintParent = this.parent
	this.hintTextVis = this.textVisible
	this.cachedHints = hints
	this.hintsValid = true
	return hints
}

// computeSizeHints performs the original (uncached) computation. Pulled out
// of SizeHints() so the cache wrapper stays small and the slow path remains
// auditable.
func (this *Button) computeSizeHints() SizeHints {
	t := Theme()
	if menu, ok := this.Parent().(*Menu); ok && menu.compact && !menu.IsVertical() && !menu.IsPopup() {
		if !this.IsTextVisible() {
			side := math.Ceil(math.Max(28, t.IconSize+8))
			return SizeHints{Width: side, Height: side, Policy: GrowVertical}
		}
		ext := t.Font.TextExtents(this.Text())
		w := math.Max(ext.Width, ext.XAdvance) + 20
		if this.IsIconVisible() {
			w += t.IconSize + 6
		}
		return SizeHints{Width: math.Ceil(w), Height: math.Ceil(math.Max(26, t.Font.FontExtents().Height+6)), Policy: GrowVertical}
	}

	if this.IsTextVisible() {

		fe := t.Font.FontExtents()
		ext := t.Font.TextExtents(this.Text())
		if this.IsInPopupMenu() {
			//ml, mr, mt, mb := t.MenuItemMargin.Margin()
			m := t.MenuItemMargin
			w := ext.Width + m.L*2 + m.R + t.IconSize + t.MenuSubMarkWidth
			if this.shortcutText != "" {
				w += t.Font.TextExtents(this.shortcutText).Width + m.L*2
			}
			h := math.Max(t.IconSize, fe.Height) + m.T + m.B
			return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
		} else if this.IsIconVisible() {
			m := t.ButtonMargin
			h := math.Max(t.IconSize, fe.Height)
			w := ext.Width + h
			w += m.L*2 + m.R
			h += m.T + m.B
			return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
		} else {
			m := t.ButtonMargin
			h := fe.Height
			w := ext.Width
			w += m.L + m.R
			h += m.T + m.B
			return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
		}
	} else {
		m := t.ButtonMargin
		w := m.L + t.IconSize + m.R
		h := m.T + t.IconSize + m.B
		//core.Debug(w, h)
		return SizeHints{Width: w, Height: h, Policy: GrowHorizontal | GrowVertical}
	}
}

// invalidateHints marks the SizeHints cache stale. Called by setters that
// don't flow through Action.MTime (SetTextVisible) and could otherwise leave
// the cache pointing at obsolete metrics.
func (this *Button) invalidateHints() {
	this.hintsValid = false
}

func (this *Button) SetSubPopup(iw IWidget) {
	if this.subPopup != nil && this.subPopup != iw {
		this.HideSubPopup()
	}
	this.subPopup = iw
	if iw != nil {
		iw.SetVisible(false)
	}
}

func (this *Button) SubPopup() IWidget {
	return this.subPopup
}

func (this *Button) ShowSubPopup() {
	if !this.IsEnabled() || !this.IsAllAncentorsVisible() || this.subPopup == nil || this.IsSubPopupVisible() {
		return
	}

	owner := this.OwnerMenu()
	if owner != nil {
		owner.HideAllSubs()
	}

	if this.cbSubPopup != nil {
		this.cbSubPopup(this.self.(IButton))
	}
	if !this.IsEnabled() || !this.IsAllAncentorsVisible() || this.subPopup == nil {
		return
	}

	this.subPopup.SetParent(this)
	this.subPopup.AttachWindow(WtPopup)
	if this.subPopup.Window() == nil {
		return
	}

	hints := this.subPopup.SizeHints()
	this.subPopup.SetSize(0, 0)
	this.subPopup.SetSize(hints.Width, hints.Height)
	x, y := this.MapToGlobal(0, 0)
	w, h := this.Size()
	popupVertical := !this.IsInPopupMenu()
	var overlap float64
	if popupVertical {
		overlap = 1
	} else {
		overlap = 3
	}
	LayoutPopup(this.subPopup, x, y, w, h, popupVertical, overlap)
	this.subPopup.Show()
	this.subPopup.PushCapture()
	if menu, ok := this.subPopup.(*Menu); ok {
		menu.setHighlight(-1)
		menu.SetFocus()
	}
	this.Self().Update()

}

func (this *Button) HideSubPopup() {
	if this.subPopup == nil {
		return
	}
	this.subPopup.Hide()
}

func (this *Button) SetSubPopupCallback(fn func(IButton)) {
	this.cbSubPopup = fn
}

func (this *Button) OwnerMenu() IMenu {
	m, _ := this.parent.(IMenu)
	return m
}

func (this *Button) IsInPopupMenu() bool {
	if this.parent == nil {
		return false
	}
	if _, ok := this.parent.(IMenu); !ok {
		return false
	}
	return this.parent.IsPopup()
}

func (this *Button) IsSubPopupVisible() bool {
	return this.subPopup != nil &&
		this.subPopup.IsVisible() &&
		this.subPopup.IsAllAncentorsVisible()
}

func (this *Button) SetAction(a IAction) {
	this.action = a
	this.Self().Update()
}

func (this *Button) Action() IAction {
	if this.action == nil {
		this.action = new(Action)
	}
	return this.action
}

func (this *Button) SetText(text string) {
	this.Action().SetText(text)
}

func (this *Button) SetIcon(icon paint.Icon) {
	// An icon given directly has no name to round-trip, so the old one must go:
	// keeping it would have the design file save, and codegen emit, a name that
	// no longer matches the icon actually on the button.
	this.iconName = ""
	this.Action().SetIcon(icon)
}

func (this *Button) IconName() string {
	return this.iconName
}

// SetIconName loads the icon by resource name (LoadIcon resolves it under
// core.ResourceDir()) and remembers the name, which is what makes an icon
// settable from the designer's property sheet: the name persists into the
// design file and comes back out as generated code. "" clears both.
func (this *Button) SetIconName(name string) {
	if name == "" {
		this.SetIcon(nil)
		return
	}
	this.SetIcon(LoadIcon(name))
	// After SetIcon, which clears the name it cannot know.
	this.iconName = name
}

//func (this *Button) IsTextVisible() bool {
//	return this.Text() != ""
//}

func (this *Button) IsPushed() bool {
	return this.pushed
}

// SetDefault marks this button as the default one — the button Enter
// activates in a dialog. The theme rings it like a focused button so the Enter
// target is visible before focus ever reaches it. Keeping exactly one button
// flagged is the dialog's job, see Dialog.markDefaultButton.
func (this *Button) SetDefault(b bool) {
	if this.isDefault == b {
		return
	}
	this.isDefault = b
	this.Self().Update()
}

func (this *Button) IsDefault() bool {
	return this.isDefault
}

func (this *Button) IsChecked() bool {
	if iface, ok := this.Action().(interface {
		IsChecked() bool
	}); ok {
		return iface.IsChecked()
	}
	return false
}

func (this *Button) asButton() {

}

func (this *Button) OnIdle() {
	a := this.Action()
	if rev := a.Rev(); rev != this.syncRev {
		this.Update()
		this.syncRev = rev
	}
}

func (this *Button) IsTextVisible() bool {

	if this.IsInPopupMenu() {
		return true
	}

	if this.Text() == "" {
		return false
	}

	if this.textVisible {
		return true
	}

	// 没有图标时, 文本可见
	if this.Icon() == nil {
		return true
	}

	return false
}

func (this *Button) SetTextVisible(b bool) {
	if this.textVisible == b {
		return
	}
	this.textVisible = b
	this.invalidateHints()
}

// ShortcutText is a right-aligned hint in popup menus, not a key binding.
func (this *Button) ShortcutText() string { return this.shortcutText }

func (this *Button) SetShortcutText(text string) {
	if this.shortcutText == text {
		return
	}
	this.shortcutText = text
	this.invalidateHints()
	if menu, ok := this.Parent().(*Menu); ok {
		menu.Layout()
	}
	this.Update()
}

func (this *Button) IsIconVisible() bool {
	if !this.IsTextVisible() {
		return true
	}
	return this.Icon() != nil
}

func (this *Button) ExportGv(g *gv.Graph) {
	this.Widget.ExportGv(g)
	self := this.Self()
	node := g.Node(self)
	if this.subPopup != nil {
		edge := g.Edge(node, this.SubPopup())
		edge.Color = "pink"
		edge.TextColor = edge.Color
		edge.Text = "SubPopup"
	}
}

// Cache the material face per widget; do not allocate a Cairo gradient on every
// repaint. Size and colors form the key, so theme changes cannot reuse stale ink.
func (this *Button) normalFace(top, bottom paint.Color, height float64) *paint.LinearGradient {
	if this.face == nil || this.faceHeight != height || this.faceTop != top || this.faceBottom != bottom {
		this.face = paint.NewLinearGradient(0, 0, 0, float32(height))
		this.face.AddStop(0, top)
		this.face.AddStop(1, bottom)
		this.faceHeight, this.faceTop, this.faceBottom = height, top, bottom
	}
	return this.face
}

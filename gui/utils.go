package gui

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
	"reflect"
)

// 此函数功能同reflect.TypeOf, 放在这里是为了便于使用
func TypeOf(i interface{}) reflect.Type {
	return reflect.TypeOf(i)
}

func GetDbgText(o interface{}) string {
	var text string
	if iTitle, ok := o.(ITitle); ok {
		text = iTitle.Title()
	} else if iText, ok := o.(IText); ok {
		text = iText.Text()
	} else if iString, ok := o.(IString); ok {
		text = iString.String()
	}
	return EllipsisText(text, 16)
}

func EllipsisText(s string, maxCharCount int) string {
	if maxCharCount-3 <= 0 {
		return ""
	}
	text := []rune(s)
	n := len(text)
	if n <= maxCharCount-3 {
		return s
	}

	return string(text[:maxCharCount-3]) + "..."
}

func FindOwnerFrame(iw IWidget) *Frame {
	for iw != nil {
		if frame, ok := iw.(*Frame); ok {
			return frame
		}
		iw = iw.Parent()
	}
	return nil
}

func FindOwnerDock(iw IWidget) IDock {
	for iw != nil {
		if dock, ok := iw.(IDock); ok {
			return dock
		}
		iw = iw.Parent()
	}
	return nil
}

func PromptSaveClose(parent IWidget, a interface{}) bool {
	idirty, _ := a.(interface {
		DirtyList() []string
	})
	isave, _ := a.(interface {
		Save() bool
	})
	iclose, _ := a.(interface {
		Close()
	})

	if idirty != nil {
		core.Debug(`object ` + core.ObjInfo(a) + ` has "DirtyList" method, try prompt and save.`)
		if isave == nil {
			core.Warn(`object ` + core.ObjInfo(a) + ` has not "Save" method.`)
		} else {
			dlist := idirty.DirtyList()
			if len(dlist) > 0 {
				core.Debug(`prompt save contents for object ` + core.ObjInfo(a))
				var msg string
				if len(dlist) == 1 {
					msg = dlist[0] + " 已经修改, 是否保存?"
				} else {
					msg = "以下内容已经修改, 是否保存?\n"
					for _, v := range dlist {
						msg = msg + "\n" + v
					}
				}
				ret := ShowMessageBox(parent,
					LoadIcon("question"),
					"Save",
					msg,
					[]string{"@save", "@discard", "@cancel"})
				switch ret {
				case "@save":
					core.Debug("user click @save button, try save contents.")
					if !isave.Save() {
						core.Debug("failed in save")
						return false
					}
				case "@discard":
					core.Debug("user click @dicard button, close without save.")
				default:
					core.Debug(`return code from messagebox is "`, ret, `", user cancel action, nothing to do.`)
					return false
				}
			} else {
				core.Debug(`dirty list of object ` + core.ObjInfo(a) + ` is empty, no need to save.`)
			}
		}
	} else if isave != nil {
		core.Debug(`object ` + core.ObjInfo(a) + ` has not "DirtyList" method, try save without prompt.`)
		if !isave.Save() {
			return false
		}
	}

	if iclose == nil {
		core.Debug(`object ` + core.ObjInfo(a) + ` has not Close() method`)
	} else {
		core.Debug(`try close the object ` + core.ObjInfo(a))
		iclose.Close()
	}

	iw, ok := a.(IWidget)
	if ok && iw.Parent() != nil {
		iw.Detach()
	}
	return true
}

func LayoutPopup1(popup IWidget, xref, yref float64) {
	LayoutPopup(popup, xref, yref, 0, 0, false, 0)
}

func LayoutPopup(popup IWidget, xref, yref, wref, href float64, vertical bool, overlap float64) {
	area := popupWorkAreaAt(xref, yref)
	if menu, ok := popup.(*Menu); ok && menu.IsPopup() {
		menu.fitPopupWorkArea(area)
	}
	pw, ph := popup.Size()
	x, y := popupPositionInWorkArea(area, xref, yref, wref, href, pw, ph, vertical, overlap)
	// Set popup position directly using global screen coordinates.
	// Previously this did MapFromGlobal + SetPos which triggered
	// Window.SetPos doing MapToGlobal again - the double conversion
	// corrupted position on repeated open/close.
	setPopupGlobalPos(popup, x, y)
}

var appIcon paint.Icon

// SetAppIcon opts a host into an explicit application icon before creating its
// windows. Passing nil restores the executable-name lookup behavior.
func SetAppIcon(icon paint.Icon) {
	appIcon = icon
}

func AppIcon() paint.Icon {
	if appIcon == nil {
		name := core.ExeFileBaseName(false)
		appIcon = LoadIcon(name)
	}
	return appIcon
}

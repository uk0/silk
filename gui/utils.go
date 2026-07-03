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
	x0, y0 := xref, yref
	x1, y1 := x0+wref, y0+href
	if vertical {
		y0 += overlap
		y1 -= overlap
	} else {
		x0 += overlap
		x1 -= overlap
	}

	dx0, dy0, dw, dh := DesktopArea()
	dx1, dy1 := dx0+dw, dy0+dh

	pw, ph := popup.Size()

	var x, y float64
	if vertical {
		if y1+ph > dy1-32 {
			x, y = x0, y0-ph
		} else {
			x, y = x0, y1
		}
	} else {
		if x1+pw > dx1-32 {
			x, y = x0-pw, y0
		} else {
			x, y = x1, y0
		}
	}
	if x+pw > dx1 {
		x = dx1 - pw
	}
	if x < dx0 {
		x = dx0
	}
	if y+ph > dy1 {
		y = dy1 - ph
	}
	if y < dy0 {
		y = dy0
	}
	// Set popup position directly using global screen coordinates.
	// Previously this did MapFromGlobal + SetPos which triggered
	// Window.SetPos doing MapToGlobal again - the double conversion
	// corrupted position on repeated open/close.
	setPopupGlobalPos(popup, x, y)
}

var appIcon paint.Icon

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

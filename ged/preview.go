package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

// 预览 hosting.
//
// The previewed tree itself comes from GedScene.Generate — the same build the
// code generator models, and one that leaves the design untouched (see
// preview_test.go). What lives here is only what the *window* around it must
// not get wrong: it has to be recognisable as a preview, it must never open
// missing part of the design, and closing it must leave the designer as it was.

// The smallest window a preview opens at. A form designed smaller than this
// would come up as a sliver of title bar with no room for its own borders.
const (
	previewMinWidth  = 320.0
	previewMinHeight = 240.0
)

// previewSize raises a form's size to the floor above, leaving anything larger
// alone.
func previewSize(w, h float64) (float64, float64) {
	if w < previewMinWidth {
		w = previewMinWidth
	}
	if h < previewMinHeight {
		h = previewMinHeight
	}
	return w, h
}

// previewTitle builds the preview window's title.
//
// unbound marks a design whose widgets are bound to tags: the preview starts no
// drivers and no scada.Services, so every such widget shows its design-time
// value forever. Saying so in the title is what stops a static number from
// being read as a live reading off the plant.
func previewTitle(formTitle string, unbound bool) string {
	t := "预览"
	if formTitle != "" {
		t += " - " + formTitle
	}
	if unbound {
		t += " (未连接数据)"
	}
	return t
}

// previewBlockerLabel names a widget the preview cannot build, for the dialog
// that reports it. The designer's own name for it comes first because that is
// what the user selects it by; the factory name follows so an unnamed widget is
// still identifiable.
func previewBlockerLabel(name, factoryName string) string {
	if factoryName == "" {
		factoryName = "未知类型"
	}
	if name == "" {
		return factoryName
	}
	return name + " (" + factoryName + ")"
}

// previewBlockers lists every widget in the scene that FakeWidget.Generate
// cannot construct, in scene order.
//
// Generate builds each widget through NewFakeWidgetFromFactory and returns nil
// when that fails, and both callers — GedScene.Generate and the nested-child
// loop — skip a nil silently. So an unbuildable widget does not fail the
// preview, it disappears from it, together with everything nested inside it;
// that is why a blocker's children are not walked here either.
//
// The condition tested is factory registration, the one that varies between a
// document and the program reading it. A FakeWidget's factory name is always
// derived from a live widget instance (SetWidget -> core.FactoryNameOf), so the
// other half of Generate's test — that the factory makes a gui.IWidget — cannot
// fail for a name that resolves at all, and is not worth constructing every
// widget in the design a second time to ask.
func previewBlockers(scene *GedScene) []string {
	var out []string
	var walk func(items []graph.IItem)
	walk = func(items []graph.IItem) {
		for _, item := range items {
			fake, ok := item.(*FakeWidget)
			if !ok {
				continue
			}
			if core.FindFactory(fake.WidgetFactoryName()) == nil {
				out = append(out, previewBlockerLabel(fake.WidgetName(), fake.WidgetFactoryName()))
				continue
			}
			if fake.HasChildren() {
				walk(fake.Children())
			}
		}
	}
	walk(scene.Children())
	return out
}

// PreviewForm is the window root of a 预览. It owns the title, Esc and handing
// keyboard focus back, and nothing else: the generated form hangs underneath it
// exactly as GedScene.Generate produced it, so what the user sees is the tree
// the code generator would emit and not a re-interpretation of it.
type PreviewForm struct {
	gui.Form
	focusBack gui.IWidget
}

func newPreviewForm() *PreviewForm {
	p := new(PreviewForm)
	p.Init(p)
	return p
}

// BuildPreview builds the window root for a preview of scene. It returns
// (nil, blocked) when the design holds widgets the preview cannot build, so the
// caller can name them instead of opening a window that is quietly missing part
// of the design.
func BuildPreview(scene *GedScene) (*PreviewForm, []string) {
	if scene == nil {
		return nil, nil
	}
	if blocked := previewBlockers(scene); len(blocked) > 0 {
		return nil, blocked
	}
	design := scene.Generate()
	if design == nil {
		return nil, nil
	}
	form := design.Form()
	if form == nil {
		return nil, nil
	}

	host := newPreviewForm()
	host.SetTitle(previewTitle(scene.FormTitle(), len(sceneTagNames(scene)) > 0))
	w, h := previewSize(form.Size())
	host.SetSize(w, h)
	form.SetBounds(0, 0, w, h)
	form.SetParent(host)
	// Generate hands back a hidden form — it builds trees for callers that only
	// read them. Only the host gets Show()n, so without this the preview window
	// opens empty.
	form.SetVisible(true)
	return host, nil
}

// RestoreFocusTo names the widget that takes keyboard focus back when the
// preview closes. Keys are delivered to one global focus widget (window_glfw.go
// onKey), so a preview must take focus to receive Esc at all — otherwise Esc
// reaches the design canvas behind it and clears the designer's selection — and
// must give it back, because destroying a window does not clear that global.
func (this *PreviewForm) RestoreFocusTo(w gui.IWidget) {
	this.focusBack = w
}

// OnKeyDown closes the preview on Esc. Focus is handed back before the window
// goes, since the window is what routes the key that would otherwise land on a
// destroyed widget.
func (this *PreviewForm) OnKeyDown(key int, repeat bool) {
	if key != gui.KeyEsc {
		return
	}
	this.Close()
	if win := this.Window(); win != nil {
		win.Close()
	}
}

// Close hands keyboard focus back to the designer. Window.Close calls it on the
// way down, so the title-bar button restores focus exactly as Esc does; it is
// idempotent because Esc calls it first and the window then calls it again.
func (this *PreviewForm) Close() {
	if this.focusBack != nil {
		this.focusBack.SetFocus()
		this.focusBack = nil
	}
}

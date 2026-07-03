package gui

import (
	"errors"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
)

// LoadForm loads a UI design from a .silkui file (or any legacy design file
// produced by the Silk designer: .cml / .silk / .form) and returns a
// live Form widget that can be shown immediately.
//
// This entry point does NOT require the ged package, so any Go app can
// consume a designer-produced layout using just silk/core + silk/gui.
//
// Example:
//
//	form, err := gui.LoadForm("main.silkui")
//	if err != nil { log.Fatal(err) }
//	form.SetParent(mainFrame)
//	form.Show()
func LoadForm(filename string) (*Form, error) {
	doc, err := core.LoadTDocFile(filename)
	if err != nil {
		return nil, err
	}
	return LoadFormFromDoc(doc)
}

// LoadFormFromDoc creates a Form from an already-parsed TDoc. Useful when the
// design was loaded from an embedded resource or generated on the fly.
//
// The accepted document shape matches the output of ged.GedScene.SaveDesign:
//
//	root: val="form", WriteAttr("bounds", Rect), WriteAttr("title", string)
//	  "children":
//	    child: val=<factoryName>, WriteAttr("bounds", Rect), [WriteAttr("name", string)]
//	      ... nested children ...
//
// For forward compatibility, a few modern attribute keys are also accepted:
//
//   - root-level "form_title" / "form_w" / "form_h"
//   - per-widget "factory" (string) / "x" "y" "w" "h" (float, mm)
//   - per-widget "text" / "checked"
//   - nested children under a "widget"-keyed sub-node
func LoadFormFromDoc(doc *core.TDoc) (*Form, error) {
	if doc == nil {
		return nil, errors.New("gui.LoadFormFromDoc: nil doc")
	}

	form := NewForm()

	// Form title — legacy key is "title", modern alternative is "form_title".
	var title string
	_ = doc.ReadAttr("title", &title)
	if title == "" {
		_ = doc.ReadAttr("form_title", &title)
	}
	if title != "" {
		form.SetTitle(title)
	}

	// Form size — legacy stores a full Rect under "bounds"; modern alternative
	// is the pair form_w / form_h (in mm). Convert mm to pixels the same way
	// the designer does at runtime.
	var bounds geom.Rect
	if err := doc.ReadAttr("bounds", &bounds); err == nil && bounds.Width > 0 && bounds.Height > 0 {
		form.SetSize(MmToPixelZ(bounds.Width), MmToPixelZ(bounds.Height))
	} else {
		var w, h float64
		_ = doc.ReadAttr("form_w", &w)
		_ = doc.ReadAttr("form_h", &h)
		if w > 0 && h > 0 {
			form.SetSize(MmToPixelZ(w), MmToPixelZ(h))
		}
	}

	// Legacy layout puts widgets under a "children" sub-node (unkeyed subs).
	if childDoc := doc.ChildByKey("children", false); childDoc != nil {
		for _, sub := range childDoc.Childdren() {
			if w, err := loadWidget(sub); err == nil && w != nil {
				w.SetParent(form)
			}
		}
	}

	// Modern layout puts each child directly under the root with key="widget".
	for _, sub := range doc.Childdren() {
		if sub.Key() != "widget" {
			continue
		}
		if w, err := loadWidget(sub); err == nil && w != nil {
			w.SetParent(form)
		}
	}

	return form, nil
}

// loadWidget recursively materialises a widget from its TDoc representation.
//
// It accepts two structural dialects:
//
//  1. Designer-produced (primary): node value == factory name, Rect stored
//     under the "bounds" attribute, children either nested under a "children"
//     sub-node or as further unkeyed subs.
//  2. Modern / hand-written: factory name under the "factory" attribute,
//     bounds expressed as separate "x" / "y" / "w" / "h" float attrs, child
//     widgets as sub-docs with key="widget".
//
// Widget-specific fields (text, checked, ...) are applied via interface
// detection so the loader stays decoupled from concrete widget types.
func loadWidget(doc *core.TDoc) (IWidget, error) {
	if doc == nil {
		return nil, errors.New("gui.loadWidget: nil doc")
	}

	// Discover the factory name. Legacy designer files store it as the node
	// value; newer variants may use a dedicated "factory" / "factoryName"
	// attribute.
	var factoryName string
	_ = doc.Value(&factoryName)
	factoryName = strings.TrimSpace(factoryName)
	if factoryName == "" {
		_ = doc.ReadAttr("factory", &factoryName)
	}
	if factoryName == "" {
		_ = doc.ReadAttr("factoryName", &factoryName)
	}
	if factoryName == "" {
		return nil, errors.New("gui.loadWidget: missing factory name")
	}

	factory := core.FindFactory(factoryName)
	if factory == nil {
		return nil, errors.New("gui.loadWidget: unknown widget type: " + factoryName)
	}

	obj := factory.New()
	widget, ok := obj.(IWidget)
	if !ok {
		return nil, errors.New("gui.loadWidget: factory does not produce IWidget: " + factoryName)
	}

	// Bounds: try Rect attribute first, then fall back to x/y/w/h scalars.
	var rect geom.Rect
	if err := doc.ReadAttr("bounds", &rect); err == nil && (rect.Width != 0 || rect.Height != 0) {
		widget.SetBounds(
			MmToPixelZ(rect.X),
			MmToPixelZ(rect.Y),
			MmToPixelZ(rect.Width),
			MmToPixelZ(rect.Height),
		)
	} else {
		var x, y, w, h float64
		_ = doc.ReadAttr("x", &x)
		_ = doc.ReadAttr("y", &y)
		_ = doc.ReadAttr("w", &w)
		_ = doc.ReadAttr("h", &h)
		if w > 0 || h > 0 {
			widget.SetBounds(MmToPixelZ(x), MmToPixelZ(y), MmToPixelZ(w), MmToPixelZ(h))
		}
	}

	// Optional text label — applied via duck-typed interface so any widget
	// exposing SetText(string) is honoured.
	var text string
	_ = doc.ReadAttr("text", &text)
	if text != "" {
		if tw, ok := widget.(interface{ SetText(string) }); ok {
			tw.SetText(text)
		}
	}

	// Optional checked/boolean state for checkboxes / toggles.
	if sub := doc.ChildByKey("checked", false); sub != nil {
		var b bool
		if err := sub.Value(&b); err == nil {
			if cw, ok := widget.(interface{ SetChecked(bool) }); ok {
				cw.SetChecked(b)
			}
		}
	}

	// Optional generic value (sliders, progress bars, etc.).
	if sub := doc.ChildByKey("value", false); sub != nil {
		var fv float64
		if err := sub.Value(&fv); err == nil {
			if vw, ok := widget.(interface{ SetValue(float64) }); ok {
				vw.SetValue(fv)
			}
		}
	}

	// Recurse into children using either dialect.
	if childDoc := doc.ChildByKey("children", false); childDoc != nil {
		for _, sub := range childDoc.Childdren() {
			if cw, err := loadWidget(sub); err == nil && cw != nil {
				cw.SetParent(widget)
			}
		}
	}
	for _, sub := range doc.Childdren() {
		if sub.Key() != "widget" {
			continue
		}
		if cw, err := loadWidget(sub); err == nil && cw != nil {
			cw.SetParent(widget)
		}
	}

	return widget, nil
}

// SaveForm writes a Form's current widget hierarchy to a .silkui file using
// the same TDoc dialect that GedScene.SaveDesign produces, so files written
// here round-trip cleanly through LoadForm and through the visual designer.
//
// The output format is intentionally compatible with existing .cml designs:
// each widget node stores its factory name as the node value, its
// bounds under the "bounds" attribute (as a geom.Rect), and its children in
// a "children" sub-node.
//
// If the supplied filename has no extension, the default .silkui extension
// is appended automatically.
func SaveForm(form *Form, filename string) error {
	if form == nil {
		return errors.New("gui.SaveForm: nil form")
	}
	if filename == "" {
		return errors.New("gui.SaveForm: empty filename")
	}
	if !hasExt(filename) {
		filename += ".silkui"
	}

	doc := core.NewTDoc()
	if err := doc.SetValue("form"); err != nil {
		return err
	}

	// Convert pixel size back to mm so files stay resolution-independent.
	w, h := form.Size()
	_ = doc.WriteAttr("bounds", geom.Rect{
		X:      0,
		Y:      0,
		Width:  PixelToMm(w),
		Height: PixelToMm(h),
	})
	_ = doc.WriteAttr("title", form.Title())

	children := form.Children()
	if len(children) > 0 {
		childDoc := core.NewTDoc()
		childDoc.SetKey("children")
		for _, c := range children {
			if sub := serializeWidget(c); sub != nil {
				childDoc.AddChild(sub)
			}
		}
		doc.AddChild(childDoc)
	}

	return doc.SaveFile(filename)
}

// serializeWidget produces a TDoc node for a single live widget, matching the
// format FakeWidget.SaveDesign uses on disk.
func serializeWidget(w IWidget) *core.TDoc {
	if w == nil {
		return nil
	}
	name := core.FactoryNameOf(w)
	if name == "" {
		return nil
	}
	node := core.NewTDoc()
	_ = node.SetValue(name)
	x, y, ww, wh := w.Bounds()
	_ = node.WriteAttr("bounds", geom.Rect{
		X:      PixelToMm(x),
		Y:      PixelToMm(y),
		Width:  PixelToMm(ww),
		Height: PixelToMm(wh),
	})
	if tr, ok := w.(interface{ Text() string }); ok {
		if t := tr.Text(); t != "" {
			_ = node.WriteAttr("text", t)
		}
	}
	if ch, ok := w.(interface{ IsChecked() bool }); ok {
		if ch.IsChecked() {
			_ = node.WriteAttr("checked", true)
		}
	}
	kids := w.Children()
	if len(kids) > 0 {
		childDoc := core.NewTDoc()
		childDoc.SetKey("children")
		for _, k := range kids {
			if sub := serializeWidget(k); sub != nil {
				childDoc.AddChild(sub)
			}
		}
		node.AddChild(childDoc)
	}
	return node
}

// hasExt returns true if filename contains a non-trivial extension.
func hasExt(filename string) bool {
	for i := len(filename) - 1; i >= 0; i-- {
		switch filename[i] {
		case '.':
			// An extension of length zero (trailing dot) doesn't count.
			return i < len(filename)-1
		case '/', '\\':
			return false
		}
	}
	return false
}

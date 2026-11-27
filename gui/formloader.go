package gui

import (
	"errors"
	"fmt"
	"github.com/uk0/silk/decl"
	"reflect"

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
	// Older hand-authored SDK documents omit the root factory value. Keep
	// accepting that dialect without mutating a caller-owned/embedded TDoc.
	source := doc
	if !doc.HasValue() && doc.ChildByKey("factory", false) == nil && doc.ChildByKey("factoryName", false) == nil {
		source = doc.Clone()
		_ = source.SetValue("form")
	}
	node, err := decl.FromTDoc(source)
	if err != nil {
		return nil, fmt.Errorf("gui.LoadFormFromDoc: %w", err)
	}
	if node.Type != "gui.Form" {
		return nil, fmt.Errorf("gui.LoadFormFromDoc: root must be gui.Form, got %q", node.Type)
	}
	obj, err := buildLoadedWidget(node)
	if err != nil {
		return nil, err
	}
	form, ok := obj.(*Form)
	if !ok {
		return nil, fmt.Errorf("gui.LoadFormFromDoc: root factory returned %T, want *gui.Form", obj)
	}
	return form, nil
}

// loadWidget shares the same decoder, atomic property initialization and
// layout-cell registration as generated decl code. Nested absolute designer
// bounds are localized once by the decoder, never once per SDK/preview path.
func loadWidget(doc *core.TDoc) (IWidget, error) {
	node, err := decl.FromTDoc(doc)
	if err != nil {
		return nil, fmt.Errorf("gui.loadWidget: %w", err)
	}
	return buildLoadedWidget(node)
}

func buildLoadedWidget(node *decl.Node) (IWidget, error) {
	// BuildWithIndex normally indexes only named nodes, but this SDK loader
	// must reject *every* non-widget factory instead of silently losing a
	// nameless child. These private IDs belong only to the decoded AST, not
	// the source TDoc or live widgets; no user names are rewritten on disk.
	types := make(map[string]string)
	var identify func(*decl.Node)
	identify = func(n *decl.Node) {
		n.ID = fmt.Sprintf("load-%d", len(types))
		types[n.ID] = n.Type
		for _, child := range n.Children {
			if child != nil {
				identify(child)
			}
		}
	}
	identify(node)
	obj, index, err := node.BuildWithIndex()
	if err != nil {
		return nil, fmt.Errorf("gui.loadWidget: %w", err)
	}
	for id, value := range index {
		if _, ok := value.(IWidget); !ok {
			return nil, fmt.Errorf("gui.loadWidget: factory %q does not produce IWidget", types[id])
		}
	}
	widget, ok := obj.(IWidget)
	if !ok {
		return nil, fmt.Errorf("gui.loadWidget: factory %q does not produce IWidget", node.Type)
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

	// Store logical pixels at the same fixed 96 DPI used by decl/silkgen.
	// Monitor DPI must not alter an SDK form when it is saved and loaded.
	w, h := form.Size()
	_ = doc.WriteAttr("bounds", geom.Rect{
		X:      0,
		Y:      0,
		Width:  formLogicalPixelToMM(w),
		Height: formLogicalPixelToMM(h),
	})
	_ = doc.WriteAttr("title", form.Title())
	_ = doc.WriteAttr("bounds_space", "absolute")

	children := form.Children()
	if len(children) > 0 {
		childDoc := core.NewTDoc()
		childDoc.SetKey("children")
		for _, c := range children {
			if sub := serializeWidget(c); sub != nil {
				if c == form.LayoutRoot() {
					_ = sub.WriteAttr(decl.FormLayoutFillProp, true)
				}
				childDoc.AddChild(sub)
			}
		}
		doc.AddChild(childDoc)
	}

	return doc.SaveFile(filename)
}

// Designer files express document geometry in millimetres. SDK widgets use
// logical pixels, not device pixels, matching the generator's fixed 96 DPI.
func formLogicalPixelToMM(value float64) float64 { return value * 25.4 / 96 }

// serializeWidget produces a TDoc node for a single live widget, matching the
// format FakeWidget.SaveDesign uses on disk.
func serializeWidget(w IWidget) *core.TDoc {
	return serializeWidgetAt(w, 0, 0)
}

func serializeWidgetAt(w IWidget, parentX, parentY float64) *core.TDoc {
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
	x, y = x+parentX, y+parentY
	_ = node.WriteAttr("bounds", geom.Rect{
		X:      formLogicalPixelToMM(x),
		Y:      formLogicalPixelToMM(y),
		Width:  formLogicalPixelToMM(ww),
		Height: formLogicalPixelToMM(wh),
	})
	if tr, ok := w.(interface{ Text() string }); ok {
		_ = node.WriteAttr("text", tr.Text())
	}
	if ch, ok := w.(interface{ IsChecked() bool }); ok {
		_ = node.WriteAttr("checked", ch.IsChecked())
	}
	if manager, ok := w.(interface{ LayoutEnabled() bool }); ok {
		_ = node.WriteAttr("layout_managed", manager.LayoutEnabled())
	}
	if properties, ok := w.(core.IEnumProperties); ok {
		props := core.NewTDoc()
		props.SetKey("props")
		properties.EnumProperties(&formPropertyWriter{doc: props})
		if props.HasChildren() {
			node.AddChild(props)
		}
	}
	kids := w.Children()
	// An editable combo owns its Edit and popup internally. Rebuilding the
	// factory creates them; serializing them again manufactures duplicate UI.
	if _, ok := w.(*ComboBox); ok {
		kids = nil
	}
	switch w.(type) {
	case *Card, *GroupBox:
		if len(kids) == 1 {
			content := w.(interface{ Content() IWidget }).Content()
			if content == nil {
				_ = node.WriteAttr(decl.ContentLayoutProp, false)
			} else if manager, ok := content.(interface{ LayoutEnabled() bool }); ok && manager.LayoutEnabled() {
				_ = node.WriteAttr(decl.ContentLayoutProp, true)
			}
		}
	}
	if len(kids) > 0 {
		childDoc := core.NewTDoc()
		childDoc.SetKey("children")
		for _, k := range kids {
			if sub := serializeWidgetAt(k, x, y); sub != nil {
				saveFormLayoutCell(w, k, sub)
				childDoc.AddChild(sub)
			}
		}
		node.AddChild(childDoc)
	}
	return node
}

// Preserve the same explicit property-sheet contract used by the designer,
// including zero, false and empty strings. No event/binding code is executed.
type formPropertyWriter struct{ doc *core.TDoc }

func (p *formPropertyWriter) AddProperty(id string, get, set interface{}) {
	if get == nil || set == nil {
		return
	}
	fn := reflect.ValueOf(get)
	if fn.Kind() != reflect.Func || fn.IsNil() || fn.Type().NumIn() != 0 || fn.Type().NumOut() != 1 {
		return
	}
	value := fn.Call(nil)[0].Interface()
	if raw, err := core.PersistString(value); err == nil {
		_ = p.doc.WriteAttr(id, raw)
	}
}

func saveFormLayoutCell(parent, child IWidget, doc *core.TDoc) {
	row, col, rowSpan, colSpan, found := 0, 0, 1, 1, false
	switch layout := parent.(type) {
	case *GridLayout:
		for _, cell := range layout.cells {
			if cell.widget == child {
				row, col, rowSpan, colSpan, found = cell.row, cell.col, cell.rowSpan, cell.colSpan, true
				break
			}
		}
	case *FormLayout:
		for i, pair := range layout.rows {
			if pair.label == child {
				row, col, found = i, 0, true
				break
			}
			if pair.widget == child {
				row, col, found = i, 1, true
				break
			}
		}
	}
	if found {
		_ = doc.WriteAttr("layout_row", row)
		_ = doc.WriteAttr("layout_column", col)
		_ = doc.WriteAttr("layout_row_span", rowSpan)
		_ = doc.WriteAttr("layout_column_span", colSpan)
	}
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

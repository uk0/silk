package gui

import (
	"fmt"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/jsonx"
)

func init() { core.RegisterFactory("gui.JSONView", core.TypeOf((*JSONView)(nil))) }

// JSONView is a read-only, lazily expanded JSON inspector. All widget methods
// and signals run on the UI thread. Parse large input in a worker and pass the
// resulting immutable document to SetDocument from gui.Post.
// JSONText is a DESIGN SAMPLE. LoadJSON/SetDocument never persist runtime data.
type JSONView struct {
	columnWidth float64
	TreeView
	doc                   *jsonx.Document
	jsonModel             *jsonViewModel
	jsonText, rootPointer string
	pageSize              int
	lastError             error
	selected              func(string)
	failed                func(string)
	notified              *jsonViewRow
}

func NewJSONView() *JSONView { v := new(JSONView); v.Init(v); return v }
func (v *JSONView) Init(self IWidget) {
	v.TreeView.Init(self)
	v.SetFocusPolicy(TabFocus)
	v.pageSize = 128
	v.jsonText = "{}"
	v.doc, _ = jsonx.Parse([]byte(v.jsonText))
	v.resetJSONModel(v.doc.Root())
	v.TreeView.SetActivatedCallback(func(_ any, _ ModelIndex) { v.notifyJSONSelection() })
}
func (v *JSONView) JSONText() string { return v.jsonText }
func (v *JSONView) SetJSONText(text string) {
	if err := v.LoadJSON(text); err == nil {
		v.jsonText = text
	}
}
func (v *JSONView) RootPointer() string { return v.rootPointer }
func (v *JSONView) SetRootPointer(pointer string) {
	root, err := v.doc.Lookup(pointer)
	if err != nil {
		v.jsonError(err)
		return
	}
	v.rootPointer = pointer
	v.lastError = nil
	v.resetJSONModel(root)
}
func (v *JSONView) PageSize() int { return v.pageSize }
func (v *JSONView) SetPageSize(size int) {
	size = max(16, min(256, size))
	if v.pageSize == size {
		return
	}
	v.pageSize = size
	root, err := v.doc.Lookup(v.rootPointer)
	if err == nil {
		v.resetJSONModel(root)
	}
}
func (v *JSONView) Document() *jsonx.Document   { return v.doc }
func (v *JSONView) LastError() error            { return v.lastError }
func (v *JSONView) SigSelected(fn func(string)) { v.selected = fn }
func (v *JSONView) SigError(fn func(string))    { v.failed = fn }
func (v *JSONView) jsonError(err error) error {
	v.lastError = err
	if err != nil && v.failed != nil {
		v.failed(err.Error())
	}
	return err
}

// LoadJSON is synchronous and bounded by jsonx's defaults. Failure preserves
// the previous document, expanded rows and current selection.
func (v *JSONView) LoadJSON(text string) error {
	d, err := jsonx.Parse([]byte(text))
	if err != nil {
		return v.jsonError(err)
	}
	return v.SetDocument(d)
}
func (v *JSONView) SetDocument(doc *jsonx.Document) error {
	root, err := doc.Lookup(v.rootPointer)
	if err != nil {
		return v.jsonError(err)
	}
	v.doc = doc
	v.lastError = nil
	v.resetJSONModel(root)
	return nil
}
func (v *JSONView) resetJSONModel(root jsonx.Node) {
	v.notified = nil
	v.jsonModel = newJSONViewModel(v, root)
	v.TreeView.SetModel(v.jsonModel)
	v.SetScrollY(0)
	v.Layout()
	v.Update()
}
func (v *JSONView) SelectedNode() (jsonx.Node, bool) {
	r := v.jsonModel.row(v.CurrentIndex())
	if r == nil || r.grouped {
		return jsonx.Node{}, false
	}
	return r.node, true
}
func (v *JSONView) notifyJSONSelection() {
	r := v.jsonModel.row(v.CurrentIndex())
	if r == v.notified {
		return
	}
	v.notified = r
	if r != nil && !r.grouped && v.selected != nil {
		v.selected(r.node.Pointer())
	}
}
func (v *JSONView) OnKeyDown(key int, repeat bool) {
	if !v.IsEnabled() {
		return
	}
	if key == 'C' && (IsKeyDown(KeyCtrl) || isActionModifier()) {
		var err error
		if IsKeyDown(KeyShift) {
			err = v.CopySelectedPointer()
		} else {
			err = v.CopySelectedJSON()
		}
		if err != nil {
			v.jsonError(err)
		}
		return
	}
	v.TreeView.OnKeyDown(key, repeat)
	v.notifyJSONSelection()
}
func (v *JSONView) SetCurrentRow(row int) { v.TreeView.SetCurrentRow(row); v.notifyJSONSelection() }

// SelectPointer expands only the ancestor/range chain needed to reveal a node.
// The pointer is absolute even when RootPointer displays a subtree.
func (v *JSONView) SelectPointer(pointer string) error {
	if _, err := v.doc.Lookup(pointer); err != nil {
		return v.jsonError(err)
	}
	tokens, _ := jsonx.SplitPointer(pointer)
	base, _ := jsonx.SplitPointer(v.rootPointer)
	if len(tokens) < len(base) {
		return v.jsonError(fmt.Errorf("JSONView: pointer outside displayed subtree"))
	}
	for i := range base {
		if base[i] != tokens[i] {
			return v.jsonError(fmt.Errorf("JSONView: pointer outside displayed subtree"))
		}
	}
	r := v.jsonModel.root
	for _, key := range tokens[len(base):] {
		index, err := r.node.ChildIndex(key)
		if err != nil {
			return v.jsonError(err)
		}
		for {
			v.TreeView.Expand(v.jsonModel.index(r, 0))
			v.jsonModel.materialize(r)
			start, _, chunk := v.jsonModel.span(r)
			r = r.children[(index-start)/chunk]
			if !r.grouped {
				break
			}
		}
	}
	row := v.TreeView.getRow(v.jsonModel.index(r, 0), false)
	if row == nil {
		return v.jsonError(jsonx.ErrNotFound)
	}
	v.lastError = nil
	v.SetCurrentRow(row.ri)
	return nil
}
func (v *JSONView) SelectedJSON() (string, error) {
	n, ok := v.SelectedNode()
	if !ok {
		return "", jsonx.ErrNotFound
	}
	b, err := n.JSON()
	return string(b), err
}
func (v *JSONView) CopySelectedJSON() error {
	text, err := v.SelectedJSON()
	if err != nil {
		return err
	}
	_, err = Clipboard.SetData(text)
	return err
}
func (v *JSONView) CopySelectedPointer() error {
	n, ok := v.SelectedNode()
	if !ok {
		return jsonx.ErrNotFound
	}
	_, err := Clipboard.SetData(n.Pointer())
	return err
}
func (v *JSONView) EnumProperties(list core.IPropertyList) {
	v.TreeView.EnumProperties(list)
	list.AddProperty("JSON Text", v.JSONText, v.SetJSONText)
	list.AddProperty("Root Pointer", v.RootPointer, v.SetRootPointer)
	list.AddProperty("Page Size", v.PageSize, v.SetPageSize)
}

// Recompute our proportional columns when the viewport changes. HeaderView
// otherwise caches sizes from construction (before a parent assigns bounds).
// Do not reset model rows or selection just to resize columns.
func (v *JSONView) Layout() {
	if v.header == nil {
		return
	}
	if v.header.Model() != nil && (v.columnWidth != v.Width() || v.header.SectionCount() != 3) {
		v.header.OnBeginReset()
		v.header.Layout()
		v.columnWidth = v.Width()
	}
	v.TreeView.Layout()
}

package decl

import (
	"math"
	"reflect"
)

// attachChild wires child as a child of parent at runtime, by calling
// any SetParent-shaped method the child exposes. We use reflection here
// because:
//
//   - parent's static type is not known to the decl package; widgets
//     define their own concrete interfaces (gui.IWidget, *gui.Form, ...)
//     and we don't want decl to depend on gui.
//   - reflect.MethodByName is one-time per child Build, so the cost
//     is dominated by the actual widget construction, not the lookup.
//
// SetParent is searched on the child (the typical Silk pattern: the
// child says "my parent is X"). If the method is missing or has a
// signature we cannot satisfy, the call is silently skipped. Designs
// that want strict attachment can build the IWidget tree themselves
// and bypass this helper.
//
// Method shape accepted:
//
//	func (c) SetParent(parent <T>)   where T is interface or struct
//	                                 pointer; we pass parent through
//	                                 a reflect.Value carrying its
//	                                 dynamic type, which assignability
//	                                 rules then narrow to T or fail.
//
// Methods returning a value are accepted; the return is ignored.
func attachChild(parent, child interface{}) {
	if parent == nil || child == nil {
		return
	}
	cv := reflect.ValueOf(child)
	method := cv.MethodByName("SetParent")
	if !method.IsValid() {
		return
	}
	mt := method.Type()
	if mt.NumIn() != 1 {
		return
	}
	pv := reflect.ValueOf(parent)
	paramType := mt.In(0)
	if !pv.Type().AssignableTo(paramType) {
		// Parent's static type is not assignable to the SetParent
		// argument. This is the common "parent is *concreteForm but
		// SetParent wants gui.IWidget" case — the assertion succeeds
		// because IWidget embeds the methods *concreteForm provides.
		// When AssignableTo says no, we genuinely cannot bridge.
		return
	}
	method.Call([]reflect.Value{pv})
}

// FormLayoutFillProp marks one direct layout child as filling its form.
const FormLayoutFillProp = "layout_fill_form"

// ContentLayoutProp records an explicit single managed layout (true) or
// authored free placement (false) within a Card or GroupBox.
const ContentLayoutProp = "layout_fill_content"

func usesContentSlot(n *Node) bool {
	if n.Type != "gui.Card" && n.Type != "gui.GroupBox" {
		return false
	}
	var only *Node
	for _, child := range n.Children {
		if child == nil {
			continue
		}
		if only != nil {
			return false // Never replace freely placed siblings one by one.
		}
		only = child
	}
	if only == nil {
		return false
	}
	mode, explicit := n.PropValue(ContentLayoutProp)
	if !explicit {
		return n.Type == "gui.GroupBox"
	}
	lit, ok := mode.(Lit)
	if !ok {
		return false
	}
	fill, ok := asBool(lit.V)
	if !ok || !fill {
		return false
	}
	if enabled, explicit := only.PropValue("layout_enabled"); explicit {
		lit, ok := enabled.(Lit)
		if !ok {
			return false
		}
		on, valid := asBool(lit.V)
		if !valid || !on {
			return false
		}
	}
	switch only.Type {
	case "gui.HBox", "gui.VBox", "gui.GridLayout", "gui.FormLayout":
		return true
	}
	return false
}

func attachFormLayoutRoot(form, child interface{}, node *Node) bool {
	raw, _ := node.PropValue(FormLayoutFillProp)
	lit, ok := raw.(Lit)
	if !ok {
		return false
	}
	fill, ok := asBool(lit.V)
	if !ok || !fill {
		return false
	}
	// A malformed marker on an ordinary widget must not move it.
	switch node.Type {
	case "gui.HBox", "gui.VBox", "gui.GridLayout", "gui.FormLayout":
	default:
		return false
	}
	call := reflect.ValueOf(form).MethodByName("SetLayoutRoot")
	value := reflect.ValueOf(child)
	if !call.IsValid() || !value.IsValid() {
		return false
	}
	mt := call.Type()
	if mt.NumIn() != 1 || !value.Type().AssignableTo(mt.In(0)) ||
		mt.NumOut() != 1 || mt.Out(0).Kind() != reflect.Bool {
		return false
	}
	return call.Call([]reflect.Value{value})[0].Bool()
}

// attachLayoutChild registers actual Grid/Form cells and invokes container
// adders. Plain SetParent does not establish private cell/content ownership.
func attachLayoutChild(parent, child interface{}, parentType string, node *Node, index int, singleContent bool) bool {
	if manager, ok := parent.(interface{ LayoutEnabled() bool }); ok && !manager.LayoutEnabled() {
		return false
	}
	var method string
	var args []reflect.Value
	switch parentType {
	case "gui.GridLayout", "gui.FormLayout":
		method = "AddLayoutWidget"
		values := []int{index / 2, index % 2, 1, 1}
		for i, key := range []string{"layout_row", "layout_column", "layout_row_span", "layout_column_span"} {
			if val, ok := node.PropValue(key); ok {
				if lit, ok := val.(Lit); ok {
					if number, ok := asFloat(lit.V); ok && !math.IsNaN(number) && !math.IsInf(number, 0) && number >= 0 && number <= 4095 {
						values[i] = int(number)
					}
				}
			}
		}
		if values[2] < 1 {
			values[2] = 1
		}
		if values[3] < 1 {
			values[3] = 1
		}
		if parentType == "gui.FormLayout" {
			values[1] %= 2
		}
		args = []reflect.Value{reflect.ValueOf(child)}
		for _, n := range values {
			args = append(args, reflect.ValueOf(n))
		}
	case "gui.Card", "gui.GroupBox":
		if !singleContent {
			return false
		}
		fallthrough
	case "gui.HBox", "gui.VBox":
		method = "AddWidget"
		args = []reflect.Value{reflect.ValueOf(child)}
	default:
		return false
	}
	m := reflect.ValueOf(parent).MethodByName(method)
	if !m.IsValid() || m.Type().NumIn() != len(args) {
		return false
	}
	for i, a := range args {
		if !a.IsValid() || !a.Type().AssignableTo(m.Type().In(i)) {
			return false
		}
	}
	m.Call(args)
	return true
}

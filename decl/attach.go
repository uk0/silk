package decl

import "reflect"

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

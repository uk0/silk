package decl

import (
	"fmt"
	"math"
	"reflect"
	"strconv"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/i18n"
)

// Widget capability shapes used by applyProp's interface assertions.
// Each one mirrors the duck-typed setters formloader.go already relies
// on so any widget that opted into those interfaces is automatically
// reachable from a decl Lit prop.
type (
	textSetter    interface{ SetText(string) }
	checkedSetter interface{ SetChecked(bool) }
	valueSetter   interface{ SetValue(float64) }
	titleSetter   interface{ SetTitle(string) }
)

// Build instantiates the widget tree rooted at n. The returned object
// is the concrete value produced by the registered factory; callers
// that know the type can type-assert (e.g. *gui.Form). The runtime
// makes no assumption beyond "the factory returns a value with at
// least the prop setters the props request" — passing a decl tree
// with a wrong factory name surfaces as an error here, but a wrong
// prop name silently no-ops (matches formloader.go's behaviour).
//
// Children are attached via SetParent. The runtime does NOT require a
// concrete IWidget interface; any object that implements SetParent
// (with a single argument matching the parent's static type) is
// assembled into the tree. When SetParent is missing, children are
// still Built — they just don't get auto-parented.
//
// For applications that need to wire event handlers after construction,
// see BuildWithIndex which additionally returns a map from Node.ID to
// the corresponding widget instance.
func (n *Node) Build() (interface{}, error) {
	obj, _, err := n.BuildWithIndex()
	return obj, err
}

// BuildWithIndex behaves like Build but additionally collects every
// Node with a non-empty ID into a flat map keyed by that ID. Apps can
// then look up specific widgets by their declared name and bind event
// handlers, install reactive bindings, or imperatively mutate state
// without walking the widget tree.
//
// Duplicate IDs in a single tree silently overwrite the earlier entry —
// designer-authored docs already enforce uniqueness at the index layer
// so this is the right policy for runtime users too. If your tooling
// is generating decl trees programmatically and may emit duplicates,
// validate before calling Build().
func (n *Node) BuildWithIndex() (interface{}, map[string]interface{}, error) {
	idx := make(map[string]interface{})
	root, err := buildInto(n, idx)
	return root, idx, err
}

func buildInto(n *Node, idx map[string]interface{}) (interface{}, error) {
	if n == nil {
		return nil, fmt.Errorf("decl.Build: nil node")
	}

	factory := core.FindFactory(n.Type)
	if factory == nil {
		return nil, fmt.Errorf("decl.Build: unknown widget type %q", n.Type)
	}
	obj := factory.New()
	if obj == nil {
		return nil, fmt.Errorf("decl.Build: factory %q returned nil", n.Type)
	}

	handled := applyCompoundProperties(obj, n.Props)
	for _, p := range n.Props {
		if !handled[p.Name] {
			applyProp(obj, p)
		}
	}

	applyGeometry(obj, n)

	if n.ID != "" {
		idx[n.ID] = obj
	}

	for childIndex, c := range n.Children {
		if c == nil {
			continue
		}
		child, err := buildInto(c, idx)
		if err != nil {
			return nil, fmt.Errorf("decl.Build: %s child: %w", n.Type, err)
		}
		// Use reflection-free duck-typing: the parent's static type is
		// not known here, so we go through a typed reflective adapter
		// in attach.go. Inlined check kept simple: only attach when
		// the child exposes a SetParent that accepts an interface{}
		// or our concrete obj.
		if !attachLayoutChild(obj, child, n.Type, c, childIndex) {
			attachChild(obj, child)
		}
	}

	if layout, ok := obj.(interface{ Layout() }); ok {
		layout.Layout()
	}
	return obj, nil
}

// applyProp handles the small fixed set of well-known prop names that
// every Silk widget honours via duck-typed interfaces. Unknown prop
// names are intentionally silent — the design echoes formloader.go's
// "best-effort apply" semantics so older designs with attribs the
// runtime no longer cares about still load cleanly.
//
// Value variants:
//   - Lit: pass through to the duck-typed setter
//   - TrKey: resolve via i18n.T() then treat as a string Lit. This
//     is what makes designer-authored .silkui files automatically
//     localise — the widget's text/title/etc. is whatever the active
//     locale's translator returns for the source string.
//   - Ref / Bind / Expr: skipped here. Higher-level callers walk the
//     AST first, install event/binding wiring, then call Build.
func applyProp(obj interface{}, p Prop) {
	var lit Lit
	switch v := p.Value.(type) {
	case Persisted:
		applyPersistedProperty(obj, p.Name, v.Source)
		return
	case Lit:
		lit = v
	case TrKey:
		// Resolve translation at Build time. The translated string
		// becomes a string-typed Lit for the duck-typed setters.
		lit = Lit{V: i18n.T(v.Source)}
	default:
		return
	}
	switch p.Name {
	case "selectedIndex", "activeIndex":
		if setter, ok := obj.(interface{ SetActiveIndex(int) }); ok {
			if value, ok := asFloat(lit.V); ok && math.Trunc(value) == value && value >= -math.Ldexp(1, strconv.IntSize-1) && value < math.Ldexp(1, strconv.IntSize-1) {
				setter.SetActiveIndex(int(value))
				return
			}
		}
	case "layout_enabled":
		if enabled, ok := asBool(lit.V); ok {
			if manager, ok := obj.(interface{ SetLayoutEnabled(bool) }); ok {
				manager.SetLayoutEnabled(enabled)
				return
			}
		}
	case "text", "Text":
		if s, ok := lit.V.(string); ok {
			if tw, ok := obj.(textSetter); ok {
				tw.SetText(s)
				return
			}
		}
	case "title", "Title":
		if s, ok := lit.V.(string); ok {
			if tw, ok := obj.(titleSetter); ok {
				tw.SetTitle(s)
				return
			}
		}
	case "checked", "Checked":
		if b, ok := asBool(lit.V); ok {
			if cw, ok := obj.(checkedSetter); ok {
				cw.SetChecked(b)
				return
			}
		}
	case "value", "Value":
		if setter, ok := obj.(interface{ SetValue(int) }); ok {
			if value, valid := propertyInteger(lit); valid {
				setter.SetValue(value)
				return
			}
		}
		if f, ok := asFloat(lit.V); ok {
			if vw, ok := obj.(valueSetter); ok {
				vw.SetValue(f)
				return
			}
		}
	}
	applyLiteralProperty(obj, p.Name, lit.V)
}

// applyGeometry updates all four coordinates together, preserving factory
// defaults for omitted dimensions. Repeated SetWidth/SetHeight calls can
// trigger intermediate layout and lose the intended bounds.
func applyGeometry(obj interface{}, n *Node) {
	w, ok := obj.(interface {
		Bounds() (float64, float64, float64, float64)
		SetBounds(float64, float64, float64, float64)
	})
	if !ok {
		return
	}
	x, y, width, height := w.Bounds()
	changed := false
	for _, p := range []struct {
		name string
		dst  *float64
	}{{"x", &x}, {"y", &y}, {"width", &width}, {"height", &height}} {
		v, exists := n.PropValue(p.name)
		lit, literal := v.(Lit)
		if !exists || !literal {
			continue
		}
		value, valid := asFloat(lit.V)
		if !valid || math.IsNaN(value) || math.IsInf(value, 0) || ((p.name == "width" || p.name == "height") && value < 0) {
			continue
		}
		*p.dst, changed = value, true
	}
	if changed {
		w.SetBounds(x, y, width, height)
	}
}

// asBool / asFloat handle the narrow set of numeric-vs-string
// promotions the persist layer can leave in a Lit after a TDoc
// round-trip. After ToTDoc → FromTDoc, an int literal may come back
// as a string (PersistSscan returns the raw token). We accept both
// the strongly-typed form and the string form to keep the runtime
// agnostic to how the AST was authored.
func asBool(v interface{}) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		switch x {
		case "true", "True", "1":
			return true, true
		case "false", "False", "0":
			return false, true
		}
	}
	return false, false
}

func asFloat(v interface{}) (float64, bool) {
	if v == nil {
		return 0, false
	}
	var number float64
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		number = rv.Float()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		number = float64(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		number = float64(rv.Uint())
	case reflect.String:
		var err error
		number, err = strconv.ParseFloat(rv.String(), 64)
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

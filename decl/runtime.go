package decl

import (
	"fmt"

	"silk/core"
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

	for _, p := range n.Props {
		applyProp(obj, p)
	}

	if n.ID != "" {
		idx[n.ID] = obj
	}

	for _, c := range n.Children {
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
		attachChild(obj, child)
	}

	return obj, nil
}

// applyProp handles the small fixed set of well-known prop names that
// every Silk widget honours via duck-typed interfaces. Unknown prop
// names are intentionally silent — the design echoes formloader.go's
// "best-effort apply" semantics so older designs with attribs the
// runtime no longer cares about still load cleanly.
//
// Non-Lit values (Ref / Bind / Expr) need higher-level wiring (event
// dispatch, reactive subscription, code-eval) and are skipped here;
// the consumer of decl is expected to walk the AST first, install
// handlers, then call Build.
func applyProp(obj interface{}, p Prop) {
	lit, ok := p.Value.(Lit)
	if !ok {
		return
	}
	switch p.Name {
	case "text", "Text":
		if s, ok := lit.V.(string); ok {
			if tw, ok := obj.(textSetter); ok {
				tw.SetText(s)
			}
		}
	case "title", "Title":
		if s, ok := lit.V.(string); ok {
			if tw, ok := obj.(titleSetter); ok {
				tw.SetTitle(s)
			}
		}
	case "checked", "Checked":
		if b, ok := asBool(lit.V); ok {
			if cw, ok := obj.(checkedSetter); ok {
				cw.SetChecked(b)
			}
		}
	case "value", "Value":
		if f, ok := asFloat(lit.V); ok {
			if vw, ok := obj.(valueSetter); ok {
				vw.SetValue(f)
			}
		}
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
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		var f float64
		if _, err := core.PersistSscan(x, &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

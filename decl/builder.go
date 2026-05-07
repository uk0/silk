package decl

// Attr is anything that mutates a Node during construction.
//
// We use this functional-options pattern instead of a typed struct of
// optional fields so the builder reads naturally (one Attr per English
// concept) and so users can extend with their own attrs without forking
// the package. The pattern also lets the type-specific helpers (Button,
// VBox, etc.) compose: a single helper just calls New with a cascade of
// attrs.
type Attr interface {
	apply(*Node)
}

type attrFunc func(*Node)

func (f attrFunc) apply(n *Node) { f(n) }

// ID sets the designer index key on the node — the name the designer
// uses to locate it in the property panel and the form designer.
func ID(id string) Attr {
	return attrFunc(func(n *Node) { n.ID = id })
}

// P sets a property by name. value is wrapped in a Value automatically:
//
//   - already-a-Value: passed through (use Ref(...), Bind(...), Expr(...))
//   - Go primitive: wrapped in Lit
//   - nil: stored as Lit{nil}
//
// Passing the same name twice is allowed; the later P wins.
func P(name string, value interface{}) Attr {
	return attrFunc(func(n *Node) {
		n.SetProp(name, toValue(value))
	})
}

// Child appends a single child node.
func Child(c *Node) Attr {
	return attrFunc(func(n *Node) {
		if c != nil {
			n.Children = append(n.Children, c)
		}
	})
}

// Children appends multiple child nodes. Convenience wrapper around
// repeated Child(...) calls.
func Children(cs ...*Node) Attr {
	return attrFunc(func(n *Node) {
		for _, c := range cs {
			if c != nil {
				n.Children = append(n.Children, c)
			}
		}
	})
}

// New constructs a Node of an arbitrary factory type. This is the
// foundation of every type-specific helper below: each one is just
// New(typeName, ...).
//
// Use New directly when:
//
//   - The widget type is not in this package's pre-baked helper set.
//   - You're authoring a generator that emits AST programmatically and
//     wants to stay agnostic about widget identity.
func New(typeName string, attrs ...Attr) *Node {
	n := &Node{Type: typeName}
	for _, a := range attrs {
		if a != nil {
			a.apply(n)
		}
	}
	return n
}

// toValue normalises arbitrary Go values into the Value interface so
// authors can write `P("text", "hello")` instead of `P("text", Lit{"hello"})`.
// Pre-existing Value implementations pass through unchanged so authors
// can opt into Ref / Bind / Expr without escaping the helper.
func toValue(v interface{}) Value {
	switch x := v.(type) {
	case Value:
		return x
	case nil:
		return Lit{V: nil}
	default:
		return Lit{V: x}
	}
}

// --- Pre-baked helpers for the common widget set. -------------------
//
// These are deliberately one-line wrappers around New. The package
// documents the dominant widget types so casual authors don't have to
// remember exact factory names. Adding more is a one-line patch — every
// widget already lives in core's factory registry, so we just need a
// wrapper for ergonomics.

// Form constructs a top-level form ("gui.Form").
func Form(attrs ...Attr) *Node { return New("gui.Form", attrs...) }

// VBox is a vertical-layout container.
func VBox(attrs ...Attr) *Node { return New("gui.VBox", attrs...) }

// HBox is a horizontal-layout container.
func HBox(attrs ...Attr) *Node { return New("gui.HBox", attrs...) }

// GridLayout is a 2-D row/column container.
func GridLayout(attrs ...Attr) *Node { return New("gui.GridLayout", attrs...) }

// Button is a clickable button. Common props: text, click.
func Button(attrs ...Attr) *Node { return New("gui.Button", attrs...) }

// Label is a static text label. Common prop: text.
func Label(attrs ...Attr) *Node { return New("gui.Label", attrs...) }

// Edit is a single-line text input. Common props: text, placeholder.
func Edit(attrs ...Attr) *Node { return New("gui.Edit", attrs...) }

// CheckBox is a binary toggle. Common props: text, checked.
func CheckBox(attrs ...Attr) *Node { return New("gui.CheckBox", attrs...) }

// RadioButton is a mutually-exclusive choice. Common props: text, checked.
func RadioButton(attrs ...Attr) *Node { return New("gui.RadioButton", attrs...) }

// ComboBox is a dropdown selector. Common props: items, selectedIndex.
func ComboBox(attrs ...Attr) *Node { return New("gui.ComboBox", attrs...) }

// Slider is a continuous-value slider. Common props: value, min, max.
func Slider(attrs ...Attr) *Node { return New("gui.Slider", attrs...) }

// ProgressBar is a determinate progress indicator. Common props: value, min, max.
func ProgressBar(attrs ...Attr) *Node { return New("gui.ProgressBar", attrs...) }

// GroupBox is a labelled rectangular grouping container.
func GroupBox(attrs ...Attr) *Node { return New("gui.GroupBox", attrs...) }

// TabWidget is a multi-page tabbed container.
func TabWidget(attrs ...Attr) *Node { return New("gui.TabWidget", attrs...) }

// Card is a styled container with elevated background.
func Card(attrs ...Attr) *Node { return New("gui.Card", attrs...) }

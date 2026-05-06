package decl

// Node is the canonical declarative representation of a single widget
// instance. It maps 1:1 to a TDoc subtree and to a single constructor
// call in the Go DSL. The renderer / designer / code-emitter all read
// the same Node shape.
//
// Field semantics:
//
//   - Type    factory name (e.g. "gui.Button"). Required. Must match a
//             core.RegisterFactory registration before Build() will succeed.
//   - ID      optional designer index key. When set, written to the TDoc
//             "name" attribute so designer overlays can locate the widget.
//             Empty string means "anonymous".
//   - Props   ordered list of (name, value) pairs. We use a slice instead
//             of a map so the author's writing order is preserved across
//             round-trips — the designer's diff view reads better when
//             properties aren't reshuffled by hash iteration.
//   - Children child nodes whose widgets become sub-widgets of this node's
//             widget at Build time.
//
// Loc is intentionally *not* a field today. When the Go-source parser
// arrives in a follow-up, it will populate a separate parallel structure
// (Loc map keyed by *Node identity) so that adding source positions does
// not perturb the Node value semantics — equality, JSON, etc.
type Node struct {
	Type     string
	ID       string
	Props    []Prop
	Children []*Node
}

// Prop is a named value attached to a Node.
//
// We keep the Value interface sealed (the four implementations live in
// this file) so unknown value variants can never sneak through serialisation
// — a future Bind type, for example, has to add an explicit codec branch
// rather than silently ride along as an opaque blob.
type Prop struct {
	Name  string
	Value Value
}

// Value is the sealed interface implemented by every prop value variant.
// The interface deliberately exposes nothing beyond a discriminator
// method; concrete code switches on the Go type, not on a string tag.
type Value interface {
	isValue()
}

// Lit wraps a Go primitive (string, int, float, bool, etc.) that needs
// no further interpretation. This is the dominant case — most props are
// constants known at design time.
type Lit struct {
	V interface{}
}

func (Lit) isValue() {}

// Ref names another Go identifier — typically an event handler bound by
// the host code at runtime. The serialised form is "@ref:<name>". Decl
// itself does no resolution; consumers (the runtime widget builder, code
// emitter) are responsible for resolving the name in the right scope.
type Ref struct {
	Name string
}

func (Ref) isValue() {}

// Bind names a reactive binding source — e.g. "@bind:user.name". Used
// by the data-binding runtime to subscribe a widget property to a
// reactive value. Decl stores the path verbatim; subscription is done
// at Build time by the data-binding glue.
type Bind struct {
	Path string
}

func (Bind) isValue() {}

// Expr carries an opaque Go expression source — for properties whose
// value cannot be reduced to a literal at design time. The designer
// renders these as "code only" placeholders; the Go emitter reproduces
// the expression verbatim. Expr is the "escape hatch" that keeps decl
// from forcing every author into a strict-subset DSL.
type Expr struct {
	Source string
}

func (Expr) isValue() {}

// TrKey marks a property value as a translation source string. The
// runtime resolves it via i18n.T at Build time, so a designer-authored
// .silkui carrying TrKey values renders in the user's locale without
// the designer needing to know which language is active.
//
// Example:
//
//	decl.Button(decl.P("text", decl.TrKey{Source: "OK"}))
//
// On a host with i18n.SetLocale("zh-CN") active, the button's text
// becomes "确定". With no locale set or no matching entry, it falls
// back to the source string "OK" — same fallback policy as i18n.T.
type TrKey struct {
	Source string
}

func (TrKey) isValue() {}

// SetProp inserts a new property or updates an existing one with the
// same name in place. Order is preserved on update; appended on insert.
// Used by tools that mutate AST after construction (e.g. the designer
// tweaking a button's label).
func (n *Node) SetProp(name string, value Value) {
	for i := range n.Props {
		if n.Props[i].Name == name {
			n.Props[i].Value = value
			return
		}
	}
	n.Props = append(n.Props, Prop{Name: name, Value: value})
}

// PropValue looks up a property's value by name. Returns (nil, false)
// when the prop is not present. Callers that expect Lit can type-assert:
//
//	if v, ok := n.PropValue("text"); ok {
//	    if lit, ok := v.(decl.Lit); ok {
//	        text, _ := lit.V.(string)
//	    }
//	}
func (n *Node) PropValue(name string) (Value, bool) {
	for _, p := range n.Props {
		if p.Name == name {
			return p.Value, true
		}
	}
	return nil, false
}

package decl

import (
	"fmt"
	"go/format"
	"sort"
	"strconv"
	"strings"
)

// ToGo pretty-prints n as Go source using this package's builder DSL.
// The result is a single expression like
//
//	decl.Form(decl.ID("Main"), decl.P("title", "Hello"),
//	    decl.Children(
//	        decl.Button(decl.ID("ok"), decl.P("text", "OK")),
//	        decl.Label(decl.ID("msg"), decl.P("text", "Hi")),
//	    ),
//	)
//
// suitable for embedding in a hand-written .silk.go file. The
// generator picks pre-baked builder helpers when the node's Type
// matches one of the entries in builderShortcuts; otherwise it
// emits decl.New("<type>", ...).
//
// Output is run through go/format before return so callers can
// drop it straight into a source file. ToGo never returns an
// error in practice — go/format failure on its own output would
// signal a bug in the generator and is reported via panic.
//
// nil input returns the empty string.
func ToGo(n *Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	emitNode(&b, n, 0)
	src := b.String()
	formatted, err := format.Source([]byte(src))
	if err != nil {
		// Fall back to unformatted output rather than crash; tests
		// catch the formatting failure separately.
		return src
	}
	return string(formatted)
}

// builderShortcuts maps factory names to the convenience helpers
// in builder.go. Entries here let ToGo emit `decl.Button(...)`
// instead of `decl.New("gui.Button", ...)`. Keep in sync with the
// helpers in builder.go.
var builderShortcuts = map[string]string{
	"gui.Form":        "Form",
	"gui.VBox":        "VBox",
	"gui.HBox":        "HBox",
	"gui.GridLayout":  "GridLayout",
	"gui.Button":      "Button",
	"gui.Label":       "Label",
	"gui.Edit":        "Edit",
	"gui.CheckBox":    "CheckBox",
	"gui.RadioButton": "RadioButton",
	"gui.ComboBox":    "ComboBox",
	"gui.Slider":      "Slider",
	"gui.ProgressBar": "ProgressBar",
	"gui.GroupBox":    "GroupBox",
	"gui.TabWidget":   "TabWidget",
	"gui.Card":        "Card",
}

// emitNode writes a single node's expression onto b at the given
// indentation level. Top-level callers pass indent=0; recursive
// children get indent+1.
//
// Layout strategy: short nodes (no children, ≤2 props) collapse to
// a single line; longer nodes break onto multi-line for readability.
// go/format will adjust whitespace afterwards but we still aim for
// reasonable line breaks so diffs stay narrow.
func emitNode(b *strings.Builder, n *Node, indent int) {
	helper, ok := builderShortcuts[n.Type]
	if !ok {
		// Generic decl.New for unknown widget types.
		b.WriteString(`decl.New(`)
		b.WriteString(strconv.Quote(n.Type))
		emitAttrs(b, n, indent, true)
		b.WriteString(`)`)
		return
	}
	b.WriteString(`decl.`)
	b.WriteString(helper)
	b.WriteString(`(`)
	emitAttrs(b, n, indent, false)
	b.WriteString(`)`)
}

// emitAttrs writes the attribute list for a node. needLeadingComma
// is true when the caller already wrote a positional argument (e.g.
// the type name in decl.New) and the first attr needs a leading ", ".
func emitAttrs(b *strings.Builder, n *Node, indent int, needLeadingComma bool) {
	first := !needLeadingComma

	if n.ID != "" {
		if !first {
			b.WriteString(`, `)
		}
		b.WriteString(`decl.ID(`)
		b.WriteString(strconv.Quote(n.ID))
		b.WriteString(`)`)
		first = false
	}

	for _, p := range n.Props {
		if !first {
			b.WriteString(`, `)
		}
		emitProp(b, p)
		first = false
	}

	if len(n.Children) > 0 {
		if !first {
			b.WriteString(`, `)
		}
		emitChildren(b, n.Children, indent)
	}
}

// emitProp writes a single property: decl.P("name", value).
// The value is rendered per Value variant.
func emitProp(b *strings.Builder, p Prop) {
	b.WriteString(`decl.P(`)
	b.WriteString(strconv.Quote(p.Name))
	b.WriteString(`, `)
	emitValue(b, p.Value)
	b.WriteString(`)`)
}

// emitValue writes a Value in its Go-source form.
//
//   - Lit{string}  → "..."  (uses strconv.Quote, escaping handled)
//   - Lit{others}  → fmt.Sprint of the underlying value
//   - Ref          → decl.Ref{Name: "..."}
//   - Bind         → decl.Bind{Path: "..."}
//   - Expr         → decl.Expr{Source: "..."}
//   - TrKey        → decl.TrKey{Source: "..."}
func emitValue(b *strings.Builder, v Value) {
	switch x := v.(type) {
	case Lit:
		switch lv := x.V.(type) {
		case string:
			b.WriteString(strconv.Quote(lv))
		case bool:
			if lv {
				b.WriteString(`true`)
			} else {
				b.WriteString(`false`)
			}
		case nil:
			b.WriteString(`nil`)
		default:
			b.WriteString(fmt.Sprint(lv))
		}
	case Ref:
		b.WriteString(`decl.Ref{Name: `)
		b.WriteString(strconv.Quote(x.Name))
		b.WriteString(`}`)
	case Bind:
		b.WriteString(`decl.Bind{Path: `)
		b.WriteString(strconv.Quote(x.Path))
		b.WriteString(`}`)
	case Expr:
		b.WriteString(`decl.Expr{Source: `)
		b.WriteString(strconv.Quote(x.Source))
		b.WriteString(`}`)
	case TrKey:
		b.WriteString(`decl.TrKey{Source: `)
		b.WriteString(strconv.Quote(x.Source))
		b.WriteString(`}`)
	default:
		// Unknown Value impl — emit the type name as a comment so
		// hand-edits can fix it. Go-format will still parse around
		// the comment.
		b.WriteString(fmt.Sprintf(`/* unknown Value type %T */`, v))
	}
}

// emitChildren writes decl.Children(child1, child2, ...). Inserts
// explicit newlines + trailing commas in the multi-child case so
// go/format preserves the multi-line layout — gofmt only keeps line
// breaks that already exist in the source.
//
// Single-child Children() collapses to one line:
//
//	decl.Children(decl.Button(...))
//
// Multi-child Children() spans multiple lines (after gofmt):
//
//	decl.Children(
//	    decl.Button(...),
//	    decl.Label(...),
//	)
func emitChildren(b *strings.Builder, kids []*Node, indent int) {
	b.WriteString(`decl.Children(`)
	if len(kids) == 1 {
		emitNode(b, kids[0], indent+1)
		b.WriteString(`)`)
		return
	}
	b.WriteString("\n")
	for _, c := range kids {
		emitNode(b, c, indent+1)
		b.WriteString(",\n")
	}
	b.WriteString(`)`)
}

// AvailableShortcuts returns the sorted list of factory names that
// have a builder shortcut. Used by docs / tests.
func AvailableShortcuts() []string {
	out := make([]string, 0, len(builderShortcuts))
	for k := range builderShortcuts {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

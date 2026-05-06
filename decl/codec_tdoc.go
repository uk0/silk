package decl

import (
	"errors"
	"strings"

	"silk/core"
)

// TDoc layout produced/consumed by this codec
//
//	root: val=<factory name>          // e.g. "gui.Form"
//	  "name": val=<id string>         // optional, only when Node.ID != ""
//	  <prop name>: val=<encoded value>
//	  ...
//	  "widget": <recursive subtree>
//	  ...
//
// The "widget" key for children matches the modern dialect of formloader.go,
// so a Node serialised by this codec is loadable by gui.LoadFormFromDoc
// without any glue. The "name" key matches what the designer writes for
// the widget identifier.
//
// Value encoding (string scope): the four Value variants ride on top of
// the TDoc's persisted-string format. Lit primitives go through TDoc's
// own typed accessors (Value/SetValue use the persist layer). Ref / Bind /
// Expr encode as tag-prefixed strings so a single TDoc string can carry
// any of them:
//
//	"@ref:OnClick"      → Ref{Name: "OnClick"}
//	"@bind:user.name"   → Bind{Path: "user.name"}
//	"@expr:foo()*2"     → Expr{Source: "foo()*2"}
//
// A literal string that happens to start with "@ref:" is theoretically
// ambiguous; we accept that — designs that want a literal "@ref:..." can
// double-prefix with "@lit:".

// Tag prefixes used to distinguish non-literal value variants in the
// serialised string scope.
const (
	tagRef  = "@ref:"
	tagBind = "@bind:"
	tagExpr = "@expr:"
	tagLit  = "@lit:" // explicit literal escape for strings starting with "@"
)

// ToTDoc serialises n into a *core.TDoc rooted at the returned doc. The
// returned doc has no parent and can be directly fed to TDocFile.WriteTo
// or further nested under a parent designer document.
//
// nil n returns nil. Internal nil children are skipped silently — the
// builder allows passing nil into Children(...), and we don't want every
// emitter to defensively filter.
func ToTDoc(n *Node) *core.TDoc {
	if n == nil {
		return nil
	}
	doc := core.NewTDoc()
	_ = doc.SetValue(n.Type)

	if n.ID != "" {
		_ = doc.WriteAttr("name", n.ID)
	}

	for _, p := range n.Props {
		writeProp(doc, p.Name, p.Value)
	}

	for _, c := range n.Children {
		if c == nil {
			continue
		}
		sub := ToTDoc(c)
		if sub == nil {
			continue
		}
		sub.SetKey("widget")
		doc.AddChild(sub)
	}

	return doc
}

// FromTDoc parses a TDoc subtree (as produced by ToTDoc, or as written
// by the designer's modern dialect) back into a Node. Any sub-node not
// matching the well-known "name" / "widget" keys is read as a property —
// this is the rule formloader uses, kept symmetric so designer-authored
// docs round-trip through decl without information loss.
//
// Returns an error when doc is nil or the factory name (TDoc value) is
// empty. Unknown attribute keys are kept; an unknown widget type is NOT
// rejected here because Build() is the right place to fail noisily —
// we want decl to faithfully model whatever the designer wrote.
func FromTDoc(doc *core.TDoc) (*Node, error) {
	if doc == nil {
		return nil, errors.New("decl.FromTDoc: nil doc")
	}
	var typeName string
	_ = doc.Value(&typeName)
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return nil, errors.New("decl.FromTDoc: empty factory name (TDoc value)")
	}

	n := &Node{Type: typeName}

	for _, sub := range doc.Childdren() {
		key := sub.Key()
		switch key {
		case "":
			// Unkeyed sub-nodes — legacy designer dialect put children here.
			// We accept them as nested widgets to keep the loader symmetric
			// with formloader.LoadFormFromDoc's "children" branch.
			if sub.HasValue() || sub.HasChildren() {
				child, err := FromTDoc(sub)
				if err == nil {
					n.Children = append(n.Children, child)
				}
			}
		case "name":
			var s string
			_ = sub.Value(&s)
			n.ID = s
		case "widget":
			child, err := FromTDoc(sub)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, child)
		default:
			// Property: read its serialised string and decode the variant.
			var s string
			_ = sub.Value(&s)
			n.Props = append(n.Props, Prop{Name: key, Value: decodeValue(s)})
		}
	}

	return n, nil
}

// writeProp dispatches on the Value variant to choose the right TDoc
// write path. Lit values pass through WriteAttr's native typed handling
// — the persist layer formats numbers, bools, and strings (with
// quoting) without our intervention. Ref/Bind/Expr round-trip via a
// tag-prefixed string so the variant is recoverable on read.
//
// The earlier "stringify then write" approach double-encoded strings:
// PersistString quoted "OK" to `"OK"`, the resulting Go string was
// then written through SetValue which called PersistString again,
// producing `"\"OK\""` on disk. Splitting the two paths fixes that.
func writeProp(doc *core.TDoc, name string, v Value) {
	switch x := v.(type) {
	case Lit:
		// String literals that start with "@" need the explicit @lit:
		// escape so they don't parse back as Ref/Bind/Expr after a
		// round-trip. Numeric and bool primitives never collide with
		// the tag prefixes.
		if s, ok := x.V.(string); ok && strings.HasPrefix(s, "@") {
			_ = doc.WriteAttr(name, tagLit+s)
			return
		}
		_ = doc.WriteAttr(name, x.V)
	case Ref:
		_ = doc.WriteAttr(name, tagRef+x.Name)
	case Bind:
		_ = doc.WriteAttr(name, tagBind+x.Path)
	case Expr:
		_ = doc.WriteAttr(name, tagExpr+x.Source)
	}
}

// decodeValue is the inverse of writeProp. The four tag prefixes route
// directly to Ref/Bind/Expr/Lit-escape; everything else is wrapped as
// a Lit holding the deserialised Go string. Type coercion (string →
// bool / float / int) is the runtime's responsibility — applyProp's
// asBool / asFloat already handle this for the dominant prop names so
// the AST stays a faithful textual mirror of what was written.
//
// Note: the input s is assumed to have already been unquoted by
// TDoc.Value(&s), which uses PersistSscan's quotedString scanner. So
// "\"OK\"" on disk arrives here as the Go string OK; tag dispatch works
// against the user-visible value, not the on-disk encoding.
func decodeValue(s string) Value {
	switch {
	case strings.HasPrefix(s, tagRef):
		return Ref{Name: s[len(tagRef):]}
	case strings.HasPrefix(s, tagBind):
		return Bind{Path: s[len(tagBind):]}
	case strings.HasPrefix(s, tagExpr):
		return Expr{Source: s[len(tagExpr):]}
	case strings.HasPrefix(s, tagLit):
		return Lit{V: s[len(tagLit):]}
	default:
		return Lit{V: s}
	}
}

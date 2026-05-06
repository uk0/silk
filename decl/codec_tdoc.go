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
		s := encodeValue(p.Value)
		_ = doc.WriteAttr(p.Name, s)
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

// encodeValue produces the string written into the TDoc value slot for
// a single property. Non-literal variants are tag-prefixed so they are
// recoverable.
//
// For Lit, we let core.PersistString format the underlying primitive
// (numbers, bools etc.). PersistString quotes strings, so re-loading
// goes through the inverse path in decodeValue.
func encodeValue(v Value) string {
	switch x := v.(type) {
	case Lit:
		s, err := core.PersistString(x.V)
		if err != nil {
			return ""
		}
		// A literal string that begins with "@" needs an explicit escape so
		// it doesn't parse back as a Ref/Bind/Expr. PersistString quotes
		// strings, so the leading rune of s for a string literal is "
		// (not @). Numeric / bool literals never collide. Belt-and-braces
		// guard regardless: if the *unquoted* primitive starts with "@" we
		// prepend the literal escape.
		if s, ok := x.V.(string); ok && strings.HasPrefix(s, "@") {
			return tagLit + s
		}
		return s
	case Ref:
		return tagRef + x.Name
	case Bind:
		return tagBind + x.Path
	case Expr:
		return tagExpr + x.Source
	default:
		return ""
	}
}

// decodeValue parses the inverse of encodeValue. The four prefixes route
// to their typed Value; everything else goes through PersistSscan as a
// generic string and surfaces as a Lit.
func decodeValue(s string) Value {
	switch {
	case strings.HasPrefix(s, tagRef):
		return Ref{Name: s[len(tagRef):]}
	case strings.HasPrefix(s, tagBind):
		return Bind{Path: s[len(tagBind):]}
	case strings.HasPrefix(s, tagExpr):
		return Expr{Source: s[len(tagExpr):]}
	case strings.HasPrefix(s, tagLit):
		// Explicit literal escape for strings beginning with "@".
		return Lit{V: s[len(tagLit):]}
	default:
		// Decode through the persist layer — numbers, bools, quoted
		// strings all come back as their natural Go type. Falls back to
		// the raw string when PersistSscan can't pick a concrete type.
		var raw string
		_, err := core.PersistSscan(s, &raw)
		if err != nil {
			return Lit{V: s}
		}
		return Lit{V: raw}
	}
}

package decl

import (
	"reflect"
	"testing"
)

// Test1_BuilderShapes pins the basic constructor invariants. Each
// helper must produce a Node whose Type matches the factory name
// the designer + formloader expect; an attribute applied via P
// must round-trip to PropValue without mutation.
func Test1_BuilderShapes(t *testing.T) {
	n := Button(ID("ok"), P("text", "OK"), P("click", Ref{Name: "OnOk"}))
	if n.Type != "gui.Button" {
		t.Errorf("Button type = %q, want gui.Button", n.Type)
	}
	if n.ID != "ok" {
		t.Errorf("ID = %q, want ok", n.ID)
	}
	if got := len(n.Props); got != 2 {
		t.Fatalf("len(Props) = %d, want 2", got)
	}
	v, ok := n.PropValue("text")
	if !ok {
		t.Fatalf("text prop missing")
	}
	if lit, ok := v.(Lit); !ok || lit.V != "OK" {
		t.Errorf("text prop = %#v, want Lit{OK}", v)
	}
	v, ok = n.PropValue("click")
	if !ok {
		t.Fatalf("click prop missing")
	}
	if r, ok := v.(Ref); !ok || r.Name != "OnOk" {
		t.Errorf("click prop = %#v, want Ref{OnOk}", v)
	}
}

// Test2_PSetPropOverwritesInPlace verifies that setting a prop a
// second time updates the existing entry rather than appending a
// duplicate. Order preservation matters for the future Go-source
// emitter (authors expect their writing order to survive).
func Test2_PSetPropOverwritesInPlace(t *testing.T) {
	n := Label(P("text", "Hi"))
	n.SetProp("text", Lit{V: "Hello"})
	n.SetProp("color", Lit{V: "red"})

	if got := len(n.Props); got != 2 {
		t.Fatalf("len(Props) = %d, want 2", got)
	}
	if n.Props[0].Name != "text" {
		t.Errorf("Props[0].Name = %q, want text", n.Props[0].Name)
	}
	if lit, ok := n.Props[0].Value.(Lit); !ok || lit.V != "Hello" {
		t.Errorf("text prop = %#v, want Lit{Hello}", n.Props[0].Value)
	}
}

// Test3_TDocRoundTripStringProp puts a string prop through ToTDoc and
// FromTDoc and confirms the value is preserved. Strings are the most
// common prop type and the canonical Persist target.
func Test3_TDocRoundTripStringProp(t *testing.T) {
	orig := Button(ID("ok"), P("text", "OK"))
	doc := ToTDoc(orig)
	if doc == nil {
		t.Fatalf("ToTDoc returned nil")
	}
	got, err := FromTDoc(doc)
	if err != nil {
		t.Fatalf("FromTDoc: %v", err)
	}

	if got.Type != orig.Type || got.ID != orig.ID {
		t.Errorf("Type/ID drift: got=%+v orig=%+v", got, orig)
	}
	if got.Props[0].Name != "text" {
		t.Errorf("prop name drift: got %q", got.Props[0].Name)
	}
	if lit, ok := got.Props[0].Value.(Lit); !ok || lit.V != "OK" {
		t.Errorf("prop value drift: got %#v", got.Props[0].Value)
	}
}

// Test4_TDocRoundTripValueVariants covers the three non-Lit value
// variants. Each must come back as the same concrete type carrying
// the same payload — silently downgrading to Lit is a regression.
func Test4_TDocRoundTripValueVariants(t *testing.T) {
	n := Button(
		P("ref", Ref{Name: "OnClick"}),
		P("bind", Bind{Path: "user.name"}),
		P("expr", Expr{Source: "f(x) * 2"}),
	)
	doc := ToTDoc(n)
	got, err := FromTDoc(doc)
	if err != nil {
		t.Fatalf("FromTDoc: %v", err)
	}

	if r, ok := got.Props[0].Value.(Ref); !ok || r.Name != "OnClick" {
		t.Errorf("Ref round-trip: got %#v", got.Props[0].Value)
	}
	if b, ok := got.Props[1].Value.(Bind); !ok || b.Path != "user.name" {
		t.Errorf("Bind round-trip: got %#v", got.Props[1].Value)
	}
	if e, ok := got.Props[2].Value.(Expr); !ok || e.Source != "f(x) * 2" {
		t.Errorf("Expr round-trip: got %#v", got.Props[2].Value)
	}
}

// Test5_TDocRoundTripChildren builds a small tree (Form > VBox >
// [Button, Label]) and confirms the post-roundtrip AST is structurally
// identical (Type, ID, Props, Children len).
func Test5_TDocRoundTripChildren(t *testing.T) {
	orig := Form(
		ID("MainWindow"),
		P("title", "Hello"),
		Child(VBox(ID("root"),
			Children(
				Button(ID("ok"), P("text", "OK")),
				Label(ID("greeting"), P("text", "Hello, world!")),
			),
		)),
	)

	doc := ToTDoc(orig)
	got, err := FromTDoc(doc)
	if err != nil {
		t.Fatalf("FromTDoc: %v", err)
	}

	// Top-level shape.
	if got.Type != "gui.Form" || got.ID != "MainWindow" {
		t.Errorf("root drift: %+v", got)
	}
	if len(got.Children) != 1 {
		t.Fatalf("root children = %d, want 1", len(got.Children))
	}
	vbox := got.Children[0]
	if vbox.Type != "gui.VBox" || vbox.ID != "root" {
		t.Errorf("vbox drift: %+v", vbox)
	}
	if len(vbox.Children) != 2 {
		t.Fatalf("vbox children = %d, want 2", len(vbox.Children))
	}
	if vbox.Children[0].Type != "gui.Button" || vbox.Children[0].ID != "ok" {
		t.Errorf("button child drift: %+v", vbox.Children[0])
	}
	if vbox.Children[1].Type != "gui.Label" || vbox.Children[1].ID != "greeting" {
		t.Errorf("label child drift: %+v", vbox.Children[1])
	}
}

// Test6_TDocRoundTripEscapesAtPrefix protects the @-escape path: a
// literal string starting with "@" must NOT be mistaken for a Ref/
// Bind/Expr after a round-trip. The encoder prefixes such strings
// with @lit:.
func Test6_TDocRoundTripEscapesAtPrefix(t *testing.T) {
	n := Label(P("text", "@notARef"))
	doc := ToTDoc(n)
	got, _ := FromTDoc(doc)
	if lit, ok := got.Props[0].Value.(Lit); !ok || lit.V != "@notARef" {
		t.Errorf("@-prefixed literal lost: got %#v", got.Props[0].Value)
	}
}

// Test7_FromTDocRejectsEmptyType is the negative path: a TDoc with
// no value (no factory name) is unusable and FromTDoc must surface
// an explicit error rather than producing a Node{Type: ""} that
// later fails mysteriously inside Build().
func Test7_FromTDocRejectsEmptyType(t *testing.T) {
	if _, err := FromTDoc(nil); err == nil {
		t.Errorf("FromTDoc(nil) should error")
	}
}

// Test8_ToTDocAcceptsNil keeps the codec friendly for partial trees:
// passing nil does not panic; it returns nil so callers can chain.
func Test8_ToTDocAcceptsNil(t *testing.T) {
	if got := ToTDoc(nil); got != nil {
		t.Errorf("ToTDoc(nil) = %v, want nil", got)
	}
}

// Test9_NewArbitraryType lets users instantiate widgets we don't have
// pre-baked helpers for. The Type field is whatever was passed in.
func Test9_NewArbitraryType(t *testing.T) {
	n := New("gui.Custom", ID("c1"), P("foo", 42))
	if n.Type != "gui.Custom" || n.ID != "c1" {
		t.Errorf("New drift: %+v", n)
	}
	if v, ok := n.PropValue("foo"); !ok {
		t.Errorf("foo prop missing")
	} else if lit, ok := v.(Lit); !ok || !reflect.DeepEqual(lit.V, 42) {
		t.Errorf("foo prop = %#v, want Lit{42}", v)
	}
}

// Test10_BuildUnknownTypeErrors covers the runtime negative path: a
// factory name that was never registered must surface as an error.
// We intentionally do NOT register a fake factory in this test; the
// gui-side widget tests that exercise real factories live in
// decl_runtime_test.go where the gui package is imported.
func Test10_BuildUnknownTypeErrors(t *testing.T) {
	n := New("decl.NeverRegistered")
	if _, err := n.Build(); err == nil {
		t.Errorf("Build() of unknown type should error")
	}
}

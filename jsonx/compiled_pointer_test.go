package jsonx

import (
	"errors"
	"testing"
)

func TestCompiledPointerPreservesPublicNodeAndCanBeReused(t *testing.T) {
	ptr := "/a~1b/~0温度/0//v"
	p, err := CompilePointer(ptr)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"hot", "cold"} {
		d, err := Parse([]byte("{\"a/b\":{\"~温度\":[{\"\":{\"v\":\"" + value + "\"}}]}}"))
		if err != nil {
			t.Fatal(err)
		}
		n, err := p.Lookup(d)
		if err != nil || n.Key() != "v" || n.Pointer() != ptr || n.Value() != value {
			t.Fatalf("lookup: node=%+v value=%v err=%v", n, n.Value(), err)
		}
		root, err := (CompiledPointer{}).Lookup(d)
		if err != nil || root.Key() != "$" || root.Pointer() != "" || root.Kind() != Object {
			t.Fatalf("zero-value pointer is not the root: %+v %v", root, err)
		}
		if allocs := testing.AllocsPerRun(200, func() {
			result, err := p.Lookup(d)
			if err != nil || !result.Valid() {
				panic("unexpected compiled lookup failure")
			}
		}); allocs != 0 {
			t.Fatalf("compiled lookup allocated %v times", allocs)
		}
	}
}

func TestCompiledPointerValidationSeparatesSyntaxAndDocumentShape(t *testing.T) {
	if _, err := CompilePointer("/invalid~2"); !errors.Is(err, ErrPointer) {
		t.Fatalf("invalid escape: %v", err)
	}
	p, err := CompilePointer("/a/01")
	if err != nil {
		t.Fatalf("01 is a valid object key: %v", err)
	}
	arrayDoc, err := Parse([]byte("{\"a\":[1,2]}"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Lookup(arrayDoc); !errors.Is(err, ErrPointer) {
		t.Fatalf("accepted noncanonical array index: %v", err)
	}
	objDoc, err := Parse([]byte("{\"a\":{\"01\":true}}"))
	if err != nil {
		t.Fatal(err)
	}
	if n, err := p.Lookup(objDoc); err != nil || n.Value() != true {
		t.Fatalf("literal object key: %v %v", n.Value(), err)
	}
	if _, err := p.Lookup(nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nil document: %v", err)
	}
}

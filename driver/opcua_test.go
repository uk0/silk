package driver

import (
	"testing"

	"github.com/gopcua/opcua/ua"
)

// gopcua ships no embeddable server, so these tests are socket-free: they cover
// NodeID address parsing, the variant->Go extraction helper and the Driver
// interface assertion. A live read/write round trip against a real OPC-UA
// server is NOT exercised here.

// TestOPCUANodeIDParseValid checks the documented address forms parse to the
// right namespace and identifier via ua.ParseNodeID (the addressing ReadPoint
// and WritePoint rely on).
func TestOPCUANodeIDParseValid(t *testing.T) {
	cases := []struct {
		addr   string
		ns     uint16
		strID  string // expected string identifier, "" if numeric
		intID  uint32 // expected numeric identifier, meaningful when strID == ""
	}{
		{"ns=2;s=Demo.Float", 2, "Demo.Float", 0},
		{"ns=0;s=Foo.Bar", 0, "Foo.Bar", 0},
		{"ns=3;i=1001", 3, "", 1001},
		{"i=2258", 0, "", 2258},
		{"ns=5;i=42", 5, "", 42},
	}
	for _, c := range cases {
		id, err := ua.ParseNodeID(c.addr)
		if err != nil {
			t.Errorf("ParseNodeID(%q): unexpected error %v", c.addr, err)
			continue
		}
		if id.Namespace() != c.ns {
			t.Errorf("ParseNodeID(%q).Namespace() = %d, want %d", c.addr, id.Namespace(), c.ns)
		}
		if c.strID != "" {
			if id.StringID() != c.strID {
				t.Errorf("ParseNodeID(%q).StringID() = %q, want %q", c.addr, id.StringID(), c.strID)
			}
		} else if id.IntID() != c.intID {
			t.Errorf("ParseNodeID(%q).IntID() = %d, want %d", c.addr, id.IntID(), c.intID)
		}
	}
}

// TestOPCUANodeIDParseInvalid checks malformed addresses are rejected, so a bad
// point address surfaces as an error from ReadPoint/WritePoint rather than a
// silent misread.
func TestOPCUANodeIDParseInvalid(t *testing.T) {
	cases := []string{
		"ns=abc;i=1",      // namespace not an integer
		"ns=2;i=notanum",  // numeric id not a number
		"foo;i=1",         // namespace part missing ns=/nsu= prefix
		"ns=99999999;i=1", // namespace id out of range
	}
	for _, addr := range cases {
		if _, err := ua.ParseNodeID(addr); err == nil {
			t.Errorf("ParseNodeID(%q): expected error, got nil", addr)
		}
	}
}

// TestOPCUAVariantValue checks the variant->Go extraction: the concrete server
// type passes through unchanged and a nil variant yields nil.
func TestOPCUAVariantValue(t *testing.T) {
	if got := variantValue(nil); got != nil {
		t.Errorf("variantValue(nil) = %v, want nil", got)
	}
	cases := []interface{}{
		float64(3.14),
		true,
		int32(-7),
		uint32(42),
		"hello",
	}
	for _, want := range cases {
		v := ua.MustVariant(want)
		if got := variantValue(v); got != want {
			t.Errorf("variantValue(MustVariant(%#v)) = %#v, want %#v", want, got, want)
		}
	}
}

// TestOPCUAImplementsDriver asserts *OPCUA satisfies the Driver interface at
// runtime (the compile-time guard lives in opcua.go).
func TestOPCUAImplementsDriver(t *testing.T) {
	var d Driver = NewOPCUA("opc.tcp://127.0.0.1:4840")
	if d == nil {
		t.Fatal("NewOPCUA returned a nil Driver")
	}
}

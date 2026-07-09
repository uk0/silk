package device

import (
	"testing"

	"github.com/uk0/silk/driver"
)

// TestParsePoints covers the CSV point-list parser: field trimming, optional
// order/access with defaults, comments and blank lines, and the bool path.
func TestParsePoints(t *testing.T) {
	csv := "\n# a comment\nlevel, hr:0, Float32, ABCD, RO\nsp, hr:2, Int32, CDAB, RW\nrun, coil:0, Bool\n"
	pts, err := parsePoints(csv)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(pts) != 3 {
		t.Fatalf("got %d points, want 3", len(pts))
	}
	want0 := driver.TagPoint{Tag: "level", Address: "hr:0", Type: driver.TypeFloat32, Order: driver.BigEndian, Access: driver.ReadOnly}
	if pts[0] != want0 {
		t.Errorf("point 0 = %+v, want %+v", pts[0], want0)
	}
	if pts[1].Order != driver.LittleByteSwap {
		t.Errorf("point 1 order = %v, want CDAB", pts[1].Order)
	}
	if pts[1].Access != driver.ReadWrite {
		t.Errorf("point 1 access = %v, want RW", pts[1].Access)
	}
	if pts[2].Type != driver.TypeBool || pts[2].Order != driver.BigEndian || pts[2].Access != driver.ReadOnly {
		t.Errorf("point 2 = %+v, want bool/default order/RO", pts[2])
	}
}

// TestParsePointsErrors verifies malformed lines are rejected.
func TestParsePointsErrors(t *testing.T) {
	for _, bad := range []string{
		"a,b",              // too few fields
		"a,b,NotAType",     // unknown type
		"a,b,Int16,NOPE",   // unknown byte order
	} {
		if _, err := parsePoints(bad); err == nil {
			t.Errorf("parsePoints(%q) = nil error, want error", bad)
		}
	}
}

// TestProtocolRoundTrip checks the string<->enum mapping used by the property.
func TestProtocolRoundTrip(t *testing.T) {
	if parseProtocol("s7") != ProtoS7 || parseProtocol("S7") != ProtoS7 {
		t.Error("s7 should parse to ProtoS7")
	}
	if parseProtocol("modbus") != ProtoModbusTCP || parseProtocol("") != ProtoModbusTCP {
		t.Error("default/modbus should parse to ProtoModbusTCP")
	}
	if ProtoS7.String() != "s7" || ProtoModbusTCP.String() != "modbus" {
		t.Error("protocol String mismatch")
	}
}

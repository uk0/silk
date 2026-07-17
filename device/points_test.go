package device

import (
	"testing"
)

// TestParseFormatRoundTrip parses a multi-line point set, formats it back and
// re-parses, requiring the model to survive the round trip unchanged, and pins
// the canonical FormatPoints output (all five fields, defaults filled in).
func TestParseFormatRoundTrip(t *testing.T) {
	csv := "level, hr:0,   Float32, ABCD, RO\n" +
		"sp,    hr:2,   Int32,   CDAB, RW\n" +
		"run,   coil:0, Bool\n" +
		"count, hr:8,   UInt32,  BADC, RO\n"
	pts, err := ParsePoints(csv)
	if err != nil {
		t.Fatalf("ParsePoints: %v", err)
	}
	if len(pts) != 4 {
		t.Fatalf("got %d points, want 4", len(pts))
	}

	formatted := FormatPoints(pts)
	pts2, err := ParsePoints(formatted)
	if err != nil {
		t.Fatalf("re-ParsePoints(%q): %v", formatted, err)
	}
	if len(pts2) != len(pts) {
		t.Fatalf("round-trip point count %d != %d", len(pts2), len(pts))
	}
	for i := range pts {
		if pts[i] != pts2[i] {
			t.Errorf("round-trip point %d = %+v, want %+v", i, pts2[i], pts[i])
		}
	}

	// FormatPoints must emit every field so the ABCD/RO defaults on the Bool
	// line survive the round trip.
	want := "level,hr:0,Float32,ABCD,RO\n" +
		"sp,hr:2,Int32,CDAB,RW\n" +
		"run,coil:0,Bool,ABCD,RO\n" +
		"count,hr:8,UInt32,BADC,RO\n"
	if formatted != want {
		t.Errorf("FormatPoints =\n%q\nwant\n%q", formatted, want)
	}
}

// TestValidatePointLine checks single-line validation: good lines (including the
// blank and comment cases that carry no point) pass; a missing type field, an
// unknown type and an unknown byte order are each rejected.
func TestValidatePointLine(t *testing.T) {
	good := []string{
		"level,hr:0,Float32,ABCD,RO",
		"run, coil:0, Bool", // optional order/access omitted
		"",                  // blank
		"   ",               // whitespace only
		"# a comment",       // comment
	}
	for _, line := range good {
		if err := ValidatePointLine(line); err != nil {
			t.Errorf("ValidatePointLine(%q) = %v, want nil", line, err)
		}
	}

	bad := []string{
		"level,hr:0",            // missing type
		"level,hr:0,NotAType",   // bad type
		"level,hr:0,Int16,NOPE", // bad byte order
	}
	for _, line := range bad {
		if err := ValidatePointLine(line); err == nil {
			t.Errorf("ValidatePointLine(%q) = nil, want error", line)
		}
	}
}

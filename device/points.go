package device

import (
	"fmt"
	"strings"

	"github.com/uk0/silk/driver"
)

// The tag point list is a small CSV text: one point per line, fields
// "tag,address,type,order,access". order and access are optional and default to
// ABCD (big-endian) and RO (read-only). Blank lines and lines beginning with '#'
// are comments. Whitespace around fields is trimmed. Example:
//
//	level, hr:0,   Float32, ABCD, RO
//	sp,    hr:2,   Int32,   CDAB, RW
//	run,   coil:0, Bool
//
// ParsePoints, FormatPoints and ValidatePointLine are the single source of truth
// for this format; component.go and the designer point editor both go through
// them.

// ParsePoints parses the CSV point list into driver.TagPoint values. It is the
// one parser for the point-list format. Blank and comment lines are skipped; a
// malformed line returns an error naming its 1-based line number.
func ParsePoints(csv string) ([]driver.TagPoint, error) {
	var out []driver.TagPoint
	for i, raw := range strings.Split(csv, "\n") {
		pt, skip, err := parsePointLine(raw)
		if err != nil {
			return nil, fmt.Errorf("device: line %d: %w", i+1, err)
		}
		if skip {
			continue
		}
		out = append(out, pt)
	}
	return out, nil
}

// FormatPoints renders points back to canonical CSV text: one line per point
// with all five fields, each line newline-terminated. It round-trips with
// ParsePoints, so ParsePoints(FormatPoints(pts)) returns pts.
func FormatPoints(pts []driver.TagPoint) string {
	var b strings.Builder
	for _, p := range pts {
		b.WriteString(p.Tag)
		b.WriteByte(',')
		b.WriteString(p.Address)
		b.WriteByte(',')
		b.WriteString(p.Type.String())
		b.WriteByte(',')
		b.WriteString(p.Order.String())
		b.WriteByte(',')
		b.WriteString(p.Access.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// ValidatePointLine reports whether one point line is well-formed, returning the
// same field error ParsePoints would raise for it (without the line-number
// prefix). Blank and comment lines are valid and carry no point.
func ValidatePointLine(line string) error {
	_, _, err := parsePointLine(line)
	return err
}

// parsePointLine parses a single CSV line. skip is true for blank/comment lines,
// which are valid but yield no point.
func parsePointLine(raw string) (pt driver.TagPoint, skip bool, err error) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return driver.TagPoint{}, true, nil
	}
	f := strings.Split(line, ",")
	for j := range f {
		f[j] = strings.TrimSpace(f[j])
	}
	if len(f) < 3 {
		return pt, false, fmt.Errorf("need at least tag,address,type")
	}
	dt, err := parseType(f[2])
	if err != nil {
		return pt, false, err
	}
	order := driver.BigEndian
	if len(f) >= 4 && f[3] != "" {
		if order, err = parseOrder(f[3]); err != nil {
			return pt, false, err
		}
	}
	access := driver.ReadOnly
	if len(f) >= 5 && strings.EqualFold(f[4], "RW") {
		access = driver.ReadWrite
	}
	return driver.TagPoint{Tag: f[0], Address: f[1], Type: dt, Order: order, Access: access}, false, nil
}

func parseType(s string) (driver.DataType, error) {
	switch strings.ToLower(s) {
	case "bool":
		return driver.TypeBool, nil
	case "int16":
		return driver.TypeInt16, nil
	case "uint16":
		return driver.TypeUInt16, nil
	case "int32":
		return driver.TypeInt32, nil
	case "uint32":
		return driver.TypeUInt32, nil
	case "int64":
		return driver.TypeInt64, nil
	case "uint64":
		return driver.TypeUInt64, nil
	case "float32":
		return driver.TypeFloat32, nil
	case "float64":
		return driver.TypeFloat64, nil
	}
	return 0, fmt.Errorf("unknown type %q", s)
}

func parseOrder(s string) (driver.ByteOrder, error) {
	switch strings.ToUpper(s) {
	case "ABCD", "BIG":
		return driver.BigEndian, nil
	case "DCBA", "LITTLE":
		return driver.LittleEndian, nil
	case "BADC":
		return driver.BigByteSwap, nil
	case "CDAB":
		return driver.LittleByteSwap, nil
	}
	return 0, fmt.Errorf("unknown byte order %q", s)
}

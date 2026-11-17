package point_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/uk0/silk/point"
)

func TestExpressionExactIntegerComparisons(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		input        any
		want         bool
	}{
		{"json-adjacent", "x == 9007199254740992", json.Number("9007199254740993"), false},
		{"json-exact", "x == 9007199254740993", json.Number("9007199254740993"), true},
		{"int-adjacent", "x > 9007199254740992", int64(9007199254740993), true},
		{"max", "x == 9223372036854775807", int64(math.MaxInt64), true},
		{"min", "x == -9223372036854775808", int64(math.MinInt64), true},
		{"mixed-above", "x > 9007199254740992.0", int64(9007199254740993), true},
		{"mixed-upper-bound", "x < 9223372036854775808.0", int64(math.MaxInt64), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, err := point.Compile(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			v, err := e.Evaluate(tc.input)
			if err != nil || v != tc.want {
				t.Fatalf("%s(%v): %v, %v; want %v", tc.source, tc.input, v, err, tc.want)
			}
		})
	}
}

func TestExpressionIntegerStatusWordsAndArithmetic(t *testing.T) {
	for _, tc := range []struct {
		source string
		input  any
		want   any
	}{
		{"x+2", int64(9007199254740993), int64(9007199254740995)},
		{"x-1", int64(math.MaxInt64), int64(math.MaxInt64 - 1)},
		{"x*3", int64(3_000_000_000), int64(9_000_000_000)},
		{"(x & 0x8) != 0", int64(9), true},
		{"(x >> 4) & 0b1111", int64(0xa5), int64(10)},
		{"x &^ 0x2", int64(7), int64(5)},
		{"x | 0o10", int64(1), int64(9)},
		{"x ^ 3", int64(7), int64(4)},
		{"^x", int64(0), int64(-1)},
		{"x << 3", int64(2), int64(16)},
		{"x % 10", int64(math.MaxInt64), int64(7)},
		{"x % -1", int64(math.MinInt64), int64(0)},
		{"x/2", int64(5), 2.5},
		{"x*0.1+2", 928.0, 94.80000000000001},
	} {
		t.Run(tc.source, func(t *testing.T) {
			e, err := point.Compile(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			v, err := e.Evaluate(tc.input)
			if err != nil || v != tc.want {
				t.Fatalf("got %v (%T), %v; want %v (%T)", v, v, err, tc.want, tc.want)
			}
		})
	}
}

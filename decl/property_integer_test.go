package decl

import (
	"math"
	"strconv"
	"testing"
)

func TestPropertyIntegerPreservesExactTypedValues(t *testing.T) {
	type namedInt int64
	type namedUint uint64
	type namedString string
	for _, tc := range []struct {
		value Value
		want  int
	}{
		{Lit{V: math.MaxInt}, math.MaxInt}, {Lit{V: math.MinInt}, math.MinInt},
		{Lit{V: namedInt(math.MaxInt - 1)}, math.MaxInt - 1}, {Lit{V: namedUint(math.MaxInt)}, math.MaxInt},
		{Lit{V: float64(-42)}, -42}, {Lit{V: float32(250)}, 250},
		{Lit{V: namedString(strconv.Itoa(math.MaxInt))}, math.MaxInt},
		{Persisted{Source: strconv.Itoa(math.MinInt)}, math.MinInt},
	} {
		if got, ok := propertyInteger(tc.value); !ok || got != tc.want {
			t.Errorf("%#v: got %d valid%v want%d", tc.value, got, ok, tc.want)
		}
	}
	for _, value := range []Value{Lit{V: math.Ldexp(1, strconv.IntSize-1)}, Lit{V: math.NaN()}, Lit{V: math.Inf(-1)}, Lit{V: 1.25}, Lit{V: uint64(math.MaxInt) + 1}, Lit{V: nil}, Lit{V: true}, Lit{V: "1.25"}, Persisted{Source: "1+1"}, Persisted{Source: "1.0"}, Persisted{Source: "\"42\""}, TrKey{Source: "42"}, Ref{}} {
		if got, ok := propertyInteger(value); ok {
			t.Errorf("accepted %#v as %d", value, got)
		}
	}
}

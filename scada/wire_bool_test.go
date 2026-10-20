package scada

import (
	"testing"

	"github.com/uk0/silk/core"
)

func TestBoolSampleSetterPreservesRawNumericCoercions(t *testing.T) {
	for _, tc := range []struct {
		raw  any
		want bool
	}{{true, true}, {false, false}, {float32(1), true}, {float32(0), false}, {int32(-2), true}, {int32(0), false}, {float64(3), true}, {nil, false}} {
		got := !tc.want
		boolSampleSetter(func(v bool) { got = v })(core.Value{Raw: tc.raw})
		if got != tc.want {
			t.Errorf("%T(%v): got %v, want %v", tc.raw, tc.raw, got, tc.want)
		}
	}
}

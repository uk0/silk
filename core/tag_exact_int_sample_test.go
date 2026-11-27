package core

import (
	"math"
	"testing"
)

func TestTagIntPreservesTypedInt64(t *testing.T) {
	for _, exact := range []int64{math.MinInt64, -(1 << 53) - 1, (1 << 53) + 1, math.MaxInt64} {
		if v := (Value{Raw: exact}).Int(); v != exact {
			t.Errorf("typed integer lost precision: want %d got %d", exact, v)
		}
	}
	if got := (Value{Raw: 12.5}).Int(); got != 12 {
		t.Errorf("legacy float narrowing changed: %d", got)
	}
}

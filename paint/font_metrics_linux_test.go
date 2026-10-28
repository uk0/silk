//go:build linux && cgo && !silk_pure_go

package paint

import (
	"math"
	"testing"

	"github.com/uk0/silk/cairo"
	"github.com/uk0/silk/geom"
)

func TestLinuxLogicalFontMetricsSurviveFractionalScale(t *testing.T) {
	if globalFontOptions.HintMetrics() != cairo.HINT_METRICS_OFF {
		t.Fatal("device-rounded metrics invalidate retained logical layouts")
	}
	for _, family := range []string{"sans", "monospace"} {
		for _, size := range []int{12, 17, 24} {
			font := NewFont(family, size, false, false)
			text := "Wi apple 123.45"
			base := LayoutText(font.ScaledFont(nil), text)
			for _, scale := range []float64{.75, 1.25, 1.5, 1.75, 2} {
				var ctm geom.Mat3x2
				ctm.InitScale(scale, scale)
				got := LayoutText(font.ScaledFont(&ctm), text)
				if len(got.Carets) != len(base.Carets) {
					t.Fatal("lost caret boundaries")
				}
				for i, x := range base.Carets {
					if math.Abs(got.Carets[i]-x) > .000001 {
						t.Fatalf("family=%s size=%d scale=%g caret=%d: got=%g logical=%g", family, size, scale, i, got.Carets[i], x)
					}
				}
			}
		}
	}
}

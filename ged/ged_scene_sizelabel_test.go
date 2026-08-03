package ged

import (
	"fmt"
	"testing"

	"github.com/uk0/silk/paint"
)

// TestSizeLabelStaysInsideForm: the form-size readout must end inside the
// form's right edge for every form size. DrawAll clips each item to its own
// bounds, so a readout laid out from a fixed offset is cut off by the clip as
// soon as the string is wider than that offset — at the default 100x100 form
// the 7mm-tall "100 x 100 mm" advances 44mm, nearly twice the old 25mm offset.
func TestSizeLabelStaysInsideForm(t *testing.T) {
	f := paint.NewFont("", sizeLabelFontSize, false, false)

	for _, tc := range []struct{ w, h float64 }{
		{100, 100}, // the default new-form size
		{60, 40},
		{1024, 768},
	} {
		label := fmt.Sprintf("%.0f x %.0f mm", tc.w, tc.h)
		adv := f.TextExtents(label).XAdvance
		x := sizeLabelX(0, tc.w, adv, sizeLabelInset)

		if x+adv > tc.w {
			t.Errorf("form %.0fx%.0f: label %q spans [%.2f,%.2f] mm, past the form's right edge %.0f",
				tc.w, tc.h, label, x, x+adv, tc.w)
		}
		if x < 0 {
			t.Errorf("form %.0fx%.0f: label starts at %.2f mm, left of the form", tc.w, tc.h, x)
		}
	}
}

// TestSizeLabelClampedOnNarrowForm: a form narrower than its own readout still
// starts the label at the left edge rather than outside the form, so the part
// that survives the clip is the leading digits and not a blank strip.
func TestSizeLabelClampedOnNarrowForm(t *testing.T) {
	if x := sizeLabelX(10, 5, 44, sizeLabelInset); x != 10 {
		t.Errorf("sizeLabelX on a too-narrow form = %v, want the form's left edge 10", x)
	}
}

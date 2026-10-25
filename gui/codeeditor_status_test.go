package gui

import "testing"

func TestEditorStatusNeverOverlapsAtNarrowWidths(t *testing.T) {
	for _, widths := range [][5]float64{{80, 125, 34, 14, 62}, {210, 220, 50, 24, 90}} {
		for width := 0.0; width <= 960; width++ {
			x := editorStatusSlots(width, widths)
			for i := range x {
				if x[i] < 0 {
					continue
				}
				if x[i] < 10 || x[i]+widths[i] > width-10 {
					t.Fatalf("width=%g: field %d outside status bar: %v", width, i, x)
				}
				for j := i + 1; j < len(x); j++ {
					if x[j] >= 0 && x[i] < x[j]+widths[j]+16 && x[j] < x[i]+widths[i]+16 {
						t.Fatalf("width=%g: fields %d and %d overlap: %v", width, i, j, x)
					}
				}
			}
			if width >= widths[0]+20 && x[0] != 10 {
				t.Fatal("position was hidden despite fitting", width, x)
			}
		}
	}
}

func TestEditorStatusKeepsPrimaryFieldsBeforeCounts(t *testing.T) {
	widths := [5]float64{80, 125, 34, 14, 62}
	x := editorStatusSlots(330, widths)
	if x[0] < 0 || x[2] < 0 || x[3] < 0 || x[4] < 0 || x[1] >= 0 {
		t.Fatal("narrow editor must keep position and modes without colliding counts", x)
	}
	x = editorStatusSlots(900, widths)
	for i, v := range x {
		if v < 0 {
			t.Fatal("wide editor unexpectedly hid field", i)
		}
	}
}

func TestEditorStatusIndentNeverDisplacesPrimaryFields(t *testing.T) {
	for _, widths := range [][6]float64{{80, 125, 34, 24, 62, 72}, {210, 220, 50, 40, 90, 96}} {
		for width := 0.0; width < 1200; width++ {
			old := editorStatusSlots(width, [5]float64{widths[0], widths[1], widths[2], widths[3], widths[4]})
			got := editorStatusSlotsWithIndent(width, widths)
			for i, v := range old {
				if got[i] != v {
					t.Fatal("indentation displaced existing field", width, got, old)
				}
			}
			if got[5] >= 0 {
				if got[5] < 10 || got[5]+widths[5] > width-10 {
					t.Fatal("indent label clipped", width, got)
				}
				for i := 0; i < 5; i++ {
					if got[i] >= 0 && got[5] < got[i]+widths[i]+16 && got[i] < got[5]+widths[5]+16 {
						t.Fatal("indent label overlaps", width, got)
					}
				}
			}
			if width >= 1100 && got[5] < 0 {
				t.Fatal("wide pane hid indentation")
			}
		}
	}
}

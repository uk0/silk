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

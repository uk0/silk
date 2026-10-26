package ged

import (
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"math"
	"testing"
)

func requirePrecisePosition(t *testing.T, item graph.IItem, x, y float64) {
	t.Helper()
	if math.Abs(item.X()-x) > 1e-9 || math.Abs(item.Y()-y) > 1e-9 {
		t.Fatalf("position=(%.12g, %.12g), want=(%.12g, %.12g)", item.X(), item.Y(), x, y)
	}
}

func TestPreciseKeyboardMoveHistory(t *testing.T) {
	for _, snap := range []bool{false, true} {
		t.Run(map[bool]string{false: "snap-off", true: "snap-on"}[snap], func(t *testing.T) {
			_, v, item, sb := selectionStatusFixture(t)
			defer v.Close()
			item.SetBounds(15.25, 20.75, 60.5, 12.125)
			v.SetSnapEnabled(snap)
			v.OnKeyDown(gui.KeyRight, false)
			for i := 0; i < 20; i++ {
				v.OnKeyDown(gui.KeyRight, true)
			}
			requirePrecisePosition(t, item, 36.25, 20.75)
			if v.Scene().UndoStack().Count() != 1 {
				t.Fatal("repeat did not merge")
			}
			if sb.Message() != "Selected: SpinBox (36.25, 20.75) 60.5×12.125 mm" {
				t.Fatal(sb.Message())
			}
			historyCanvasAction(t, v, "edit-undo").Trigger(v)
			requirePrecisePosition(t, item, 15.25, 20.75)
			historyCanvasAction(t, v, "edit-redo").Trigger(v)
			requirePrecisePosition(t, item, 36.25, 20.75)
			v.OnKeyDown(gui.KeyLeft, false)
			requirePrecisePosition(t, item, 35.25, 20.75)
			v.OnKeyDown(gui.KeyDown, false)
			requirePrecisePosition(t, item, 35.25, 21.75)
			v.OnKeyDown(gui.KeyUp, false)
			requirePrecisePosition(t, item, 35.25, 20.75)
			v.SetGridStep(2.5)
			dx, dy, _ := nudgeDelta(gui.KeyDown, true, 1, v.coarseNudgeStep())
			v.nudgeSelection(dx, dy)
			requirePrecisePosition(t, item, 35.25, 23.25)
			if item.Width() != 60.5 || item.Height() != 12.125 {
				t.Fatal("move resized item")
			}
		})
	}
}

func TestPreciseAlignmentAndDistribution(t *testing.T) {
	t.Run("fractional-pinned-edge", func(t *testing.T) {
		v := NewGedView()
		defer v.Close()
		s := v.GedScene()
		a := addFakeAt(t, s, "a", 10.25, 20.75, 10.5, 8.125)
		b := addFakeAt(t, s, "b", 30.5, 40.125, 12.25, 9.5)
		a.SetLockPos(true)
		v.Selection().Add(a)
		v.Selection().Add(b)
		v.AlignSelection(AlignLeft)
		requirePrecisePosition(t, a, 10.25, 20.75)
		requirePrecisePosition(t, b, 10.25, 40.125)
		v.Undo()
		requirePrecisePosition(t, b, 30.5, 40.125)
		v.Redo()
		requirePrecisePosition(t, b, 10.25, 40.125)
	})
	t.Run("fractional-gap", func(t *testing.T) {
		v := NewGedView()
		defer v.Close()
		s := v.GedScene()
		a := addFakeAt(t, s, "a", .25, 4.75, 10.5, 8)
		b := addFakeAt(t, s, "b", 30.5, 6.125, 12.25, 8)
		c := addFakeAt(t, s, "c", 75.75, 8.25, 14.125, 8)
		v.Selection().AddMulti([]graph.IItem{a, b, c})
		v.AlignSelection(DistributeH)
		requirePrecisePosition(t, a, .25, 4.75)
		requirePrecisePosition(t, b, 37.125, 6.125)
		requirePrecisePosition(t, c, 75.75, 8.25)
		if math.Abs((b.X()-a.X()-a.Width())-(c.X()-b.X()-b.Width())) > 1e-9 {
			t.Fatal("unequal gaps")
		}
		v.Undo()
		requirePrecisePosition(t, b, 30.5, 6.125)
		v.Redo()
		requirePrecisePosition(t, b, 37.125, 6.125)
	})
	t.Run("fractional-centre", func(t *testing.T) {
		v := NewGedView()
		defer v.Close()
		s := v.GedScene()
		a := addFakeAt(t, s, "a", 10.25, 20.75, 10.5, 8)
		b := addFakeAt(t, s, "b", 30.5, 40.125, 12.25, 8)
		v.Selection().AddMulti([]graph.IItem{a, b})
		v.AlignSelection(AlignHCenter)
		requirePrecisePosition(t, a, 20.8125, 20.75)
		requirePrecisePosition(t, b, 19.9375, 40.125)
	})
}

func TestPreciseCanvasSnapIsExplicit(t *testing.T) {
	for _, tc := range []struct {
		name        string
		snap        bool
		pitch, x, y float64
	}{{"off", false, 5, 22.75, 29.25}, {"on", true, 5, 25, 30}, {"fractional-pitch", true, 2.5, 22.5, 30}} {
		t.Run(tc.name, func(t *testing.T) {
			v := NewGedView()
			defer v.Close()
			v.SetSize(700, 500)
			v.Layout()
			s := v.GedScene()
			s.SetSize(200, 150)
			item := addFakeAt(t, s, "a", 15.25, 20.75, 30.5, 12.125)
			v.SetSnapEnabled(tc.snap)
			v.SetGridStep(tc.pitch)
			dragOnCanvas(v, 20, 25, 27.5, 33.5)
			requirePrecisePosition(t, item, tc.x, tc.y)
			for i := 0; i < 4; i++ {
				v.Undo()
				requirePrecisePosition(t, item, 15.25, 20.75)
				v.Redo()
				requirePrecisePosition(t, item, tc.x, tc.y)
			}
		})
	}
}

package gui

import (
	"testing"
)

func TestHBoxLayoutPositions(t *testing.T) {
	box := NewHBox()
	box.SetSpacing(4)
	box.SetSize(300, 50)

	b1 := NewButton1("A", nil)
	b2 := NewButton1("B", nil)
	b3 := NewButton1("C", nil)
	box.AddWidget(b1)
	box.AddWidget(b2)
	box.AddWidget(b3)

	box.Layout()

	x1 := b1.X()
	x2 := b2.X()
	x3 := b3.X()

	if x2 <= x1 {
		t.Errorf("b2.X (%f) should be > b1.X (%f)", x2, x1)
	}
	if x3 <= x2 {
		t.Errorf("b3.X (%f) should be > b2.X (%f)", x3, x2)
	}
}

func TestVBoxLayoutPositions(t *testing.T) {
	box := NewVBox()
	box.SetSpacing(4)
	box.SetSize(200, 300)

	l1 := NewLabel("A")
	l2 := NewLabel("B")
	l3 := NewLabel("C")
	box.AddWidget(l1)
	box.AddWidget(l2)
	box.AddWidget(l3)

	box.Layout()

	y1 := l1.Y()
	y2 := l2.Y()
	y3 := l3.Y()

	if y2 <= y1 {
		t.Errorf("l2.Y (%f) should be > l1.Y (%f)", y2, y1)
	}
	if y3 <= y2 {
		t.Errorf("l3.Y (%f) should be > l2.Y (%f)", y3, y2)
	}
}

func TestHBoxHiddenWidgetSkipped(t *testing.T) {
	box := NewHBox()
	box.SetSpacing(4)
	box.SetSize(300, 50)

	b1 := NewButton1("A", nil)
	b2 := NewButton1("B", nil) // will be hidden
	b3 := NewButton1("C", nil)
	box.AddWidget(b1)
	box.AddWidget(b2)
	box.AddWidget(b3)

	// Record positions with b2 visible
	box.Layout()
	x3_visible := b3.X()

	// Now hide b2 and re-layout
	b2.Hide()
	box.Layout()
	x3_hidden := b3.X()

	// b3 should be closer to b1 when b2 is hidden
	if x3_hidden >= x3_visible {
		t.Errorf("b3.X with b2 hidden (%f) should be < b3.X with b2 visible (%f)", x3_hidden, x3_visible)
	}
}

func TestVBoxHiddenWidgetSkipped(t *testing.T) {
	box := NewVBox()
	box.SetSpacing(4)
	box.SetSize(200, 300)

	l1 := NewLabel("A")
	l2 := NewLabel("B") // will be hidden
	l3 := NewLabel("C")
	box.AddWidget(l1)
	box.AddWidget(l2)
	box.AddWidget(l3)

	box.Layout()
	y3_visible := l3.Y()

	l2.Hide()
	box.Layout()
	y3_hidden := l3.Y()

	if y3_hidden >= y3_visible {
		t.Errorf("l3.Y with l2 hidden (%f) should be < l3.Y with l2 visible (%f)", y3_hidden, y3_visible)
	}
}

func TestHBoxSpacingAffectsPositions(t *testing.T) {
	box := NewHBox()
	box.SetSize(300, 50)

	b1 := NewButton1("A", nil)
	b2 := NewButton1("B", nil)
	box.AddWidget(b1)
	box.AddWidget(b2)

	// Layout with spacing=0
	box.SetSpacing(0)
	box.Layout()
	x2_tight := b2.X()

	// Layout with spacing=20
	box.SetSpacing(20)
	box.Layout()
	x2_spaced := b2.X()

	if x2_spaced <= x2_tight {
		t.Errorf("b2.X with spacing 20 (%f) should be > b2.X with spacing 0 (%f)", x2_spaced, x2_tight)
	}
}

func TestVBoxChildWidthFillsContainer(t *testing.T) {
	box := NewVBox()
	box.SetSize(200, 300)
	box.SetSpacing(0)

	lbl := NewLabel("test")
	box.AddWidget(lbl)

	box.Layout()

	// Child should fill the full width of the VBox (no padding)
	w := lbl.Width()
	if w != 200 {
		t.Errorf("child width = %f, want 200 (fill container)", w)
	}
}

func TestHBoxChildHeightFillsContainer(t *testing.T) {
	box := NewHBox()
	box.SetSize(300, 50)
	box.SetSpacing(0)

	lbl := NewLabel("test")
	box.AddWidget(lbl)

	box.Layout()

	h := lbl.Height()
	if h != 50 {
		t.Errorf("child height = %f, want 50 (fill container)", h)
	}
}

func TestHBoxWithPadding(t *testing.T) {
	box := NewHBox()
	box.SetSize(300, 50)
	box.SetSpacing(0)
	box.SetPadding(Padding{L: 10, R: 10, T: 5, B: 5})

	b := NewButton1("Test", nil)
	box.AddWidget(b)

	box.Layout()

	if b.X() < 10 {
		t.Errorf("child X (%f) should be >= padding left (10)", b.X())
	}
	if b.Y() < 5 {
		t.Errorf("child Y (%f) should be >= padding top (5)", b.Y())
	}
}

// --- Edge-case regression tests ---

func TestHBoxZeroStretchNoDiv0(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("HBox Layout panicked with zero-stretch children: %v", r)
		}
	}()
	box := NewHBox()
	box.SetSize(200, 50)
	b := NewButton1("A", nil)
	box.AddWidget(b)
	box.Layout() // should not panic
}

func TestVBoxZeroStretchNoDiv0(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("VBox Layout panicked with zero-stretch children: %v", r)
		}
	}()
	box := NewVBox()
	box.SetSize(200, 50)
	b := NewButton1("A", nil)
	box.AddWidget(b)
	box.Layout() // should not panic
}

func TestGridLayoutNegativeRemaining(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GridLayout panicked when spacing > size: %v", r)
		}
	}()
	grid := NewGridLayout()
	grid.SetSize(10, 10)
	grid.SetSpacing(100) // spacing > size
	b := NewButton1("A", nil)
	grid.AddWidget(b, 0, 0)
	grid.Layout() // should not panic
}

func TestCodeEditorEmptyLines(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CodeEditor panicked with empty text: %v", r)
		}
	}()
	e := NewCodeEditor()
	e.SetText("")
	// clampCursor should not panic on empty content
}

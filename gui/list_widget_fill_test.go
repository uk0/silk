package gui

import "testing"

func TestListWidgetFinderViewportFillsAvailableSpace(t *testing.T) {
	box := NewVBox()
	box.SetSpacing(6)
	input := NewEdit()
	list := NewListWidget()
	list.SetFillAvailable(true)
	// Lists in finder dialogs must remain a viewport even while async results
	// are empty, not adopt the eight-pixel empty-content preferred height.
	box.AddWidget(input)
	box.AddWidget(list)
	box.SetSize(512, 360)
	box.Layout()
	if list.Height() < 250 || list.Width() != box.Width() {
		t.Fatalf("collapsed finder viewport: %gx%g in %gx%g", list.Width(), list.Height(), box.Width(), box.Height())
	}
}

func TestListWidgetFillAvailableStableAcrossAsyncResults(t *testing.T) {
	box := NewVBox()
	box.SetSpacing(6)
	input, list := NewEdit(), NewListWidget()
	box.AddWidget(input)
	box.AddWidget(list)
	box.SetSize(512, 360)
	before := list.SizeHints()
	if list.FillAvailable() || before.Stretch != 0 {
		t.Fatal("legacy content sizing changed")
	}
	list.SetFillAvailable(true) // must relayout the existing parent immediately
	w, h := list.Size()
	if w != 512 || h < 250 {
		t.Fatalf("setter did not relayout parent: %gx%g", w, h)
	}
	for _, count := range []int{1000, 0, 1, 20} {
		list.Clear()
		for i := 0; i < count; i++ {
			list.Append(ListItem{Text: "Result"})
		}
		box.Layout()
		if list.Width() != w || list.Height() != h {
			t.Fatalf("%d results changed viewport to %gx%g", count, list.Width(), list.Height())
		}
		if list.Y() < input.Y()+input.Height()+6 || list.Y()+list.Height() > box.Height() {
			t.Fatal("list overlaps input or escapes content")
		}
	}
	box.SetSize(720, 520)
	if list.Width() != 720 || list.Height() <= h {
		t.Fatal("viewport did not follow parent resize")
	}
	list.SetActiveIndex(3)
	list.SetFillAvailable(false)
	if list.SizeHints().Stretch != 0 || list.Count() != 20 || list.ActiveIndex() != 3 {
		t.Fatal("opt-out changed rows or selection")
	}
}

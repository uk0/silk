package gui

import "testing"

func TestDialogFooterAddedAfterSizeIsUsable(t *testing.T) {
	d := NewDialog("Timer", nil)
	d.SetContent(NewLabel("Calls: 1 | depth: 1 | heartbeats: 0"))
	d.SetSize(540, 180)
	b := d.AddButton("Close", DialogCancel)
	if b.Height() < 32 || b.Width() < 88 || b.Bounds1().Right() > d.btnBox.Width() || b.Bounds1().Bottom() > d.btnBox.Height() {
		t.Fatal("footer action outside usable bounds")
	}
	next := d.AddButton("Confirm", DialogOK)
	if next.Height() < 32 || next.Width() < 88 || b.X()+b.Width()+8 > next.X() || next.X()+next.Width() != d.Width()-24 {
		t.Fatal("late footer action overlaps or loses trailing inset")
	}
	d.Layout()
	if next.X()+next.Width() != d.Width()-24 {
		t.Fatal("unchanged-size relayout moved footer")
	}
}

package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/driver"
)

// TestPointsEditorSetText verifies SetPointsText parses the CSV into the row
// model — one row per point, fields decoded through the shared parser — and that
// a malformed set is rejected without disturbing the current rows. No Draw.
func TestPointsEditorSetText(t *testing.T) {
	e := NewDevicePointsEditor()
	text := "level, hr:0, Float32, ABCD, RO\nsp, hr:2, Int32, CDAB, RW\n"
	if err := e.SetPointsText(text); err != nil {
		t.Fatalf("SetPointsText: %v", err)
	}
	if e.RowCount() != 2 {
		t.Fatalf("RowCount = %d, want 2", e.RowCount())
	}
	rows := e.Rows()
	if rows[0].Tag != "level" || rows[0].Type != driver.TypeFloat32 {
		t.Errorf("row 0 = %+v, want level/Float32", rows[0])
	}
	if rows[1].Access != driver.ReadWrite || rows[1].Order != driver.LittleByteSwap {
		t.Errorf("row 1 = %+v, want RW/CDAB", rows[1])
	}
	if got := e.Cell(1, colType); got != "Int32" {
		t.Errorf("Cell(1, colType) = %q, want Int32", got)
	}

	// A malformed set is rejected and leaves the model unchanged.
	if err := e.SetPointsText("x,y,NotAType"); err == nil {
		t.Errorf("SetPointsText(bad) = nil, want error")
	}
	if e.RowCount() != 2 {
		t.Errorf("RowCount after bad set = %d, want 2 (unchanged)", e.RowCount())
	}
}

// TestPointsEditorEditEmits verifies an inline edit updates the model and fires
// SigChanged once with the regenerated CSV text; a rejected edit fires nothing
// and leaves the row intact. No Draw.
func TestPointsEditorEditEmits(t *testing.T) {
	e := NewDevicePointsEditor()
	if err := e.SetPointsText("level, hr:0, Float32, ABCD, RO\n"); err != nil {
		t.Fatalf("SetPointsText: %v", err)
	}

	var got string
	var fired int
	e.SigChanged(func(text string) {
		got = text
		fired++
	})

	if err := e.SetCell(0, colTag, "pressure"); err != nil {
		t.Fatalf("SetCell: %v", err)
	}
	if fired != 1 {
		t.Fatalf("SigChanged fired %d times, want 1", fired)
	}
	if e.Rows()[0].Tag != "pressure" {
		t.Errorf("row 0 tag = %q, want pressure", e.Rows()[0].Tag)
	}
	if !strings.HasPrefix(got, "pressure,hr:0,Float32,ABCD,RO") {
		t.Errorf("SigChanged text = %q, want it to start with the edited point", got)
	}
	if got != e.PointsText() {
		t.Errorf("SigChanged text %q != PointsText %q", got, e.PointsText())
	}

	// An edit to an invalid type is rejected: no emit, row unchanged.
	if err := e.SetCell(0, colType, "NotAType"); err == nil {
		t.Errorf("SetCell(bad type) = nil, want error")
	}
	if fired != 1 {
		t.Errorf("SigChanged fired %d times after failed edit, want 1", fired)
	}
	if e.Rows()[0].Type != driver.TypeFloat32 {
		t.Errorf("row 0 type = %v after failed edit, want Float32", e.Rows()[0].Type)
	}
}

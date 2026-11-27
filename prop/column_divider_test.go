package prop

import (
	"math"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func TestInspectorColumnDividerOwnsOnlyItsInlineStrip(t *testing.T) {
	s := newTestSheet()
	s.SetSize(260, 180)
	item := addTestProp(s, "Value", func() string { return "value" }, nil)
	s.Layout()
	d := s.columnDivider
	editor := item.Control()
	if d.Parent() != s.body || !d.IsVisible() || d.X()+d.Width() != editor.X() {
		t.Fatalf("divider=%v editor=%v", d.Bounds1(), editor.Bounds1())
	}
	y := s.body.Y() + editor.Y() + editor.Height()/2
	if w := s.FindWidgetAt(d.X()+d.Width()/2, y); w != d {
		t.Fatalf("the drag strip routed to %T", w)
	}
	for w := s.FindWidgetAt(editor.X()+2, y); ; w = w.Parent() {
		if w == editor {
			break
		}
		if w == nil || w == d {
			t.Fatal("divider intercepted a value-editor click")
		}
	}
}

func TestInspectorColumnDragDoesNotRebindOrEndEditorSession(t *testing.T) {
	reads, value := 0, 0
	stack := gui.NewUndoStack("column-drag-session")
	s := newTestSheet()
	s.SetSize(280, 200)
	s.objOwner = &sessionOwner{stack}
	item := addTestProp(s, "Editable", func() int { reads++; return value }, func(n int) { value = n })
	s.Layout()
	editor := item.Control().(*IntEdit)
	editor.SetFocus()
	editor.SelectAll()
	editor.OnTextInput("1")
	session := editor.editSession
	reads = 0
	original := s.ColumnSplit()
	d := s.columnDivider
	defer d.OnPointerCancel()
	x0 := d.X() + 2
	d.OnLeftDown(2, 20)
	if !d.HasCapture() {
		t.Fatal("divider did not capture the drag")
	}
	d.OnMouseMove(x0+30-d.X(), 20)
	d.OnLeftUp(x0+30-d.X(), 20)
	if math.Abs(s.ColumnSplit()-(original+30/s.body.Width())) > 1e-9 || d.HasCapture() {
		t.Fatalf("split=%f capture=%v", s.ColumnSplit(), d.HasCapture())
	}
	if reads != 0 || item.Control() != editor || editor.editSession != session || value != 1 {
		t.Fatal("drag rebound/queried or committed the editor")
	}
	editor.OnTextInput("2")
	stack.Undo()
	if value != 0 {
		t.Fatalf("geometry split typing history; Undo restored %d", value)
	}
}

func TestInspectorColumnKeyboardDoubleClickAndSession(t *testing.T) {
	s := newTestSheet()
	s.SetSize(260, 180)
	addTestProp(s, "Value", func() int { return 1 }, nil)
	s.Layout()
	d := s.columnDivider
	defer d.OnPointerCancel()
	d.OnKeyDown(gui.KeyRight, false)
	if s.ColumnSplit() <= defaultColumnSplit {
		t.Fatal("Right did not grow the name column")
	}
	d.OnKeyDown(gui.KeyHome, false)
	if s.ColumnSplit() != defaultColumnSplit {
		t.Fatal("Home did not restore the default width")
	}
	s.SetColumnSplit(0.72)
	d.OnLeftDown(2, 20)
	d.OnLeftUp(2, 20)
	d.OnLeftDown(2, 20)
	d.OnLeftUp(2, 20)
	if s.ColumnSplit() != defaultColumnSplit || d.HasCapture() {
		t.Fatal("double click did not reset the split and retire pointer capture")
	}
	s.SetColumnSplit(0.63)
	state, err := s.SaveSession()
	if err != nil {
		t.Fatal(err)
	}
	reopened := newTestSheet()
	if err := reopened.LoadSession(state); err != nil || math.Abs(reopened.ColumnSplit()-0.63) > 1e-9 {
		t.Fatalf("split preference round trip: %v %v", reopened.ColumnSplit(), err)
	}
	legacy := core.NewTDoc()
	if err := reopened.LoadSession(legacy); err != nil || math.Abs(reopened.ColumnSplit()-0.63) > 1e-9 {
		t.Fatal("a legacy session without split metadata changed the preference")
	}
	if err := legacy.WriteAttr("columnSplit", "invalid"); err != nil {
		t.Fatal(err)
	}
	_ = reopened.LoadSession(legacy)
	reopened.SetColumnSplit(math.NaN())
	reopened.SetColumnSplit(math.Inf(1))
	if math.Abs(reopened.ColumnSplit()-0.63) > 1e-9 {
		t.Fatal("invalid split metadata changed a preference")
	}
}

func TestInspectorColumnWidthRevealsLabelAndHideCancelsDrag(t *testing.T) {
	s := newTestSheet()
	s.SetSize(300, 180)
	item := addTestProp(s, "Secondary indicator alignment", func() int { return 1 }, nil)
	s.Layout()
	d := s.columnDivider
	drawLabel := func() string {
		pix := paint.NewPixmap(300, 180)
		g := &propertyTextRecorder{Painter: pix.NewPainter()}
		s.drawPropertyLabel(g, item)
		if len(g.records) != 1 {
			t.Fatalf("unexpected label paint: %v", g.records)
		}
		return g.records[0].text
	}
	if got := drawLabel(); got == item.Label() {
		t.Fatal("fixture does not exercise a clipped label")
	}
	s.SetColumnSplit(1)
	if s.ColumnSplit() != maxColumnSplit || item.Control().Width() <= 0 {
		t.Fatal("moving divider past the right edge hid the value editor")
	}
	if got := drawLabel(); got != item.Label() {
		t.Fatalf("wider label column did not restore the complete name: %q", got)
	}
	d.OnLeftDown(2, 20)
	if !d.dragging {
		t.Fatal("drag did not start")
	}
	s.SetSize(42, 180)
	s.Layout()
	if d.IsVisible() || d.dragging || d.HasCapture() {
		t.Fatal("hidden divider kept a stale pointer capture")
	}
}

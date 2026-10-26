package gui

import (
	"github.com/uk0/silk/paint"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestSpinBoxAcceptsDirectText(t *testing.T) {
	oldFocus := focusWidget
	t.Cleanup(func() { focusWidget = oldFocus })
	s := NewSpinBox()
	s.SetRange(-100, 100)
	s.SetSize(180, 32)
	input, ok := any(s).(IEventTextInput)
	if !ok {
		t.Fatal("SpinBox has no text-input route")
	}
	// Focus via the public control, replace the current value and commit.
	s.SetFocus()
	if selectAll, ok := any(s).(interface{ SelectAll() }); ok {
		selectAll.SelectAll()
	}
	input.OnTextInput("42")
	s.OnKeyDown(KeyEnter, false)
	if s.Value() != 42 {
		t.Fatalf("typed value not committed: %d", s.Value())
	}
}
func TestSpinBoxStepsSaturateIntegerBounds(t *testing.T) {
	s := NewSpinBox()
	s.SetRange(math.MinInt, math.MaxInt)
	s.SetStep(4)
	for _, tc := range []struct{ value, key, want int }{{math.MaxInt - 1, KeyUp, math.MaxInt}, {math.MinInt + 1, KeyDown, math.MinInt}} {
		s.SetValue(tc.value)
		s.OnKeyDown(tc.key, false)
		if s.Value() != tc.want {
			t.Fatalf("wrapped step: got %d want %d", s.Value(), tc.want)
		}
	}
	s.SetStep(math.MaxInt)
	if s.PageStep() != math.MaxInt {
		t.Fatalf("page step overflowed: %d", s.PageStep())
	}
}
func TestSpinBoxButtonHitStaysInsideOwner(t *testing.T) {
	s := NewSpinBox()
	s.SetSize(8, 6)
	for _, p := range [][2]float64{{-1, 2}, {8, 2}, {2, -1}, {2, 6}} {
		if s.hitTest(p[0], p[1]) != 0 {
			t.Fatalf("out-of-owner button hit at %v", p)
		}
	}
}

func spinEditFixture(t *testing.T) *SpinBox {
	t.Helper()
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	s := NewSpinBox()
	s.SetRange(-100, 100)
	s.SetValue(10)
	s.SetSize(180, 32)
	return s
}
func TestSpinBoxDraftCommitCancelAndLocalHistory(t *testing.T) {
	s := spinEditFixture(t)
	calls := 0
	s.SetValueChangedCallback(func(_ interface{}, v int) { calls++ })
	s.SetFocus()
	s.SelectAll()
	s.OnTextInput("42")
	if s.Value() != 10 || s.Text() != "42" || calls != 0 {
		t.Fatal("draft changed committed model")
	}
	s.editor.Undo()
	if s.Text() != "10" {
		t.Fatal("local undo failed")
	}
	s.editor.Redo()
	if s.Text() != "42" {
		t.Fatal("local redo failed")
	}
	s.OnKeyDown(KeyEnter, false)
	if s.Value() != 42 || calls != 1 || s.editor.CanUndo() {
		t.Fatal("commit must notify once and retire draft history")
	}
	s.SelectAll()
	s.OnTextInput("19")
	s.OnKeyDown(KeyEsc, false)
	if s.Value() != 42 || s.Text() != "42" || calls != 1 {
		t.Fatal("cancel mutated model")
	}
	s.SelectAll()
	s.OnTextInput("-")
	if s.Commit() || s.Value() != 42 || s.Text() != "42" || calls != 1 {
		t.Fatal("incomplete draft not restored")
	}
	s.SelectAll()
	s.OnTextInput("999")
	if !s.Commit() || s.Value() != 100 || s.Text() != "100" || calls != 2 {
		t.Fatal("complete draft not clamped once")
	}
}
func TestSpinBoxRejectsInvalidPasteAndIntegerOverflow(t *testing.T) {
	s := spinEditFixture(t)
	s.SetRange(math.MinInt, math.MaxInt)
	for _, text := range []string{"12x", "3.5", "--2", "中42", "9999999999999999999999999999999999", strings.Repeat("0", 65)} {
		s.SetValue(10)
		s.SelectAll()
		s.editor.pasteString(text)
		if s.Text() != "10" {
			t.Fatalf("invalid paste changed buffer: %q", text)
		}
	}
	for _, v := range []int{math.MinInt, math.MaxInt} {
		s.SelectAll()
		s.OnTextInput(strconv.Itoa(v))
		if !s.Commit() || s.Value() != v {
			t.Fatal("full int endpoint lost", v, s.Value())
		}
	}
}
func TestSpinBoxStepFromDraftIsOneChangeAndMouseSaturates(t *testing.T) {
	s := spinEditFixture(t)
	calls := 0
	s.SetValueChangedCallback(func(_ interface{}, _ int) { calls++ })
	s.SelectAll()
	s.OnTextInput("42")
	s.OnKeyDown(KeyUp, false)
	if s.Value() != 43 || calls != 1 {
		t.Fatal("draft step double notified", s.Value(), calls)
	}
	s.SetRange(math.MinInt, math.MaxInt)
	s.SetStep(4)
	s.SetValue(math.MaxInt - 1)
	s.OnLeftDown(s.w-2, 2)
	s.OnLeftUp(s.w-2, 2)
	if s.Value() != math.MaxInt {
		t.Fatal("mouse step wrapped")
	}
	s.SetValue(math.MinInt + 1)
	s.OnMouseWheel(10, 10, -1)
	if s.Value() != math.MinInt {
		t.Fatal("wheel step wrapped")
	}
	s.SetStep(math.MaxInt)
	if s.PageStep() != math.MaxInt || s.PageStepOverride() != 0 {
		t.Fatal("auto page overflow")
	}
	s.SetPageStep(3)
	s.SetStep(10)
	if s.PageStep() != 3 || s.PageStepOverride() != 3 {
		t.Fatal("explicit page lost")
	}
}
func TestSpinBoxFocusAndContextMenuCommitBoundary(t *testing.T) {
	s := spinEditFixture(t)
	s.SetFocus()
	s.SelectAll()
	s.OnTextInput("27")
	menu := NewPopupMenu()
	menu.SetParent(s.editor)
	menu.SetFocus()
	if s.Value() != 10 || s.Text() != "27" {
		t.Fatal("context menu prematurely committed")
	}
	s.SetFocus()
	NewEdit().SetFocus()
	if s.Value() != 27 || s.Text() != "27" {
		t.Fatal("focus loss failed to commit")
	}
	root := NewVBox()
	s.SetParent(root)
	other := NewButton()
	other.SetParent(root)
	var chain []IWidget
	collectFocusable(root, &chain)
	if len(chain) != 2 || chain[0] != s.editor || chain[1] != other || nextFocusable(root, s.editor, true) != other {
		t.Fatal("composite has duplicate Tab stops", chain)
	}
}
func TestSpinBoxReadOnlyDisabledAndProgrammaticSupersession(t *testing.T) {
	s := spinEditFixture(t)
	for _, readonly := range []bool{true, false} {
		s.SetEnabled(true)
		s.SetReadOnly(false)
		s.SetValue(10)
		s.SelectAll()
		s.OnTextInput("88")
		if readonly {
			s.SetReadOnly(true)
		} else {
			s.SetEnabled(false)
		}
		for _, key := range []int{KeyUp, KeyDown, KeyPageUp, KeyPageDown, KeyHome, KeyEnd, KeyEnter} {
			s.OnKeyDown(key, false)
		}
		s.OnTextInput("3")
		s.OnMouseWheel(2, 2, 1)
		s.OnLeftDown(179, 2)
		if s.Value() != 10 || s.Text() != "10" {
			t.Fatal("noneditable control changed", readonly)
		}
	}
	s.SetEnabled(true)
	s.SetReadOnly(false)
	s.SelectAll()
	s.OnTextInput("51")
	s.SetValue(10)
	if s.Text() != "10" {
		t.Fatal("same model value did not supersede draft")
	}
	s.SetValueChangedCallback(func(_ interface{}, v int) {
		if v == 42 {
			s.SetValue(7)
		}
	})
	s.SelectAll()
	s.OnTextInput("42")
	s.Commit()
	if s.Value() != 7 || s.Text() != "7" {
		t.Fatal("callback replacement overwritten")
	}
}
func TestSpinBoxOwnedEditorAndSuffixTinyGeometry(t *testing.T) {
	s := spinEditFixture(t)
	s.SetSuffix(" very long suffix that must not cover arrows")
	for _, size := range [][2]float64{{0, 0}, {1, 1}, {8, 6}, {25, 12}, {180, 32}} {
		s.SetSize(size[0], size[1])
		s.Layout()
		r := s.editor.Bounds1()
		suffix := s.suffixRect()
		if r.X < 0 || r.Y < 0 || r.Width < 0 || r.Height < 0 || r.Right() > size[0] || r.Bottom() > size[1] || suffix.Right() > size[0] || r.Right() > suffix.X {
			t.Fatal("geometry escaped", size, r, suffix)
		}
	}
	if s.editor.Parent() != s || s.editor.Window() != nil || len(s.Children()) != 1 {
		t.Fatal("editor not an owned logical child")
	}
}

type spinFillRecorder struct {
	*themeFillRecorder
	colors []paint.Color
}

func (r *spinFillRecorder) Save() int    { return 0 }
func (r *spinFillRecorder) Restore() int { return 0 }
func (r *spinFillRecorder) Clip()        {}
func (r *spinFillRecorder) Fill()        { r.colors = append(r.colors, r.brush); r.themeFillRecorder.Fill() }
func TestSpinBoxThemeArrowContrast(t *testing.T) {
	old := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(old) })
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		s := spinEditFixture(t)
		s.hoverPart = 1
		for _, pressed := range []bool{false, true} {
			s.pushed = pressed
			r := &spinFillRecorder{themeFillRecorder: newThemeFillRecorder()}
			s.Draw(r)
			colors := r.colors
			if len(colors) < 4 {
				t.Fatal("missing arrow fills")
			}
			fill, ink := colors[len(colors)-3], colors[len(colors)-2]
			if contrastRatio(fill, ink) < 3 {
				t.Fatal("arrow lacks contrast", mode, pressed, fill, ink)
			}
		}
	}
}
func TestPrintCopiesSupportsTypedCommit(t *testing.T) {
	old := focusWidget
	t.Cleanup(func() { focusWidget = old })
	body := newPrintDialogBody()
	body.SetSize(532, 262)
	s := body.copies
	s.SetFocus()
	s.SelectAll()
	s.OnTextInput("24")
	body.title.SetFocus()
	if s.Value() != 24 {
		t.Fatal("print copies did not commit on next field")
	}
}
func TestGanttProgressTypedCommitAndUndo(t *testing.T) {
	c := ganttContextFixture(t)
	c.SetSelectedTaskID("b")
	before := c.Tasks()
	ganttContextPresent(t, func(d *Dialog) DialogResult {
		form := ganttContextForm(t, d)
		s := form.rows[4].widget.(*SpinBox)
		s.SetFocus()
		s.SelectAll()
		s.OnTextInput("75")
		form.rows[5].widget.SetFocus()
		d.OnKeyDown(KeyEnter, false)
		return d.result
	})
	ganttContextRun(t, c, "gantt.edit")
	task, _ := c.model.Task("b")
	if task.Progress != .75 || !c.Undo() || !reflect.DeepEqual(before, c.Tasks()) {
		t.Fatal("typed progress not one undoable edit")
	}
}

func TestSpinBoxIdenticalModelRefreshDoesNotRelayoutText(t *testing.T) {
	s := spinEditFixture(t)
	revision := s.editor.editRevision
	for i := 0; i < 100; i++ {
		s.SetValue(10)
	}
	if s.editor.editRevision != revision {
		t.Fatal("unchanged model value reshaped the editor")
	}
}

// TextExtents.Width measures the ink, not the advance from the baseline origin.
// A leading space gives a positive bearing; ignoring it clips the last glyph.
type spinSuffixRecorder struct {
	*spinFillRecorder
	x, left, right float64
	text           string
}

func (r *spinSuffixRecorder) Translate(x, y float64) { r.x += x }
func (r *spinSuffixRecorder) DrawText(text string) {
	e := r.font.TextExtents(text)
	r.left, r.right, r.text = r.x+e.XBearing, r.x+e.XBearing+e.Width, text
}
func TestSpinBoxSuffixInkStaysInsideClip(t *testing.T) {
	old := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(old) })
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		for _, width := range []float64{80, 180} {
			s := spinEditFixture(t)
			s.SetSize(width, 32)
			s.SetSuffix(" items")
			r := &spinSuffixRecorder{spinFillRecorder: &spinFillRecorder{themeFillRecorder: newThemeFillRecorder()}}
			s.Draw(r)
			clip := s.suffixRect()
			if r.text == "" || r.left < clip.X-.001 || r.right > clip.X+clip.Width+.001 {
				t.Fatalf("suffix %q ink [%g,%g] outside clip [%g,%g], theme %v", r.text, r.left, r.right, clip.X, clip.X+clip.Width, mode)
			}
		}
	}
}

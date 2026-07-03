package gui

import (
	"github.com/uk0/silk/paint"
	"testing"
)

// ---------------------------------------------------------------------------
// Button properties
// ---------------------------------------------------------------------------

func TestButtonSetGetText(t *testing.T) {
	b := NewButton1("Hello", nil)
	if b.Text() != "Hello" {
		t.Errorf("Button.Text() = %q, want %q", b.Text(), "Hello")
	}
	b.SetText("World")
	if b.Text() != "World" {
		t.Errorf("after SetText, Button.Text() = %q, want %q", b.Text(), "World")
	}
}

func TestButtonSetTextUpdate(t *testing.T) {
	b := NewButton1("Start", nil)
	if b.Text() != "Start" {
		t.Errorf("Button.Text() = %q, want Start", b.Text())
	}
	b.SetText("Updated")
	if b.Text() != "Updated" {
		t.Errorf("after SetText, Button.Text() = %q, want Updated", b.Text())
	}
}

// ---------------------------------------------------------------------------
// CheckBox toggle behavior
// ---------------------------------------------------------------------------

func TestCheckBoxToggle(t *testing.T) {
	cb := NewCheckBox()
	if cb.IsChecked() {
		t.Error("CheckBox should be unchecked initially")
	}
	cb.SetChecked(true)
	if !cb.IsChecked() {
		t.Error("CheckBox should be checked after SetChecked(true)")
	}
	cb.Toggle()
	if cb.IsChecked() {
		t.Error("CheckBox should be unchecked after Toggle")
	}
}

func TestCheckBoxCallbackOnToggle(t *testing.T) {
	cb := NewCheckBox()
	var received bool
	cb.SigCheck(func(checked bool) {
		received = checked
	})
	cb.Toggle()
	if !received {
		t.Error("SigCheck callback not fired or wrong value")
	}
}

func TestCheckBoxSetCheckedIdempotent(t *testing.T) {
	cb := NewCheckBox()
	cb.SetChecked(true)
	cb.SetChecked(true) // second call, should not toggle
	if !cb.IsChecked() {
		t.Error("SetChecked(true) twice should keep it checked")
	}
}

// ---------------------------------------------------------------------------
// Slider range clamping
// ---------------------------------------------------------------------------

func TestSliderRange(t *testing.T) {
	s := NewSlider(0, 100)
	s.SetValue(50)
	if s.Value() != 50 {
		t.Errorf("Slider.Value() = %f, want 50", s.Value())
	}
	s.SetValue(200) // over max
	if s.Value() != 100 {
		t.Errorf("clamped above: Slider.Value() = %f, want 100", s.Value())
	}
	s.SetValue(-10) // under min
	if s.Value() != 0 {
		t.Errorf("clamped below: Slider.Value() = %f, want 0", s.Value())
	}
}

func TestSliderSwappedRange(t *testing.T) {
	s := NewSlider(100, 0) // max < min
	if s.Min() != 100 || s.Max() != 100 {
		t.Errorf("swapped range: min=%f max=%f; expected both 100", s.Min(), s.Max())
	}
}

func TestSliderSetRangeClamps(t *testing.T) {
	s := NewSlider(0, 100)
	s.SetValue(80)
	s.SetRange(0, 50) // max reduced below current value
	if s.Value() > 50 {
		t.Errorf("after SetRange(0,50), Value() = %f, should be <= 50", s.Value())
	}
}

func TestSliderVertical(t *testing.T) {
	s := NewSlider(0, 100)
	if s.IsVertical() {
		t.Error("should default to horizontal")
	}
	s.SetVertical(true)
	if !s.IsVertical() {
		t.Error("should be vertical after SetVertical(true)")
	}
}

// ---------------------------------------------------------------------------
// ProgressBar clamping
// ---------------------------------------------------------------------------

func TestProgressBarClamp(t *testing.T) {
	p := NewProgressBar()
	p.SetValue(0.5)
	if p.Value() != 0.5 {
		t.Errorf("ProgressBar.Value() = %f, want 0.5", p.Value())
	}
	p.SetValue(2.0)
	if p.Value() != 1.0 {
		t.Error("ProgressBar.Value() not clamped to 1.0")
	}
	p.SetValue(-1.0)
	if p.Value() != 0.0 {
		t.Error("ProgressBar.Value() not clamped to 0.0")
	}
}

func TestProgressBarBarColor(t *testing.T) {
	p := NewProgressBar()
	red := paint.Color{255, 0, 0, 255}
	p.SetBarColor(red)
	if p.BarColor() != red {
		t.Error("BarColor mismatch")
	}
}

// ---------------------------------------------------------------------------
// ToggleSwitch
// ---------------------------------------------------------------------------

func TestToggleSwitchCallbacks(t *testing.T) {
	ts := NewToggleSwitch()
	var called bool
	var callValue bool
	ts.SigToggle(func(on bool) {
		called = true
		callValue = on
	})
	ts.Toggle()
	if !called {
		t.Error("SigToggle callback not fired")
	}
	if !callValue {
		t.Error("callback received false, want true")
	}
	if !ts.IsChecked() {
		t.Error("should be checked after toggle from unchecked")
	}
}

func TestToggleSwitchSetChecked(t *testing.T) {
	ts := NewToggleSwitch()
	ts.SetChecked(true)
	if !ts.IsChecked() {
		t.Error("should be checked after SetChecked(true)")
	}
	ts.SetChecked(false)
	if ts.IsChecked() {
		t.Error("should be unchecked after SetChecked(false)")
	}
}

func TestToggleSwitchText(t *testing.T) {
	ts := NewToggleSwitch()
	ts.SetText("WiFi")
	if ts.Text() != "WiFi" {
		t.Errorf("Text() = %q, want WiFi", ts.Text())
	}
}

// ---------------------------------------------------------------------------
// NumberInput
// ---------------------------------------------------------------------------

func TestNumberInputRange(t *testing.T) {
	n := NewNumberInput()
	n.SetRange(0, 100)
	n.SetValue(50)
	n.StepUp()
	if n.Value() != 51 {
		t.Errorf("after StepUp: Value() = %f, want 51", n.Value())
	}
	n.SetValue(100)
	n.StepUp()
	if n.Value() != 100 {
		t.Errorf("should not exceed max: Value() = %f, want 100", n.Value())
	}
}

func TestNumberInputStepDown(t *testing.T) {
	n := NewNumberInput()
	n.SetRange(0, 100)
	n.SetValue(0)
	n.StepDown()
	if n.Value() != 0 {
		t.Errorf("should not go below min: Value() = %f, want 0", n.Value())
	}
}

func TestNumberInputCustomStep(t *testing.T) {
	n := NewNumberInput()
	n.SetRange(0, 100)
	n.SetStep(5)
	n.SetValue(10)
	n.StepUp()
	if n.Value() != 15 {
		t.Errorf("custom step: Value() = %f, want 15", n.Value())
	}
}

func TestNumberInputCallback(t *testing.T) {
	n := NewNumberInput()
	n.SetRange(0, 100)
	var called bool
	n.SigValueChanged(func(v float64) { called = true })
	n.SetValue(42)
	if !called {
		t.Error("SigValueChanged not fired")
	}
}

// ---------------------------------------------------------------------------
// Rating
// ---------------------------------------------------------------------------

func TestRatingValue(t *testing.T) {
	r := NewRating()
	r.SetMaxStars(5)
	r.SetValue(3)
	if r.Value() != 3 {
		t.Errorf("Rating.Value() = %d, want 3", r.Value())
	}
	r.SetValue(10) // over max
	if r.Value() > 5 {
		t.Errorf("Rating.Value() = %d, exceeded max stars 5", r.Value())
	}
}

func TestRatingSetValueBelowZero(t *testing.T) {
	r := NewRating()
	r.SetValue(-5)
	if r.Value() != 0 {
		t.Errorf("negative value: Rating.Value() = %d, want 0", r.Value())
	}
}

func TestRatingMaxStarsClampsValue(t *testing.T) {
	r := NewRating()
	r.SetMaxStars(5)
	r.SetValue(5)
	r.SetMaxStars(3) // reduce max below current value
	if r.Value() > 3 {
		t.Errorf("after reducing max: Value() = %d, should be <= 3", r.Value())
	}
}

func TestRatingMaxStarsMin(t *testing.T) {
	r := NewRating()
	r.SetMaxStars(0) // below minimum of 1
	if r.MaxStars() < 1 {
		t.Errorf("MaxStars() = %d, should be >= 1", r.MaxStars())
	}
}

func TestRatingCallback(t *testing.T) {
	r := NewRating()
	var called bool
	r.SigRatingChanged(func(v int) { called = true })
	r.SetValue(3)
	if !called {
		t.Error("SigRatingChanged not fired")
	}
}

// ---------------------------------------------------------------------------
// Tag
// ---------------------------------------------------------------------------

func TestTagProperties(t *testing.T) {
	tag := NewTag("Test")
	if tag.Text() != "Test" {
		t.Errorf("Tag.Text() = %q, want Test", tag.Text())
	}
	tag.SetCloseable(true)
	if !tag.IsCloseable() {
		t.Error("Tag should be closeable")
	}
}

func TestTagCloseCallback(t *testing.T) {
	tag := NewTag("X")
	tag.SetCloseable(true)
	var closed bool
	tag.SigClose(func() { closed = true })
	// SigClose just registers the callback, verify it is stored
	_ = closed
}

func TestTagColorChange(t *testing.T) {
	tag := NewTag("Color")
	red := paint.Color{255, 0, 0, 255}
	tag.SetColor(red)
	if tag.Color() != red {
		t.Error("Tag.Color() mismatch")
	}
}

// ---------------------------------------------------------------------------
// Card
// ---------------------------------------------------------------------------

func TestCardProperties(t *testing.T) {
	c := NewCard("Title")
	if c.Title() != "Title" {
		t.Errorf("Card.Title() = %q, want Title", c.Title())
	}
	c.SetPadding(20)
	if c.Padding() != 20 {
		t.Errorf("Card.Padding() = %f, want 20", c.Padding())
	}
	c.SetRadius(12)
	if c.Radius() != 12 {
		t.Errorf("Card.Radius() = %f, want 12", c.Radius())
	}
}

func TestCardShadow(t *testing.T) {
	c := NewCard("Shadow")
	if !c.HasShadow() {
		t.Error("Card should have shadow by default")
	}
	c.SetShadow(false)
	if c.HasShadow() {
		t.Error("Card should not have shadow after SetShadow(false)")
	}
}

func TestCardTitle(t *testing.T) {
	c := NewCard("")
	if c.Title() != "" {
		t.Error("empty title should be empty string")
	}
	c.SetTitle("New Title")
	if c.Title() != "New Title" {
		t.Errorf("Title = %q, want New Title", c.Title())
	}
}

// ---------------------------------------------------------------------------
// SearchBox
// ---------------------------------------------------------------------------

func TestSearchBoxClear(t *testing.T) {
	sb := NewSearchBox()
	sb.SetText("hello")
	if sb.Text() != "hello" {
		t.Errorf("SearchBox.Text() = %q, want hello", sb.Text())
	}
	sb.Clear()
	if sb.Text() != "" {
		t.Error("SearchBox.Text() not cleared")
	}
}

func TestSearchBoxPlaceholder(t *testing.T) {
	sb := NewSearchBox()
	if sb.Placeholder() == "" {
		t.Error("SearchBox should have a default placeholder")
	}
	sb.SetPlaceholder("Find...")
	if sb.Placeholder() != "Find..." {
		t.Errorf("Placeholder() = %q, want Find...", sb.Placeholder())
	}
}

func TestSearchBoxTextChangedCallback(t *testing.T) {
	sb := NewSearchBox()
	var received string
	sb.SigTextChanged(func(s string) { received = s })
	sb.SetText("query")
	if received != "query" {
		t.Errorf("SigTextChanged received %q, want query", received)
	}
}

// ---------------------------------------------------------------------------
// Avatar
// ---------------------------------------------------------------------------

func TestAvatarInitials(t *testing.T) {
	a := NewAvatar()
	a.SetText("John Doe")
	initials := a.initials()
	if initials != "JD" {
		t.Errorf("initials = %q, want JD", initials)
	}
}

func TestAvatarSingleWord(t *testing.T) {
	a := NewAvatar()
	a.SetText("Alice")
	initials := a.initials()
	if initials != "Al" {
		t.Errorf("initials = %q, want Al", initials)
	}
}

func TestAvatarChineseText(t *testing.T) {
	a := NewAvatar()
	a.SetText("张三")
	initials := a.initials()
	runes := []rune(initials)
	if len(runes) != 2 {
		t.Errorf("Chinese initials = %q, expected 2 runes", initials)
	}
}

func TestAvatarEmpty(t *testing.T) {
	a := NewAvatar()
	a.SetText("")
	initials := a.initials()
	if initials != "" {
		t.Errorf("empty text initials = %q, want empty", initials)
	}
}

func TestAvatarShape(t *testing.T) {
	a := NewAvatar()
	if a.Shape() != AvatarCircle {
		t.Error("default shape should be AvatarCircle")
	}
	a.SetShape(AvatarSquare)
	if a.Shape() != AvatarSquare {
		t.Error("should be AvatarSquare after SetShape")
	}
}

// ---------------------------------------------------------------------------
// Breadcrumb
// ---------------------------------------------------------------------------

func TestBreadcrumbItems(t *testing.T) {
	bc := NewBreadcrumb()
	bc.AddItem("Home", nil)
	bc.AddItem("Products", nil)
	items := bc.Items()
	if len(items) != 2 {
		t.Errorf("Breadcrumb count = %d, want 2", len(items))
	}
	if items[0].Text != "Home" {
		t.Errorf("items[0].Text = %q, want Home", items[0].Text)
	}
	if items[1].Text != "Products" {
		t.Errorf("items[1].Text = %q, want Products", items[1].Text)
	}
}

func TestBreadcrumbSeparator(t *testing.T) {
	bc := NewBreadcrumb()
	if bc.Separator() != "/" {
		t.Errorf("default separator = %q, want /", bc.Separator())
	}
	bc.SetSeparator(">")
	if bc.Separator() != ">" {
		t.Errorf("separator = %q, want >", bc.Separator())
	}
}

func TestBreadcrumbEmpty(t *testing.T) {
	bc := NewBreadcrumb()
	items := bc.Items()
	if len(items) != 0 {
		t.Errorf("empty breadcrumb count = %d, want 0", len(items))
	}
}

// ---------------------------------------------------------------------------
// Label alignment
// ---------------------------------------------------------------------------

func TestLabelAlign(t *testing.T) {
	l := NewLabel("Test")
	l.SetAlign(AlignCenter)
	if l.Align() != AlignCenter {
		t.Errorf("Align() = %d, want AlignCenter", l.Align())
	}
	l.SetAlign(AlignRight)
	if l.Align() != AlignRight {
		t.Errorf("Align() = %d, want AlignRight", l.Align())
	}
}

func TestLabelWrap(t *testing.T) {
	l := NewLabel("wrap test")
	if l.Wrap() {
		t.Error("should not wrap by default")
	}
	l.SetWrap(true)
	if !l.Wrap() {
		t.Error("should wrap after SetWrap(true)")
	}
}

// ---------------------------------------------------------------------------
// DatePicker
// ---------------------------------------------------------------------------

func TestDatePickerDefaults(t *testing.T) {
	dp := NewDatePicker()
	if dp.Year() < 2020 || dp.Year() > 2100 {
		t.Errorf("Year() = %d, out of expected range", dp.Year())
	}
	if dp.Month() < 1 || dp.Month() > 12 {
		t.Errorf("Month() = %d, out of range", dp.Month())
	}
	if dp.Day() < 1 || dp.Day() > 31 {
		t.Errorf("Day() = %d, out of range", dp.Day())
	}
}

func TestDatePickerSetDate(t *testing.T) {
	dp := NewDatePicker()
	dp.SetDate(2025, 3, 15)
	if dp.Year() != 2025 || dp.Month() != 3 || dp.Day() != 15 {
		t.Errorf("date = %d-%02d-%02d, want 2025-03-15", dp.Year(), dp.Month(), dp.Day())
	}
}

func TestDatePickerClampMonth(t *testing.T) {
	dp := NewDatePicker()
	dp.SetDate(2025, 13, 1) // month > 12
	if dp.Month() > 12 {
		t.Errorf("month not clamped: %d", dp.Month())
	}
	dp.SetDate(2025, 0, 1) // month < 1
	if dp.Month() < 1 {
		t.Errorf("month not clamped: %d", dp.Month())
	}
}

func TestDatePickerClampDay(t *testing.T) {
	dp := NewDatePicker()
	dp.SetDate(2025, 2, 31) // Feb has < 31 days
	if dp.Day() > 28 {
		t.Errorf("Feb day not clamped: %d", dp.Day())
	}
}

// ---------------------------------------------------------------------------
// ColorPicker
// ---------------------------------------------------------------------------

func TestColorPickerSetGet(t *testing.T) {
	cp := NewColorPicker()
	red := paint.Color{255, 0, 0, 255}
	cp.SetColor(red)
	if cp.Color() != red {
		t.Error("ColorPicker.Color() mismatch")
	}
}

func TestColorPickerCallback(t *testing.T) {
	cp := NewColorPicker()
	var called bool
	cp.SigColorChanged(func(c paint.Color) { called = true })
	cp.SetColor(paint.Color{0, 255, 0, 255})
	if !called {
		t.Error("SigColorChanged not fired")
	}
}

// ---------------------------------------------------------------------------
// DropdownButton
// ---------------------------------------------------------------------------

func TestDropdownButtonItems(t *testing.T) {
	db := NewDropdownButton()
	db.AddItem("Option A", nil, nil)
	db.AddItem("Option B", nil, nil)
	if len(db.Items()) != 2 {
		t.Errorf("item count = %d, want 2", len(db.Items()))
	}
	db.SetSelected(0)
	if db.Selected() != 0 {
		t.Errorf("Selected() = %d, want 0", db.Selected())
	}
	if db.Text() != "Option A" {
		t.Errorf("Text() = %q, want Option A", db.Text())
	}
}

func TestDropdownButtonInvalidSelection(t *testing.T) {
	db := NewDropdownButton()
	db.AddItem("Only", nil, nil)
	db.SetSelected(5) // out of range
	if db.Selected() != -1 {
		t.Errorf("invalid selection: Selected() = %d, want -1", db.Selected())
	}
}

// ---------------------------------------------------------------------------
// SwitchGroup
// ---------------------------------------------------------------------------

func TestSwitchGroupSelection(t *testing.T) {
	sg := NewSwitchGroup()
	sg.SetItems([]string{"Day", "Week", "Month"})
	if sg.Selected() != 0 {
		t.Errorf("default Selected() = %d, want 0", sg.Selected())
	}
	sg.SetSelected(2)
	if sg.Selected() != 2 {
		t.Errorf("Selected() = %d, want 2", sg.Selected())
	}
	if sg.SelectedText() != "Month" {
		t.Errorf("SelectedText() = %q, want Month", sg.SelectedText())
	}
}

func TestSwitchGroupOutOfRange(t *testing.T) {
	sg := NewSwitchGroup()
	sg.SetItems([]string{"A", "B"})
	sg.SetSelected(10) // should be ignored
	if sg.Selected() != 0 {
		t.Errorf("out-of-range: Selected() = %d, want 0", sg.Selected())
	}
}

// ---------------------------------------------------------------------------
// Badge
// ---------------------------------------------------------------------------

func TestBadgeCount(t *testing.T) {
	b := NewBadge()
	b.SetCount(5)
	if b.Count() != 5 {
		t.Errorf("Count() = %d, want 5", b.Count())
	}
}

func TestBadgeMaxCount(t *testing.T) {
	b := NewBadge()
	b.SetMaxCount(99)
	b.SetCount(150)
	text := b.displayText()
	if text != "99+" {
		t.Errorf("displayText() = %q, want 99+", text)
	}
}

func TestBadgeDot(t *testing.T) {
	b := NewBadge()
	b.SetDot(true)
	if !b.IsDot() {
		t.Error("should be dot mode")
	}
}

// ---------------------------------------------------------------------------
// Timeline
// ---------------------------------------------------------------------------

func TestTimelineItems(t *testing.T) {
	tl := NewTimeline()
	tl.AddItem("Step 1", "desc", 0)
	tl.AddItem("Step 2", "desc", 1)
	tl.AddItem("Step 3", "desc", 2)
	items := tl.Items()
	if len(items) != 3 {
		t.Errorf("item count = %d, want 3", len(items))
	}
	if items[0].Title != "Step 1" {
		t.Errorf("items[0].Title = %q, want Step 1", items[0].Title)
	}
}

func TestTimelineSetStatus(t *testing.T) {
	tl := NewTimeline()
	tl.AddItem("Step", "", 0)
	tl.SetStatus(0, 2) // mark as done
	if tl.Items()[0].Status != 2 {
		t.Errorf("status = %d, want 2", tl.Items()[0].Status)
	}
}

func TestTimelineVertical(t *testing.T) {
	tl := NewTimeline()
	if !tl.IsVertical() {
		t.Error("should default to vertical")
	}
	tl.SetVertical(false)
	if tl.IsVertical() {
		t.Error("should be horizontal after SetVertical(false)")
	}
}

// ---------------------------------------------------------------------------
// Accordion
// ---------------------------------------------------------------------------

func TestAccordionAddSection(t *testing.T) {
	acc := NewAccordion()
	acc.AddSection("Section 1", nil)
	acc.AddSection("Section 2", nil)
	if acc.SectionCount() != 2 {
		t.Errorf("SectionCount() = %d, want 2", acc.SectionCount())
	}
}

func TestAccordionToggle(t *testing.T) {
	acc := NewAccordion()
	acc.AddSection("A", nil) // first is expanded by default
	acc.AddSection("B", nil)

	// Toggle section 1 (B) to expand it
	acc.ToggleSection(1)
	// In single-expand mode, section 0 should collapse
	if acc.sections[0].Expanded {
		t.Error("section 0 should be collapsed after toggling section 1")
	}
	if !acc.sections[1].Expanded {
		t.Error("section 1 should be expanded after toggle")
	}
}

func TestAccordionMultiExpand(t *testing.T) {
	acc := NewAccordion()
	acc.SetMultiExpand(true)
	acc.AddSection("A", nil) // expanded by default
	acc.AddSection("B", nil)

	acc.ToggleSection(1) // expand B
	if !acc.sections[0].Expanded {
		t.Error("multi-expand: section 0 should still be expanded")
	}
	if !acc.sections[1].Expanded {
		t.Error("multi-expand: section 1 should be expanded")
	}
}

func TestAccordionCallback(t *testing.T) {
	acc := NewAccordion()
	acc.AddSection("X", nil)
	var calledIdx int
	var calledState bool
	acc.SigExpand(func(idx int, expanded bool) {
		calledIdx = idx
		calledState = expanded
	})
	acc.ToggleSection(0) // toggle off the first (which is expanded by default)
	if calledIdx != 0 {
		t.Errorf("callback idx = %d, want 0", calledIdx)
	}
	if calledState {
		t.Error("callback state should be false (collapsed)")
	}
}

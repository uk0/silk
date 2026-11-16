package gui

import (
	"math"
	"strings"
	"testing"
)

// Exercise the same first layout used by the real unsaved-changes modal, with
// integer client dimensions as delivered by WM_SIZE. Merely retaining the text
// in a Label is not enough: every visual row must receive height and be drawn.
func TestDialogWrappedLabelFirstLayoutKeepsFullText(t *testing.T) {
	for _, text := range []string{
		"source.txt has unsaved changes. Save before quitting?",
		strings.Repeat("A very long project path / 中文目录 / file.go needs attention. ", 8),
	} {
		dialog := NewDialog("Unsaved changes", nil)
		content := NewVBox()
		label := NewLabel(text)
		label.SetWrap(true)
		content.AddWidget(label)
		dialog.SetContent(content)
		dialog.AddButton("Save All", DialogOK)
		dialog.AddButton("Discard", DialogNo)
		dialog.AddButton("Cancel", DialogCancel)
		w, h := dialog.dialogSize()
		dialog.Form.SetSize(math.Floor(w), math.Floor(h))
		dialog.Layout()
		p := newLabelPaintRecorder()
		label.Draw(p)
		if got := strings.Join(p.texts, ""); got != text {
			t.Errorf("first modal layout clipped text: size %.1fx%.1f label %.1fx%.1f rows=%d drawn=%q want=%q", w, h, label.Width(), label.Height(), len(label.lines), got, text)
		}
		if label.Height() < label.SizeHints().Height {
			t.Errorf("allocated %.1f high before wrapping, need %.1f", label.Height(), label.SizeHints().Height)
		}
	}
}

type widthMeasureProbe struct {
	Widget
	naturalCalls, widthCalls, resizes int
	hint                              SizeHints
	height                            float64
}

func (p *widthMeasureProbe) SizeHints() SizeHints { p.naturalCalls++; return p.hint }
func (p *widthMeasureProbe) HeightForWidth(float64) float64 {
	p.widthCalls++
	return p.height
}
func (p *widthMeasureProbe) OnResize() { p.resizes++ }

func TestMeasureForWidthFallbackAndInvalidInput(t *testing.T) {
	if got := MeasureForWidth(nil, 100); got != (SizeHints{}) {
		t.Fatal(got)
	}
	p := &widthMeasureProbe{hint: SizeHints{Width: 80, Height: 12}, height: 35}
	p.Init(p)
	p.SetBounds(3, 4, 90, 45)
	for _, invalid := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := MeasureForWidth(p, invalid); got != p.hint {
			t.Fatalf("invalid width %v did not retain natural hints: %+v", invalid, got)
		}
	}
	if p.widthCalls != 0 {
		t.Fatal("unconstrained width called legacy HeightForWidth")
	}
	for _, height := range []float64{-1, math.NaN(), math.Inf(1)} {
		p.height = height
		if got := MeasureForWidth(p, 20); got != p.hint {
			t.Fatalf("invalid custom height propagated: %+v", got)
		}
	}
	p.height = 35
	if got := MeasureForWidth(p, 20); got.Height != 35 || got.Width != p.hint.Width {
		t.Fatal(got)
	}
	if x, y, w, h := p.Bounds(); x != 3 || y != 4 || w != 90 || h != 45 || p.resizes != 1 || p.Window() != nil {
		t.Fatal("measurement changed geometry, dispatched Resize or attached a window")
	}
}

func TestLabelMeasureForWidthPreservesStateAndCaches(t *testing.T) {
	font := &labelCountingFont{Font: Theme().Font.Dup()}
	label := NewLabel("中文 combining e\u0301 long label? ")
	label.SetWrap(true)
	label.SetFont(font)
	label.SetBounds(3, 4, 200, 60)
	narrow := MeasureForWidth(label, 32)
	wide := MeasureForWidth(label, 200)
	if narrow.Height <= wide.Height || label.Width() != 200 || label.Height() != 60 || label.Window() != nil {
		t.Fatal("not a pure width-dependent measurement", narrow, wide)
	}
	before := font.measurements
	for i := 0; i < 100; i++ {
		if MeasureForWidth(label, 200) != wide || label.HeightForWidth(200) != wide.Height {
			t.Fatal("inconsistent cached measurement")
		}
	}
	if font.measurements != before {
		t.Fatal("unchanged width queries reshaped text")
	}
	label.SetWrap(false)
	if MeasureForWidth(label, 32) != MeasureForWidth(label, 200) {
		t.Fatal("nonwrapped label depends on target width")
	}
}

func TestVBoxWidthMeasureNestedSingleTraversalAndConstraints(t *testing.T) {
	leaf := &widthMeasureProbe{hint: SizeHints{Width: 80, Height: 12, MinHeight: 20, MaxHeight: 50, Policy: GrowHorizontal}, height: 100}
	leaf.Init(leaf)
	var root IWidget = leaf
	for i := 0; i < 64; i++ {
		box := NewVBox()
		box.AddWidget(root)
		root = box
	}
	leaf.naturalCalls, leaf.widthCalls, leaf.resizes = 0, 0, 0
	if got := MeasureForWidth(root, 120); got.Height != 50 {
		t.Fatal("nested constraint lost", got)
	}
	if leaf.naturalCalls != 1 || leaf.widthCalls != 1 || leaf.resizes != 0 {
		t.Fatal("measurement repeated subtree walks or resized the leaf", leaf.naturalCalls, leaf.widthCalls, leaf.resizes)
	}
	box := NewVBox()
	box.SetPadding(Padding{L: 5, R: 7, T: 3, B: 4})
	box.SetSpacing(6)
	box.AddWidget(root)
	hidden := NewLabel(strings.Repeat("hidden ", 100))
	hidden.SetWrap(true)
	box.AddWidget(hidden)
	hidden.SetVisible(false)
	if got := MeasureForWidth(box, 132); got.Height != 57 {
		t.Fatal("hidden child or padding changed constrained height", got)
	}
	box.SetBounds(0, 0, 132, 57)
	if root.Height() != 50 {
		t.Fatal("measured and installed heights disagree", root.Height())
	}
	box.SetLayoutEnabled(false)
	if got := MeasureForWidth(box, 5); got.Width != 132 || got.Height != 57 {
		t.Fatal("measurement rearranged a free-positioned designer container", got)
	}
}

func TestHBoxWidthMeasureShrinksOnlyWrapAndKeepsWarmCache(t *testing.T) {
	font := &labelCountingFont{Font: Theme().Font.Dup()}
	row := NewHBox()
	row.SetSpacing(8)
	row.SetPadding(Padding{L: 5, R: 7, T: 3, B: 4})
	fixed := &widthMeasureProbe{hint: SizeHints{Width: 40, Height: 20}, height: 20}
	fixed.Init(fixed)
	label := NewLabel(strings.Repeat("中文 long wrapped text? ", 6))
	label.SetWrap(true)
	label.SetFont(font)
	row.AddWidget(fixed)
	row.AddWidget(label)
	hint := MeasureForWidth(row, 220)
	if hint.Height <= 30 || fixed.resizes != 0 || label.Width() != 0 {
		t.Fatal("HBox did not purely measure its allocated text column", hint)
	}
	row.SetBounds(0, 0, 220, hint.Height)
	if fixed.Width() != 40 || math.Abs(label.Width()-160) > 1e-7 || label.Height()+1e-7 < label.SizeHints().Height {
		t.Fatal("HBox allocation and measurement disagree", fixed.Bounds1(), label.Bounds1())
	}
	p := newLabelPaintRecorder()
	label.Draw(p)
	if strings.Join(p.texts, "") != label.Text() {
		t.Fatal("wrapped row dropped text")
	}
	warm := font.measurements
	for i := 0; i < 100; i++ {
		row.Layout()
		label.Draw(newLabelPaintRecorder())
		if math.Abs(label.Width()-160) > 1e-7 {
			t.Fatal("successive layouts drifted toward wrapped ink width")
		}
	}
	if font.measurements != warm {
		t.Fatal("natural/allocated width queries defeated the warm text cache", warm, font.measurements)
	}
	label.SetWrap(false)
	row.Layout()
	if label.Width() <= 160 || fixed.Width() != 40 {
		t.Fatal("a nonshrinkable label or fixed sibling was silently compressed")
	}
}

func assertWidthMeasuredLabel(t *testing.T, label *Label) {
	t.Helper()
	p := newLabelPaintRecorder()
	label.Draw(p)
	if strings.Join(p.texts, "") != label.Text() || label.Height()+1e-7 < label.SizeHints().Height {
		t.Fatal("installed bounds do not draw every measured row", label.Bounds1(), p.texts)
	}
}

func TestWidthMeasureContainersInstallCompleteWrappedText(t *testing.T) {
	constructors := map[string]func(*Label) IWidget{
		"group": func(label *Label) IWidget {
			g := NewGroupBox("Industrial input")
			g.SetContent(label)
			return g
		},
		"form": func(label *Label) IWidget {
			f := NewFormLayout()
			f.SetSpacing(6)
			f.AddRow("Description", label)
			return f
		},
		"grid": func(label *Label) IWidget {
			g := NewGridLayout()
			g.SetSpacing(6)
			g.AddWidget(label, 0, 0)
			g.AddWidget(NewButton1("Next row", nil), 1, 0)
			return g
		},
		"flow": func(label *Label) IWidget {
			f := NewFlexWrap()
			f.AddWidget(label)
			f.AddWidget(NewButton1("Next chip", nil))
			return f
		},
	}
	for name, makeContainer := range constructors {
		t.Run(name, func(t *testing.T) {
			label := NewLabel(strings.Repeat("中文 industrial point description? ", 4))
			label.SetWrap(true)
			container := makeContainer(label)
			host := NewVBox()
			host.SetSpacing(8)
			host.AddWidget(container)
			footer := NewButton1("Outside container", nil)
			host.AddWidget(footer)
			before := label.Bounds1()
			hint := MeasureForWidth(host, 240)
			if label.Bounds1() != before || label.Window() != nil {
				t.Fatal("container measurement installed probe bounds")
			}
			host.SetBounds(0, 0, 240, hint.Height)
			assertWidthMeasuredLabel(t, label)
			if footer.Y()+1e-7 < container.Y()+container.Height()+8 || footer.Y()+footer.Height() > host.Height()+1e-7 {
				t.Fatal("wrapped child covers or displaces the following footer", footer.Bounds1(), container.Bounds1())
			}
		})
	}
}

func TestGridWidthMeasureSpanFixedTrackAndForeignOwnership(t *testing.T) {
	grid := NewGridLayout()
	grid.SetSpacing(6)
	grid.SetColumnWidth(1, 40)
	grid.SetRowHeight(0, 12)
	label := NewLabel(strings.Repeat("Span 中文 long text? ", 7))
	label.SetWrap(true)
	grid.AddWidgetSpan(label, 0, 0, 2, 1)
	marker := &widthMeasureProbe{hint: SizeHints{Width: 40, Height: 1}, height: 1}
	marker.Init(marker)
	grid.AddWidget(marker, 0, 1)
	footer := NewButton1("Following row", nil)
	grid.AddWidget(footer, 2, 0)
	hint := MeasureForWidth(grid, 180)
	grid.SetBounds(0, 0, 180, hint.Height)
	assertWidthMeasuredLabel(t, label)
	if marker.Height() != 12 || footer.Y()+1e-7 < label.Y()+label.Height()+6 {
		t.Fatal("span lost its minimum or overwrote a fixed track", marker.Bounds1(), label.Bounds1(), footer.Bounds1())
	}
	foreign := NewVBox()
	label.SetParent(foreign)
	label.SetBounds(7, 8, 90, 100)
	moved := label.Bounds1()
	grid.Layout()
	_ = MeasureForWidth(grid, 30)
	grid.SetColumnWidth(0, 10)
	if label.Parent() != foreign || label.Bounds1() != moved {
		t.Fatal("stale grid cell moved another parent's widget", label.Bounds1())
	}
}

func TestGridRegistrationBoundsAndDuplicateMove(t *testing.T) {
	grid := NewGridLayout()
	label := NewLabel("move")
	for _, bad := range [][2]int{{-1, 0}, {0, -1}, {4096, 0}, {0, 4096}} {
		grid.AddWidget(label, bad[0], bad[1])
	}
	grid.AddWidget(nil, 0, 0)
	if len(grid.cells) != 0 || label.Parent() != nil {
		t.Fatal("invalid grid registration adopted a widget")
	}
	grid.AddWidget(label, 0, 0)
	grid.AddWidgetSpan(label, 4095, 4095, 100, 100)
	if len(grid.cells) != 1 || grid.cells[0].rowSpan != 1 || grid.cells[0].colSpan != 1 || grid.cells[0].row != 4095 {
		t.Fatal("duplicate registration or oversized span", grid.cells)
	}
	empty := NewGridLayout()
	empty.SetColumnWidth(-1, 1)
	empty.SetColumnWidth(4096, 1)
	empty.SetRowHeight(-1, 1)
	empty.SetRowHeight(4096, 1)
	if len(empty.colWidths) != 0 || len(empty.rowHeights) != 0 {
		t.Fatal("invalid track index allocated metadata")
	}
}

func TestWidthMeasureFormAndGroupDoNotActOnMovedChildren(t *testing.T) {
	other := NewVBox()
	field := NewLabel("moved")
	form := NewFormLayout()
	form.AddRow("Name", field)
	field.SetParent(other)
	field.SetBounds(1, 2, 3, 4)
	moved := field.Bounds1()
	form.SetBounds(0, 0, 400, 200)
	_ = MeasureForWidth(form, 90)
	if field.Bounds1() != moved {
		t.Fatal("stale form row moved another parent's field")
	}
	group := NewGroupBox("Group")
	group.SetContent(field)
	field.SetParent(other)
	field.SetBounds(1, 2, 3, 4)
	group.SetBounds(0, 0, 400, 200)
	_ = MeasureForWidth(group, 90)
	group.SetContent(NewLabel("replacement"))
	if field.Parent() != other || field.Bounds1() != moved {
		t.Fatal("group moved or detached an independently reparented child")
	}
}

type widthHintDerivedLabel struct{ Label }

func (p *widthHintDerivedLabel) SizeHints() SizeHints {
	hint := p.Label.SizeHints()
	hint.Width, hint.MinWidth, hint.MaxWidth = 123, 90, 300
	hint.MinHeight, hint.MaxHeight, hint.Stretch = 40, 500, 3
	hint.Policy = GrowHorizontal | GrowVertical
	return hint
}

type widthHintDerivedGroup struct{ GroupBox }

func (p *widthHintDerivedGroup) SizeHints() SizeHints {
	hint := p.GroupBox.SizeHints()
	hint.Stretch = 2
	return hint
}

func TestWidthMeasurePreservesDerivedSDKSizeHints(t *testing.T) {
	label := new(widthHintDerivedLabel)
	label.Init(label)
	label.SetText(strings.Repeat("derived 中文 text ", 5))
	label.SetWrap(true)
	hint := MeasureForWidth(label, 80)
	if hint.Width != 123 || hint.MinWidth != 90 || hint.MaxWidth != 300 || hint.MinHeight != 40 || hint.MaxHeight != 500 || hint.Stretch != 3 || hint.Policy != GrowHorizontal|GrowVertical {
		t.Fatal("promoted built-in width method erased derived SizeHints", hint)
	}
	if hint.Height <= label.Label.SizeHints().Height {
		t.Fatal("inherited measurement did not wrap at its requested width")
	}
	if natural := MeasureForWidth(label, 0); natural != label.SizeHints() {
		t.Fatal("unconstrained query bypassed derived natural hints")
	}
	leaf := &widthMeasureProbe{hint: SizeHints{Width: 80, Height: 12}, height: 100}
	leaf.Init(leaf)
	var root IWidget = leaf
	for i := 0; i < 64; i++ {
		group := new(widthHintDerivedGroup)
		group.Init(group)
		group.SetContent(root)
		root = group
	}
	leaf.naturalCalls, leaf.widthCalls, leaf.resizes = 0, 0, 0
	if got := MeasureForWidth(root, 5000); got.Stretch != 2 || got.Height <= 100 {
		t.Fatal("derived nested group lost its geometry or policy", got)
	}
	if leaf.naturalCalls > 66 || leaf.widthCalls != 1 || leaf.resizes != 0 {
		t.Fatal("derived natural/allocated query became exponential or mutated bounds", leaf.naturalCalls, leaf.widthCalls, leaf.resizes)
	}
}

func TestLabelMeasuredHeightContainsFallbackInk(t *testing.T) {
	for _, text := range []string{"中文😀\n👩\u200d💻 e\u0301?", "A😀", "🇨🇳 中文 1\ufe0f\u20e3"} {
		label := NewLabel(text)
		label.SetWrap(true)
		hint := MeasureForWidth(label, 64)
		label.SetBounds(0, 0, 64, hint.Height)
		p := newLabelPaintRecorder()
		label.Draw(p)
		for i, row := range p.texts {
			ext := label.effectiveFont().TextExtents(row)
			top := p.positions[i][1] + ext.YBearing
			bottom := top + ext.Height
			if top < -1e-7 || bottom > label.Height()+1e-7 {
				t.Errorf("fallback ink clipped despite measured height: %q ink [%.2f,%.2f] label %.2f", row, top, bottom, label.Height())
			}
		}
	}
}

func BenchmarkWidthMeasureNested(b *testing.B) {
	for _, orientation := range []string{"vertical", "horizontal"} {
		b.Run(orientation, func(b *testing.B) {
			label := NewLabel(strings.Repeat("cached 中文 description ", 10))
			label.SetWrap(true)
			var root IWidget = label
			for i := 0; i < 32; i++ {
				if orientation == "vertical" {
					box := NewVBox()
					box.AddWidget(root)
					root = box
				} else {
					box := NewHBox()
					box.AddWidget(root)
					root = box
				}
			}
			_ = MeasureForWidth(root, 160)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = MeasureForWidth(root, 160)
			}
		})
	}
}

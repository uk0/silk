package ged

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func TestOutlineSwitchAndEditRetireOldSemanticSymbols(t *testing.T) {
	a, b := gui.NewCodeEditor(), gui.NewCodeEditor()
	a.SetText("package p\nfunc Alpha(){}\n")
	b.SetText("package p\nfunc Bravo(){}\n")
	p := NewCodeOutlinePanel()
	p.SetEditor(a)
	p.SetSymbols([]OutlineSymbol{{Name: "OldSemantic", Kind: "func", Line: 1}})
	p.SetEditor(b)
	wantOutlineRows(t, p, []string{"0:Bravo"})
	p.SetSymbols([]OutlineSymbol{{Name: "BeforeEdit", Kind: "func", Line: 1}})
	b.ReplaceAllText("package p\nfunc Current(){}\n")
	p.RefreshIfStale()
	wantOutlineRows(t, p, []string{"0:Current"})
}

func TestOutlinePainterThemeClippingAndFont(t *testing.T) {
	oldMode, oldFont := gui.CurrentThemeMode(), gui.Theme().Font
	t.Cleanup(func() { gui.SetThemeMode(oldMode); gui.Theme().Font = oldFont })
	for _, mode := range []gui.ThemeMode{gui.ThemeLight, gui.ThemeDark} {
		gui.SetThemeMode(mode)
		p := NewCodeOutlinePanel()
		p.SetSize(180, 160)
		var symbols []OutlineSymbol
		for i := 0; i < 5000; i++ {
			symbols = append(symbols, OutlineSymbol{Name: fmt.Sprintf("Symbol%04d%s", i, strings.Repeat("很长名称", 10)), Kind: "func", Line: i})
		}
		p.SetSymbols(symbols)
		p.selectedIdx = 1
		p.hoverIdx = 2
		for _, size := range []int{14, 24} {
			gui.Theme().Font = paint.NewFont(oldFont.Family(), size, false, false)
			p.scrollY = 5
			pix := paint.NewPixmap(180, 160)
			r := &explorerDrawRecorder{Painter: pix.NewPainter()}
			before := r.CurrentState()
			p.Draw(r)
			if r.CurrentState() != before {
				t.Error("outline leaked painter state")
			}
			if p.rowHeight < float64(size)+8 {
				t.Error("outline ignored preferred font size")
			}
			if len(r.texts) > 18 {
				t.Error("too many offscreen text calls", len(r.texts))
			}
			for i, x := range r.texts {
				if x.x+x.width > 180.1 {
					t.Error("symbol text overflows right dock", x.text)
				}
				if i > 0 && x.cy <= 0 {
					t.Error("row is not clipped below fixed header")
				}
				if math.IsNaN(x.y) {
					t.Error("invalid text baseline")
				}
			}
			for _, want := range []paint.Color{gui.Theme().FormColor, gui.Theme().ViewBGColor, gui.Theme().TextColor, gui.Theme().HighLightColor, gui.Theme().MenuActiveTextColor, gui.Theme().ButtonHoverBGColor} {
				found := false
				for _, got := range r.colors {
					found = found || got == want
				}
				if !found {
					t.Error("theme color ignored", want)
				}
			}
		}
	}
}

func TestOutlineGuardedSemanticResponses(t *testing.T) {
	a, b := gui.NewCodeEditor(), gui.NewCodeEditor()
	a.SetText("package p\nfunc Alpha(){}")
	b.SetText("package p\nfunc Bravo(){}")
	p := NewCodeOutlinePanel()
	p.SetEditor(a)
	rev := a.TextRevision()
	semantic := []OutlineSymbol{{Name: "Semantic", Kind: "macro", Line: 1}}
	if !p.SetSymbolsForEditor(a, rev, semantic) {
		t.Fatal("current response refused")
	}
	if p.flatList[0].symbol.Kind != gui.SymMacro {
		t.Fatal("macro kind lost")
	}
	row := &p.flatList[0]
	p.SetEditor(a)
	if &p.flatList[0] != row {
		t.Fatal("same editor binding discarded current response")
	}
	a.ReplaceAllText("package p\nfunc Fresh(){}")
	if p.SetSymbolsForEditor(a, rev, semantic) {
		t.Fatal("old revision response accepted")
	}
	p.RefreshIfStale()
	wantOutlineRows(t, p, []string{"0:Fresh"})
	p.SetEditor(b)
	if p.SetSymbolsForEditor(a, a.TextRevision(), semantic) {
		t.Fatal("other document response accepted")
	}
	wantOutlineRows(t, p, []string{"0:Bravo"})
	p.SetEditor(nil)
	if p.SetSymbolsForEditor(nil, 0, semantic) {
		t.Fatal("unbound response accepted")
	}
	wantOutlineRows(t, p, nil)
	p.SetEditor(b)
	p.SetSymbols(semantic)
	p.SetRevision(9)
	p.RefreshIfStale()
	wantOutlineRows(t, p, []string{"0:Bravo"})
	p.SetSymbols(semantic)
	b.ReplaceAllText("package p\nfunc Direct(){}")
	p.RefreshSymbols()
	wantOutlineRows(t, p, []string{"0:Direct"})
}

func TestOutlineGenericReceiverAndCaretBySourceNotSortOrder(t *testing.T) {
	e := gui.NewCodeEditor()
	e.SetText("package p\ntype Box[T any] struct{}\nfunc (b *Box[T]) Value(){}\nfunc Apple(){}\nfunc Zebra(){}\n")
	p := NewCodeOutlinePanel()
	p.SetEditor(e)
	wantOutlineRows(t, p, []string{"0:Box", "1:Value", "0:Apple", "0:Zebra"})
	p.SetSortMode(OutlineSortByName)
	e.ScrollToLine(3)
	idx := p.currentSymbolIndex()
	if idx < 0 || p.flatList[idx].symbol.Name != "Apple" {
		t.Fatal("caret followed display order instead of source", idx)
	}
	if n := testing.AllocsPerRun(100, func() { p.currentSymbolIndex() }); n != 0 {
		t.Fatal("unchanged caret allocated", n)
	}
	p.SetFilter("Zebra")
	if p.currentSymbolIndex() != -1 {
		t.Fatal("filter kept stale cursor row")
	}
}

func TestOutlineKeyboardScrollingAndHitGeometry(t *testing.T) {
	p := NewCodeOutlinePanel()
	p.SetSize(200, 140)
	p.SetSymbols(sampleOutline())
	var jumps []int
	p.SetNavigateCallback(func(line int) { jumps = append(jumps, line) })
	p.OnKeyDown(gui.KeyHome, false)
	p.OnKeyDown(gui.KeyRight, false) // child Handler
	if p.selectedIdx != 1 {
		t.Fatal("Right did not select child", p.selectedIdx)
	}
	p.OnKeyDown(gui.KeyLeft, false) // collapse Handler
	if len(p.flatList) != 7 {
		t.Fatal("Left did not collapse")
	}
	p.OnKeyDown(gui.KeyRight, false) // expand
	p.OnKeyDown(gui.KeyRight, false) // child ServeHTTP
	p.OnKeyDown(gui.KeyEnter, false)
	p.OnKeyDown(gui.KeyEnter, true)
	if len(jumps) != 1 || jumps[0] != 13 {
		t.Fatal("Enter activation/repeat", jumps)
	}
	p.OnKeyDown(gui.KeyLeft, false) // parent
	if p.selectedIdx != 1 {
		t.Fatal("Left did not select parent")
	}
	p.OnKeyDown(gui.KeyEnd, false)
	if p.selectedIdx != len(p.flatList)-1 || p.scrollY <= 0 {
		t.Fatal("End did not scroll selection into view")
	}
	p.OnKeyDown(gui.KeyPageUp, false)
	p.OnKeyDown(gui.KeyPageDown, false)
	p.OnKeyDown(gui.KeyUp, false)
	p.OnKeyDown(gui.KeyDown, false)
	i := p.selectedIdx
	y := p.headerHeight() + float64(i)*p.rowHeight - p.scrollY + p.rowHeight/2
	if p.hitTest(y) != i || p.hitTest(p.headerHeight()-1) != -1 || p.hitTest(p.Height()) != -1 {
		t.Fatal("hit geometry differs from draw")
	}
	p.OnLeftDown(-1, y)
	p.OnLeftDown(p.Width(), y)
	if len(jumps) != 1 {
		t.Fatal("out-of-dock click navigated")
	}
	p.SetSize(200, 1000)
	p.clampScroll()
	if p.scrollY != 0 {
		t.Fatal("resize did not clamp scroll", p.scrollY)
	}
	p.SetEnabled(false)
	p.OnKeyDown(gui.KeyHome, false)
	if p.selectedIdx != i {
		t.Fatal("disabled panel handled key")
	}
}

func BenchmarkOutlineIdleDraw(b *testing.B) {
	for _, count := range []int{100, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			p := NewCodeOutlinePanel()
			p.SetSize(240, 320)
			syms := make([]OutlineSymbol, count)
			for i := range syms {
				syms[i] = OutlineSymbol{Name: fmt.Sprintf("LongName%05d很长名称", i), Kind: "func", Line: i}
			}
			p.SetSymbols(syms)
			g := paint.NewPixmap(240, 320).NewPainter()
			p.Draw(g)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				p.Draw(g)
			}
		})
	}
}

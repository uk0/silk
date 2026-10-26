package gui

import (
	"github.com/uk0/silk/paint"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestListPresentationFitsSeparateSlotsAndPreservesModel(t *testing.T) {
	old := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(old) })
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		for _, size := range []int{13, 24} {
			l := NewListWidget()
			l.SetFont(paint.NewFont(Theme().Font.Family(), size, false, false))
			title := strings.Repeat("Very long 中文🙂 ", 20)
			detail := strings.Repeat("Details 中文🙂 ", 50)
			l.Append(ListItem{Text: title, Data: 23})
			l.SetItemPresenter(func(int, ListItem) ListItemPresentation {
				return ListItemPresentation{SecondaryText: detail, TrailingText: "Unavailable", Muted: true}
			})
			if l.RowHeight() < 2*l.Font().FontExtents().Height+8 {
				t.Fatal("two-line rows overlap")
			}
			for _, width := range []float64{0, 1, 30, 100, 220, 560} {
				row := l.presentedRow(0, width)
				for _, s := range []string{row.title, row.secondary, row.trailing} {
					if !utf8.ValidString(s) {
						t.Fatal("split Unicode")
					}
				}
				if l.Font().TextExtents(row.title).Width > row.mainWidth+.01 || l.Font().TextExtents(row.secondary).Width > width+.01 || row.trailingWidth > width*.45+.01 {
					t.Fatal("text escaped assigned slot")
				}
				if row.mainWidth+row.trailingWidth > width+.01 {
					t.Fatal("title and status overlap")
				}
			}
			if l.Item(0).Text != title || l.Item(0).Data != 23 {
				t.Fatal("presentation mutated model")
			}
			l.SetItemPresenter(nil)
			if l.RowHeight() != Theme().ItemHeight {
				t.Fatal("legacy row height not restored")
			}
		}
	}
}

func TestListPresentationCacheIsVisibleBoundedAndInvalidated(t *testing.T) {
	l := NewListWidget()
	l.SetSize(240, 150)
	for i := 0; i < 1000; i++ {
		l.Append(ListItem{Text: "Name"})
	}
	calls := 0
	l.SetItemPresenter(func(int, ListItem) ListItemPresentation {
		calls++
		return ListItemPresentation{SecondaryText: "Description"}
	})
	p := paint.NewPixmap(240, 150)
	g := p.NewPainter()
	l.Draw(g)
	n := calls
	if n == 0 || n > 8 {
		t.Fatalf("not bounded by visible rows: %d", n)
	}
	l.Draw(g)
	if calls != n {
		t.Fatal("warm repaint remeasured presentation")
	}
	l.SetActiveIndex(1)
	l.Draw(g)
	if calls != n {
		t.Fatal("selection needlessly reshaped rows")
	}
	l.SetItem(0, ListItem{Text: "Changed"})
	l.Draw(g)
	if calls <= n {
		t.Fatal("item replacement kept stale presentation")
	}
	n = calls
	l.SetSize(180, 150)
	l.Draw(g)
	if calls <= n {
		t.Fatal("resize did not invalidate")
	}
	n = calls
	l.SetFont(paint.NewFont(l.Font().Family(), 22, false, false))
	l.Draw(g)
	if calls <= n {
		t.Fatal("font did not invalidate")
	}
	oldTheme := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(oldTheme) })
	n = calls
	if oldTheme == ThemeDark {
		SetThemeMode(ThemeLight)
	} else {
		SetThemeMode(ThemeDark)
	}
	l.Draw(g)
	if calls <= n {
		t.Fatal("theme did not invalidate presentation")
	}
	l.InvalidateItemPresentation()
	if len(l.presentationRows) != 0 {
		t.Fatal("explicit invalidation failed")
	}
	for i := 0; i < 1000; i++ {
		l.presentedRow(i, 160)
	}
	if len(l.presentationRows) > 128 {
		t.Fatal("unbounded presentation cache")
	}
	l.Clear()
	if len(l.presentationRows) != 0 {
		t.Fatal("clear retained rows")
	}
}

func TestListPresentationMutedIsNotActivationPolicy(t *testing.T) {
	l := NewListWidget()
	l.SetSize(180, 120)
	l.Append(ListItem{Text: "Explain unavailable command"})
	l.SetItemPresenter(func(int, ListItem) ListItemPresentation { return ListItemPresentation{Muted: true} })
	submitted := 0
	l.SigSubmit(func(interface{}) { submitted++ })
	l.SetActiveIndex(0)
	l.OnKeyDown(KeyEnter, false)
	if submitted != 1 {
		t.Fatal("visual presentation changed activation contract")
	}
}

func TestListContentsDoNotPaintIntoPadding(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		for _, rich := range []bool{false, true} {
			l := NewListWidget()
			l.SetSize(150, 90)
			l.SetPadding(8, 8, 8, 8)
			l.Append(ListItem{Text: strings.Repeat("W", 100)})
			if rich {
				l.SetItemPresenter(func(int, ListItem) ListItemPresentation {
					return ListItemPresentation{SecondaryText: strings.Repeat("W", 100), TrailingText: "State"}
				})
			}
			w, h := int(math.Ceil(150*scale)), int(math.Ceil(90*scale))
			p := paint.NewPixmap(w, h)
			g := p.NewPainter()
			g.Scale(scale, scale)
			l.Draw(g)
			p.Flush()
			full, err := p.Image()
			if err != nil {
				t.Fatal(err)
			}
			l.Clear()
			blank := paint.NewPixmap(w, h)
			bg := blank.NewPainter()
			bg.Scale(scale, scale)
			l.Draw(bg)
			blank.Flush()
			empty, err := blank.Image()
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					if float64(x) >= 8*scale && float64(x) < 142*scale && float64(y) >= 8*scale && float64(y) < 82*scale {
						continue
					}
					if full.At(x, y) != empty.At(x, y) {
						t.Fatalf("list text escaped padding scale=%g rich=%v at %d,%d: full=%v empty=%v", scale, rich, x, y, full.At(x, y), empty.At(x, y))
					}
				}
			}
		}
	}
}

func BenchmarkListPresentationWarmCache(b *testing.B) {
	l := NewListWidget()
	l.Append(ListItem{Text: strings.Repeat("Long title 中文 ", 20)})
	l.SetItemPresenter(func(int, ListItem) ListItemPresentation {
		return ListItemPresentation{SecondaryText: strings.Repeat("Explanation ", 100), TrailingText: "Unavailable"}
	})
	l.presentedRow(0, 400)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.presentedRow(0, 400)
	}
}

func TestListHitTestingMatchesClippedContents(t *testing.T) {
	l := NewListWidget()
	l.SetSize(160, 100)
	l.SetPadding(8, 8, 8, 8)
	l.SetIconVisible(true)
	l.SetCheckBoxVisible(true)
	for i := 0; i < 20; i++ {
		l.Append(ListItem{Text: "Item"})
	}
	l.SetItemPresenter(func(int, ListItem) ListItemPresentation { return ListItemPresentation{SecondaryText: "Description"} })
	l.SetScrollY(1.25)
	for _, pt := range [][2]float64{{0, 20}, {159, 20}, {20, 0}, {20, 99}, {-1, 20}, {20, 101}} {
		if r, _ := l.HitTest(pt[0], pt[1]); r != -1 {
			t.Fatalf("invisible content hit at %v: %d", pt, r)
		}
	}
	x := 8 + l.IconSize() + 4
	if r, c := l.HitTest(x, 20); r < 0 || c != 1 {
		t.Fatalf("checkbox column hit=%d,%d", r, c)
	}
}

type listPresentationInkRecorder struct {
	paint.Painter
	texts []string
	xs    []float64
}

func (g *listPresentationInkRecorder) DrawText1(x, y float64, s string) {
	g.texts = append(g.texts, s)
	g.xs = append(g.xs, x)
	g.Painter.DrawText1(x, y, s)
}

func TestListPresentationAlignsGlyphInkInsideStatusSlot(t *testing.T) {
	l := NewListWidget()
	l.Append(ListItem{Text: "Long command title"})
	l.SetItemPresenter(func(int, ListItem) ListItemPresentation {
		return ListItemPresentation{SecondaryText: "Description", TrailingText: "Unavailable"}
	})
	row := l.presentedRow(0, 220)
	pix := paint.NewPixmap(250, 80)
	g := &listPresentationInkRecorder{Painter: pix.NewPainter()}
	g.SetFont(l.Font())
	l.drawPresentedRow(g, 0, 10, 0, 220, l.RowHeight(), true)
	if len(g.texts) != 3 {
		t.Fatal("missing presentation text")
	}
	left := []float64{10, 10 + 220 - row.trailingWidth, 10}
	width := []float64{row.mainWidth, row.trailingWidth, 220}
	for i, s := range g.texts {
		ext := l.Font().TextExtents(s)
		x := g.xs[i] + ext.XBearing
		if x < left[i]-.01 || x+ext.Width > left[i]+width[i]+.01 {
			t.Fatalf("glyph ink clipped in slot: %q x=%g left=%g width=%g", s, x, left[i], width[i])
		}
	}
}

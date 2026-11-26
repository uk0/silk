package gui

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func chartFixture(t *testing.T) *GanttChart {
	t.Helper()
	c := NewGanttChart()
	d := time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)
	err := c.SetTasks([]GanttTask{
		{ID: "project", Name: "项目 / Release"},
		{ID: "group", ParentID: "project", Name: "多语言界面"},
		{ID: "a", ParentID: "group", Name: "Design", Start: d, End: d.AddDate(0, 0, 4), Progress: .5, Owner: "UI"},
		{ID: "b", ParentID: "group", Name: "Build", Start: d.AddDate(0, 0, 4), End: d.AddDate(0, 0, 8), Dependencies: []string{"a"}, Owner: "SDK"},
		{ID: "m", ParentID: "project", Name: "Ship", Start: d.AddDate(0, 0, 8), End: d.AddDate(0, 0, 8), Milestone: true, Dependencies: []string{"b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.SetSize(960, 330)
	oldFocus := focusWidget
	t.Cleanup(func() { c.CancelInteraction(); focusWidget = oldFocus })
	return c
}
func TestGanttFactoryInitializesRealControlAndSafeProperties(t *testing.T) {
	v := core.New("gui.GanttChart")
	c, ok := v.(*GanttChart)
	if !ok {
		t.Fatalf("factory=%T", v)
	}
	if w, h := c.Size(); w < 500 || h < 200 || len(c.Tasks()) < 5 || c.DayWidth() == 0 || len(c.rows) == 0 {
		t.Fatalf("uninitialized factory chart: %.0fx%.0f tasks=%d", w, h, len(c.Tasks()))
	}
	before := c.TasksJSON()
	c.SetTasksJSON(`[{"id":"bad","name":"bad","start":"2026-01-02","end":"2026-01-01"}]`)
	if c.LastError() == nil || c.TasksJSON() != before {
		t.Fatal("invalid property replaced valid tasks or suppressed error")
	}
	c.SetTasksJSON(before)
	if c.LastError() != nil {
		t.Fatal(c.LastError())
	}
	c.SetDayWidth(math.NaN())
	c.SetRowHeight(math.Inf(1))
	c.SetTableWidth(math.Inf(-1))
	if !ganttFinite(c.DayWidth()) || !ganttFinite(c.RowHeight()) || !ganttFinite(c.TableWidth()) {
		t.Fatal("invalid geometry accepted")
	}
}

type ganttPaintRecorder struct {
	paint.Painter
	draws, rects int
	lines        [][4]float64
	x, y         float64
	text         []string
}

func (r *ganttPaintRecorder) Rectangle(x, y, w, h float64) {
	r.rects++
	r.Painter.Rectangle(x, y, w, h)
}
func (r *ganttPaintRecorder) MoveTo(x, y float64) { r.x = x; r.y = y; r.Painter.MoveTo(x, y) }
func (r *ganttPaintRecorder) LineTo(x, y float64) {
	r.lines = append(r.lines, [4]float64{r.x, r.y, x, y})
	r.x = x
	r.y = y
	r.Painter.LineTo(x, y)
}
func (r *ganttPaintRecorder) DrawText1(x, y float64, s string) {
	r.draws++
	r.text = append(r.text, s)
	r.Painter.DrawText1(x, y, s)
}
func ganttPixel(img image.Image, x, y int) paint.Color {
	return paint.ColorModel.Convert(img.At(x, y)).(paint.Color)
}

func TestGanttTableKeepsFullDatesAndDropsOptionalColumns(t *testing.T) {
	c := chartFixture(t)
	f := Theme().Font
	b := f.Dup()
	b.SetBold(true)
	for _, width := range []float64{100, 249, 320, 430, 600, 1000} {
		c.SetTableWidth(width)
		c.SetSize(1400, 330)
		l := c.geometry()
		cols := c.tableColumns(l)
		if width == 430 && len(cols) < 3 {
			t.Fatal("default chart must show start and finish dates")
		}
		sum := 0.0
		for _, w := range cols {
			if w <= 0 {
				t.Fatalf("non-positive column: %v", cols)
			}
			sum += w
		}
		if math.Abs(sum-l.split) > .001 {
			t.Fatalf("columns overflow table: %v split=%v", cols, l.split)
		}
		if len(cols) >= 3 {
			for _, font := range []paint.Font{f, b} {
				for _, date := range []string{"2026-04-06", "2026-12-31", "8888-08-28"} {
					if got := ganttElide(font, date, cols[1]-8); got != date {
						t.Fatalf("table=%v date clipped to %q: %v", width, got, cols)
					}
				}
			}
		}
	}
	// Verify the actual draw path, not only the column geometry.
	c.SetTableWidth(430)
	c.SetSize(960, 330)
	pix := paint.NewPixmap(960, 330)
	rec := &ganttPaintRecorder{Painter: pix.NewPainter()}
	c.Draw(rec)
	for _, want := range []string{"2026-04-06", "2026-04-09"} {
		found := false
		for _, text := range rec.text {
			found = found || text == want
		}
		if !found {
			t.Fatalf("full date %q missing from rendered table", want)
		}
	}
}

func TestGanttDrawThemeTokensRetinaAndAlignedRows(t *testing.T) {
	mode := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(mode) })
	for _, theme := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(theme)
		for _, scale := range []int{1, 2} {
			c := chartFixture(t)
			l := c.geometry()
			p := ganttThemeColors()
			pix := paint.NewPixmap(960*scale, 330*scale)
			g := pix.NewPainter()
			g.Scale(float64(scale), float64(scale))
			rec := &ganttPaintRecorder{Painter: g}
			c.Draw(rec)
			img, err := pix.Image()
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"project", "a", "m"} {
				i := c.rowIndex[id]
				b := c.taskRect(c.rows[i].Task, i, l)
				x, y := b.X+10, b.Y+1
				want := p.leaf
				if id == "project" {
					want = p.summary
				}
				if id == "m" {
					want = p.milestone
					x = b.X + b.Width/2
					y = b.Y + b.Height/2
				}
				got := ganttPixel(img, int(x*float64(scale)), int(y*float64(scale)))
				if got != want {
					t.Errorf("theme=%v scale=%d %s bar pixel %v want %v at %.1f,%.1f", theme, scale, id, got, want, x, y)
				}
				hit := c.hitTest(math.Max(l.split+1, b.X+b.Width/2), l.top+float64(i)*l.row+l.row/2)
				if hit.row != i {
					t.Fatalf("table/timeline row mismatch %s hit %+v want %d", id, hit, i)
				}
			}
			if contrast := contrastRatio(p.text, p.bg); contrast < 4.5 {
				t.Fatalf("text contrast=%v", contrast)
			}
			if contrast := contrastRatio(p.barText, p.leaf); contrast < 4.5 {
				t.Fatalf("bar contrast=%v", contrast)
			}
			if contrast := contrastRatio(p.barText, p.summary); contrast < 4.5 {
				t.Fatalf("summary contrast=%v", contrast)
			}
			if dir := os.Getenv("SILK_GANTT_QA_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := pix.WritePNG(filepath.Join(dir, fmt.Sprintf("gantt-%d-%dx.png", theme, scale))); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
func TestGanttDrawClipsTinyBoundsAndCullsLargeProjects(t *testing.T) {
	c := chartFixture(t)
	for _, size := range [][2]float64{{1, 1}, {30, 20}, {180, 90}, {390, 220}} {
		c.SetSize(size[0], size[1])
		pix := paint.NewPixmap(int(size[0])+20, int(size[1])+20)
		g := pix.NewPainter()
		outside := paint.Color{R: 251, G: 0, B: 251, A: 255}
		g.SetBrush1(outside)
		g.Paint()
		g.Translate(8, 8)
		c.Draw(g)
		img, err := pix.Image()
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				if x < 8 || y < 8 || x >= 8+int(size[0]) || y >= 8+int(size[1]) {
					if ganttPixel(img, x, y) != outside {
						t.Fatalf("%.0fx%.0f leaks pixel %d,%d", size[0], size[1], x, y)
					}
				}
			}
		}
	}
	var tasks []GanttTask
	d := time.Date(1800, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		tasks = append(tasks, GanttTask{ID: fmt.Sprint(i), Name: fmt.Sprintf("Row %04d", i), Start: d, End: d.AddDate(300, 0, 0)})
	}
	if err := c.SetTasks(tasks); err != nil {
		t.Fatal(err)
	}
	c.SetSize(800, 260)
	c.SetDayWidth(4)
	c.SetScrollOffsets(99999, 12000)
	pix := paint.NewPixmap(800, 260)
	rec := &ganttPaintRecorder{Painter: pix.NewPainter()}
	c.Draw(rec)
	first, last := c.visibleRange(c.geometry())
	if last-first > 10 || rec.draws > 180 || rec.rects > 700 {
		t.Fatalf("unbounded work first=%d last=%d texts=%d rects=%d", first, last, rec.draws, rec.rects)
	}
	for _, text := range rec.text {
		if text == "Row 0000" || text == "Row 0999" {
			t.Fatal("offscreen task drawn")
		}
	}
}
func TestGanttCollapsePreservesNestedDataAndSharedScroll(t *testing.T) {
	c := chartFixture(t)
	before := c.Tasks()
	if err := c.SetCollapsed("group", true); err != nil {
		t.Fatal(err)
	}
	if len(c.rows) != 3 {
		t.Fatalf("collapsed rows=%d", len(c.rows))
	}
	if !c.Undo() || !reflect.DeepEqual(c.Tasks(), before) {
		t.Fatal("collapse undo lost data")
	}
	if len(c.rows) != 5 {
		t.Fatal("undo did not rebuild visible tree")
	}
	c.SetSize(960, 120)
	// Deliberately leave half a logical pixel of the first visible row. A
	// point 2 px below the header may already hit the next row.
	const visibleTail = 0.5
	target := 2*c.geometry().row - visibleTail
	c.SetScrollOffsets(0, target)
	l := c.geometry()
	first, last := c.visibleRange(l)
	if math.Abs(c.scrollY-target) > 1e-9 || first != 1 || last <= 2 {
		t.Fatalf("fixture did not expose a partial row and its successor: scroll=%v want=%v range=%d..%d", c.scrollY, target, first, last)
	}
	for _, x := range []float64{40, l.split + 100} {
		for i, y := range []float64{l.top + visibleTail/2, l.top + 2*visibleTail} {
			if hit := c.hitTest(x, y); hit.row != i+1 {
				t.Fatalf("scroll split mismatch at (%v,%v): hit=%+v want row %d", x, y, hit, i+1)
			}
		}
	}
}

package prop

import (
	"fmt"
	"math"
	"testing"

	"github.com/uk0/silk/gui"
)

func inspectorCatalog(n int) *PropertySheet {
	s := newTestSheet()
	s.SetSize(320, 240)
	for i := 0; i < n; i++ {
		v := i
		addTestProp(s, fmt.Sprintf("Field%04d", i), func() int { return v }, func(n int) { v = n })
	}
	s.Layout()
	return s
}

func BenchmarkInspectorScroll(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s := inspectorCatalog(n)
			defer s.Clear(nil)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.SetScrollY(float64((i % 2) * 78))
			}
			b.StopTimer()
		})
	}
}

func TestInspectorScrollSteadyStateAllocations(t *testing.T) {
	s := inspectorCatalog(100)
	defer s.Clear(nil)
	i := 0
	allocs := testing.AllocsPerRun(100, func() { i++; s.SetScrollY(float64((i % 2) * 78)) })
	if allocs != 0 {
		t.Fatalf("ordinary property scrolling allocated %.0f objects per event", allocs)
	}
}

type geometryCallbackEdit struct {
	TextEdit
	move func()
}

func (e *geometryCallbackEdit) OnMove() {
	if e.move != nil {
		e.move()
	}
}
func installGeometryCallback(s *PropertySheet, p *PropertyItem) *geometryCallbackEdit {
	p.control.SetParent(nil)
	e := new(geometryCallbackEdit)
	e.Init(e)
	e.SetParent(s.body)
	e.BindProperty(&PropertyItem1{p})
	p.control = e
	p.UpdateControl()
	s.Layout()
	return e
}

func TestInspectorGeometryCallbackRetainsLatestLayout(t *testing.T) {
	for _, cause := range []string{"scroll", "resize"} {
		t.Run(cause, func(t *testing.T) {
			s := longInspector(t)
			defer s.Clear(nil)
			p := s.vlist[0]
			e := installGeometryCallback(s, p)
			e.move = func() { e.move = nil; p.SetCfgItemHeight(53); s.Layout() }
			if cause == "scroll" {
				s.SetScrollY(40)
			} else {
				s.vertSplit = .4
				s.Layout()
			}
			if e.Height() != 53 || s.vlist[1].ypos-p.ypos != 53 || e.Y() != p.ypos-s.contentTop()-s.ScrollY() {
				t.Fatal("outer geometry write overwrote reentrant layout", e.Bounds1(), s.vlist[1].ypos-p.ypos)
			}
		})
	}
}

func TestInspectorRejectsNonfiniteRowHeight(t *testing.T) {
	s := longInspector(t)
	defer s.Clear(nil)
	p := s.vlist[0]
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		p.SetCfgItemHeight(bad)
		s.Layout()
		_, limit := s.VertScrollBar().Range()
		if p.Control().Height() != s.DefaultItemHeight() || math.IsNaN(limit) || math.IsInf(limit, 0) {
			t.Fatal("invalid row height escaped into geometry", bad, p.Control().Bounds1(), limit)
		}
	}
}

func TestInspectorCachedGeometryChangesOnlyAtLayout(t *testing.T) {
	s := longInspector(t)
	defer s.Clear(nil)
	p := s.vlist[0]
	// A raw resource edit has no change signal. Presentation retains one
	// coherent committed snapshot until Layout is explicitly requested.
	p.itemConfig.WriteAttr("height", 61)
	s.SetScrollY(30)
	if p.control.Height() != 26 || s.getItemRect(p, pitemLabel).Height != 26 || s.vlist[1].ypos-p.ypos != 26 {
		t.Fatal("scroll mixed live config with committed row geometry")
	}
	s.Layout()
	if p.control.Height() != 61 || s.getItemRect(p, pitemLabel).Height != 61 || s.vlist[1].ypos-p.ypos != 61 {
		t.Fatal("Layout did not invalidate row geometry")
	}
	p.SetCfgItemHeight(39)
	if p.control.Height() != 39 || s.vlist[1].ypos-p.ypos != 39 {
		t.Fatal("public height setter did not update geometry")
	}
	s.SetFilter(p.Id())
	s.SetSize(280, 73)
	p.control.SetFocus()
	if p.control.Height() != 39 || p.control.Y()+39 > s.body.Height() {
		t.Fatal("cache broke filtered/resized focus reveal", p.control.Bounds1())
	}
}

func TestInspectorGeometryCallbackRebindAndFilter(t *testing.T) {
	for _, change := range []string{"rebind", "filter", "scroll"} {
		t.Run(change, func(t *testing.T) {
			s := longInspector(t)
			defer s.Clear(nil)
			old := s.vlist[0]
			e := installGeometryCallback(s, old)
			e.move = func() {
				e.move = nil
				switch change {
				case "rebind":
					s.Bind([]interface{}{&fakeObj{enum: func(l IPropertyList) { l.AddProperty("Replacement", func() int { return 2 }, nil) }}}, "default", struct{}{})
				case "filter":
					s.SetFilter("Field29")
				case "scroll":
					s.SetScrollY(100)
				}
			}
			s.SetScrollY(40)
			for _, p := range s.vlist {
				if p.retired || p.control.Parent() != s.body || p.control.Y() != p.ypos-s.contentTop()-s.ScrollY() || p.control.Height() != s.getItemRect(p, pitemLabel).Height {
					t.Fatal("callback left stale/misaligned editors", change, p.Id(), p.control.Bounds1())
				}
			}
			if change == "rebind" && (!old.retired || e.Parent() != nil || len(s.body.Children()) != 1 || s.vlist[0].Id() != "Replacement") {
				t.Fatal("retired binding revived")
			}
			if change == "filter" && (len(s.vlist) != 1 || s.vlist[0].Id() != "Field29" || e.IsVisible()) {
				t.Fatal("filter callback lost")
			}
			if change == "scroll" && s.ScrollY() != 100 {
				t.Fatal("nested scroll was overwritten")
			}
		})
	}
}

func TestInspectorGeometryReentryYieldsWithoutRecursion(t *testing.T) {
	s := longInspector(t)
	defer s.Clear(nil)
	e := installGeometryCallback(s, s.vlist[0])
	calls := 0
	e.move = func() { calls++; s.SetScrollY(s.ScrollY() + 1) }
	s.SetScrollY(40)
	e.move = nil
	if calls > 2 || calls == 0 {
		t.Fatal("unbounded synchronous geometry recursion", calls)
	}
	// A later UI/layout turn must install the final requested offset.
	s.Layout()
	if e.Y() != s.vlist[0].ypos-s.contentTop()-s.ScrollY() {
		t.Fatal("deferred geometry never settled")
	}
}

func TestInspectorLayoutRebindRetiresSnapshot(t *testing.T) {
	s := longInspector(t)
	defer s.Clear(nil)
	old := s.vlist[0]
	e := installGeometryCallback(s, old)
	e.move = func() {
		e.move = nil
		s.Bind([]interface{}{&fakeObj{enum: func(l IPropertyList) { l.AddProperty("NewOwner", func() int { return 7 }, nil) }}}, "default", struct{}{})
	}
	s.vertSplit = .4
	s.Layout()
	if !old.retired || e.Parent() != nil || len(s.vlist) != 1 || s.vlist[0].Id() != "NewOwner" || len(s.body.Children()) != 1 {
		t.Fatal("layout resumed a retired binding")
	}
	now := s.vlist[0]
	bounds := now.control.Bounds1()
	oldHeight := old.CfgItemHeight()
	old.SetCfgItemHeight(91)
	if old.CfgItemHeight() != oldHeight || now.control.Bounds1() != bounds {
		t.Fatal("retired metadata callback affected a newer binding")
	}
}

func TestInspectorLayoutReentryYieldsWithoutRecursion(t *testing.T) {
	s := longInspector(t)
	defer s.Clear(nil)
	e := installGeometryCallback(s, s.vlist[0])
	calls := 0
	e.move = func() { calls++; s.vertSplit = .9 - s.vertSplit; s.Layout() }
	s.vertSplit = .4
	s.Layout()
	e.move = nil
	if calls != 2 {
		t.Fatal("layout did not bound synchronous corrective passes", calls)
	}
	s.Layout()
	if e.X() != s.vertSplitPx() {
		t.Fatal("later layout did not settle latest geometry")
	}
}

// Keep the promoted property editor's SDK contract checked by the compiler.
var _ gui.IWidget = (*geometryCallbackEdit)(nil)

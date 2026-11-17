package prop

import (
	"math"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

func TestInspectorBuiltinPropertyCategories(t *testing.T) {
	for category, ids := range map[string][]string{
		"general":    {"控件名称", "控件类型", "类型", "Onéclair"},
		"layout":     {"X 位置 (mm)", "高度 (mm)", "布局行", "布局列", "跨行数", "跨列数", "间距", "内边距", "rowHeight", "tableWidth"},
		"appearance": {"标题", "文本", "颜色", "图标", "可见", "副标题", "激活图标", "显示标签", "Show Bytes"},
		"behavior":   {"Checkable", "Checked", "可用", "只读", "选中", "值", "最大值", "步长", "运行", "温度", "cascadeDependencies"},
		"binding":    {"tag", "URL", "HTTP Method", "Request Body", "Headers JSON", "JSON Pointer", "Result Type", "转换规则 JSON", "颜色规则 JSON", "协议", "点表"},
		"events":     {"OnToggled", "OnTextChanged", "OnΩChanged"},
	} {
		for _, id := range ids {
			t.Run(id, func(t *testing.T) {
				if got := categoryOfPropID(id); got != category {
					t.Errorf("categoryOfPropID(%q) = %q, want %q", id, got, category)
				}
			})
		}
	}
}

func TestInspectorActualGroupBoxCategoriesAndSearch(t *testing.T) {
	old := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocale(old) })
	for _, locale := range []string{"en", "zh-CN"} {
		i18n.SetLocale(locale)
		sheet := newTestSheet()
		group := gui.NewGroupBox("Process options")
		group.EnumProperties(&propertyListAdapter{sheet: sheet})
		for _, id := range []string{"标题", "Checkable", "Checked"} {
			sheet.rlist = append(sheet.rlist, sheet.propMap[id])
		}
		sheet.Layout()
		if headers, _ := layoutRows(sheet); !equalStrs(headers, []string{"appearance", "behavior"}) {
			t.Fatalf("%s: actual group headers = %v", locale, headers)
		}
		for query, want := range map[string][]string{
			"Behavior": {"Checkable", "Checked"}, "行为": {"Checkable", "Checked"},
			"Title": {"标题"}, "标题": {"标题"},
			"可勾选": {"Checkable"}, "选中": {"Checked"},
		} {
			sheet.SetFilter(query)
			if _, ids := layoutRows(sheet); !equalStrs(ids, want) {
				t.Errorf("%s/%s rows = %v, want %v", locale, query, ids, want)
			}
		}
		if group.IsCheckable() || !group.IsChecked() || group.Title() != "Process options" {
			t.Fatal("view-only grouping/search mutated the widget")
		}
	}
}

func TestInspectorCategoryHeaderTextSinglePassAndBounded(t *testing.T) {
	oldMode, oldLocale := gui.CurrentThemeMode(), i18n.Locale()
	t.Cleanup(func() { gui.SetThemeMode(oldMode); i18n.SetLocale(oldLocale) })
	for _, mode := range []gui.ThemeMode{gui.ThemeLight, gui.ThemeDark} {
		gui.SetThemeMode(mode)
		for _, locale := range []string{"en", "zh-CN"} {
			i18n.SetLocale(locale)
			for _, width := range []float64{12, 42, 85, 240} {
				sheet := newTestSheet()
				pix := paint.NewPixmap(260, 70)
				g := &propertyTextRecorder{Painter: pix.NewPainter()}
				font := paint.NewFont("monospace", 9, false, false)
				g.SetFont(font)
				sheet.drawCategoryHeader(g, "binding", 10, width)
				if len(g.records) > 1 {
					t.Errorf("%d/%s/%g: header draws text %d times", mode, locale, width, len(g.records))
				}
				pix.Flush()
				img, err := pix.Image()
				if err != nil {
					t.Fatal(err)
				}
				for y := 0; y < 70; y++ {
					for x := 0; x < 260; x++ {
						if x >= int(width) || y < 10 || y >= 34 {
							_, _, _, a := img.At(x, y).RGBA()
							if a != 0 {
								t.Fatalf("%d/%s/%g: header escaped band at %d,%d", mode, locale, width, x, y)
							}
						}
					}
				}
				paint.ClosePainter(g.Painter)
				paint.CloseSurface(pix)
			}
		}
	}
}

func TestInspectorBlankFilterKeepsCollapsedChevron(t *testing.T) {
	for _, query := range []string{"", " ", "\t\n", "\u3000"} {
		if categoryChevronOpen(false, query) {
			t.Errorf("blank filter %q opened collapsed chevron", query)
		}
	}
}

func TestInspectorHeaderHitStaysInsideClient(t *testing.T) {
	sheet := newTestSheet()
	sheet.SetSize(200, 160)
	addTestProp(sheet, "Checkable", func() bool { return true }, nil)
	sheet.Layout()
	y := sheet.contentTop() + categoryHeaderHeight/2
	for _, x := range []float64{-1, 200, math.Inf(1), math.NaN()} {
		sheet.OnLeftDown(x, y)
		if !sheet.categories["behavior"].expanded {
			t.Errorf("outside x=%g collapsed header", x)
			sheet.categories["behavior"].expanded = true
		}
	}
	sheet.SetSize(200, 20) // Header lies below the visible client.
	sheet.OnLeftDown(10, y)
	if !sheet.categories["behavior"].expanded {
		t.Error("clipped-away header accepted a click")
	}
	sheet.categories["behavior"].expanded = true
	sheet.SetSize(200, 160)
	sheet.OnLeftDown(10, y)
	if sheet.categories["behavior"].expanded {
		t.Error("real header click did not collapse")
	}
}

func TestInspectorCategorySearchPreservesCollapseAndHiddenControls(t *testing.T) {
	sheet := newTestSheet()
	check := addTestProp(sheet, "Checkable", func() bool { return true }, nil)
	hidden := addTestProp(sheet, "Checked", func() bool { return true }, nil)
	hidden.SetCfgHidden(true)
	sheet.categories["behavior"].expanded = false
	sheet.SetFilter("行为")
	if headers, ids := layoutRows(sheet); !equalStrs(headers, []string{"behavior"}) || !equalStrs(ids, []string{"Checkable"}) {
		t.Fatalf("category search headers=%v rows=%v", headers, ids)
	}
	if hidden.Control().IsVisible() || !check.Control().IsVisible() {
		t.Fatal("category search revived hidden row or hid the actual match")
	}
	sheet.SetFilter("\u3000")
	if _, ids := layoutRows(sheet); len(ids) != 0 || check.Control().IsVisible() {
		t.Fatal("blank filter did not restore collapse/child visibility")
	}
	sheet.categories["behavior"].expanded = true
	sheet.objCount = 2
	addTestProp(sheet, "OnToggled", func() string { return "handler" }, nil)
	sheet.SetFilter("Events")
	if headers, ids := layoutRows(sheet); len(headers) != 0 || len(ids) != 0 {
		t.Fatal("category search bypassed multi-selection event exclusion")
	}
}

func TestInspectorHeaderFontCacheTracksValueWithoutMutatingTheme(t *testing.T) {
	sheet := newTestSheet()
	font := paint.NewFont("monospace", 11, false, false)
	bold := sheet.categoryFont(font)
	if !bold.Bold() || font.Bold() || sheet.categoryFont(font) != bold {
		t.Fatal("header mutates original font or recreates warm cache")
	}
	font.SetSize(19)
	changed := sheet.categoryFont(font)
	if changed == bold || changed.Size() != 19 || bold.Size() != 11 || font.Bold() {
		t.Fatal("in-place font mutation kept stale derived metrics")
	}
	pix := paint.NewPixmap(200, 60)
	defer paint.CloseSurface(pix)
	g := pix.NewPainter()
	defer paint.ClosePainter(g)
	g.SetFont(font)
	state := g.CurrentState()
	sheet.drawCategoryHeader(g, "binding", 5, 190)
	if g.CurrentState() != state || !g.Font().Equal(font) {
		t.Fatal("header leaked painter state or font")
	}
}

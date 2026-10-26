package prop

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

type propertyTextRecord struct {
	text  string
	x, y  float64
	color paint.Color
}
type propertyTextRecorder struct {
	paint.Painter
	records []propertyTextRecord
	color   paint.Color
}

func (g *propertyTextRecorder) SetBrush1(c paint.Color) { g.color = c; g.Painter.SetBrush1(c) }
func (g *propertyTextRecorder) DrawText1(x, y float64, s string) {
	g.records = append(g.records, propertyTextRecord{s, x, y, g.color})
	g.Painter.DrawText1(x, y, s)
}

func TestPropertyDisplayLabelsPreserveIDsAndCustomConfig(t *testing.T) {
	old := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocale(old) })
	for _, locale := range []string{"en", "zh-CN"} {
		i18n.SetLocale(locale)
		for id, en := range legacyPropertyLabels {
			item := addTestProp(newTestSheet(), id, func() string { return "value" }, nil)
			want := en
			if locale == "zh-CN" {
				want = id
			}
			if item.Label() != want || item.Id() != id || item.GetValueStr() != "value" {
				t.Fatalf("%s %s label=%q", locale, id, item.Label())
			}
			if !propMatchesFilter(item.Label(), item.Id(), want) {
				t.Fatal("visible label is not searchable")
			}
			item.SetCfgLabel("Custom 控件")
			if item.Label() != "Custom 控件" || item.Id() != id {
				t.Fatal("translation changed explicit config/ID")
			}
		}
	}
}

func TestPropertyTextClipsAndElidesNarrowLabels(t *testing.T) {
	oldMode := gui.CurrentThemeMode()
	oldLocale := i18n.Locale()
	t.Cleanup(func() { gui.SetThemeMode(oldMode); i18n.SetLocale(oldLocale) })
	for _, mode := range []gui.ThemeMode{gui.ThemeLight, gui.ThemeDark} {
		gui.SetThemeMode(mode)
		for _, locale := range []string{"en", "zh-CN"} {
			i18n.SetLocale(locale)
			for _, width := range []float64{60, 100, 240} {
				t.Run(fmt.Sprintf("%d/%s/%g", mode, locale, width), func(t *testing.T) {
					sheet := newTestSheet()
					sheet.SetSize(width, 160)
					item := addTestProp(sheet, "text", func() string { return "v" }, nil)
					item.SetCfgLabel(strings.Repeat("Long控件🙂", 20))
					sheet.Layout()
					pix := paint.NewPixmap(260, 180)
					g := &propertyTextRecorder{Painter: pix.NewPainter()}
					sheet.drawPropertyLabel(g, item)
					r := sheet.getItemRect(item, pitemLabel)
					if len(g.records) != 1 {
						t.Fatal("label draw missing")
					}
					text := g.records[0].text
					if !utf8.ValidString(text) || gui.Theme().Font.TextExtents(text).Width > r.Width-4 {
						t.Fatal("label escaped allocated column")
					}
					if text == item.Label() {
						t.Fatal("long label was not elided")
					}
					pix.Flush()
					img, err := pix.Image()
					if err != nil {
						t.Fatal(err)
					}
					for y := 0; y < 180; y++ {
						for x := int(r.X + r.Width - 4); x < 260; x++ {
							_, _, _, a := img.At(x, y).RGBA()
							if a != 0 {
								t.Fatalf("label pixel outside column at %d,%d", x, y)
							}
						}
					}
				})
			}
		}
	}
}

func TestMixedPropertyHintUsesReadableThemeTextAndLocale(t *testing.T) {
	oldMode := gui.CurrentThemeMode()
	oldLocale := i18n.Locale()
	t.Cleanup(func() { gui.SetThemeMode(oldMode); i18n.SetLocale(oldLocale) })
	for _, mode := range []gui.ThemeMode{gui.ThemeLight, gui.ThemeDark} {
		gui.SetThemeMode(mode)
		for _, locale := range []string{"en", "zh-CN"} {
			i18n.SetLocale(locale)
			for _, typ := range []string{"text", "float", "int", "color", "bool"} {
				t.Run(fmt.Sprintf("%d/%s/%s", mode, locale, typ), func(t *testing.T) {
					obj := func(v int) *fakeObj {
						return &fakeObj{enum: func(l IPropertyList) {
							switch typ {
							case "text":
								l.AddProperty("value", func() string { return fmt.Sprint(v) }, nil)
							case "float":
								l.AddProperty("value", func() float64 { return float64(v) }, nil)
							case "int":
								l.AddProperty("value", func() int { return v }, nil)
							case "color":
								l.AddProperty("value", func() paint.Color { return paint.Color{R: uint8(v), A: 255} }, nil)
							case "bool":
								l.AddProperty("value", func() bool { return v == 1 }, nil)
							}
						}}
					}
					item := bindSheet([]interface{}{obj(1), obj(2)}, &recordingOwner{}).propMap["value"]
					want := "Multiple values"
					if locale == "zh-CN" {
						want = "多值"
					}
					if item.MultiValueHint() != want {
						t.Fatalf("hint=%q", item.MultiValueHint())
					}
					c := item.Control()
					c.SetSize(110, 28)
					if !strings.Contains(gui.GetToolTip(c), want) {
						t.Fatal("full mixed hint absent from tooltip")
					}
					if typ == "bool" {
						if c.(*CheckBox).Text() != want {
							t.Fatal("boolean mixed label missing")
						}
						return
					}
					pix := paint.NewPixmap(140, 50)
					g := &propertyTextRecorder{Painter: pix.NewPainter()}
					c.(interface{ Draw(paint.Painter) }).Draw(g)
					if len(g.records) == 0 {
						t.Fatal("mixed hint not painted")
					}
					last := g.records[len(g.records)-1]
					if last.color != gui.Theme().TextColor || last.text == "" {
						t.Fatal("mixed hint uses low contrast/background color")
					}
					if gui.Theme().Font.TextExtents(last.text).Width > 110-gui.Theme().EditPadding.L-gui.Theme().EditPadding.R {
						t.Fatal("mixed hint overflows control")
					}
				})
			}
		}
	}
}

package ged

import (
	"github.com/uk0/silk/i18n"
	"strings"
	"testing"
)

func TestPaletteLocaleKeepsCategoryIdentityAndDragPayload(t *testing.T) {
	old := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocale(old) })
	for _, locale := range []string{"en", "zh-CN"} {
		i18n.SetLocale(locale)
		list := NewWidgetList()
		want := "Input"
		if locale == "zh-CN" {
			want = "输入控件"
		}
		if got := list.Item(0).Text; got != "▼ "+want {
			t.Fatalf("%s header=%q", locale, got)
		}
		if list.categories[0].name != "输入控件 (Input)" {
			t.Fatal("plugin category identity changed")
		}
		list.SetFilter("Rating")
		if list.Count() != 1 || list.Item(0).Data != "gui.Rating" {
			t.Fatal("localized palette changed filter or drag payload")
		}
		list.SetFilter("")
		list.toggleCategory(0)
		if !strings.HasPrefix(list.Item(0).Text, "▶ "+want) {
			t.Fatal("collapsed header lost label")
		}
	}
	if got := paletteCategoryLabel("acme.Process Widgets"); got != "acme.Process Widgets" {
		t.Fatalf("unknown plugin category=%q", got)
	}
}

package prop

import (
	"github.com/uk0/silk/i18n"
	"testing"
)

func TestPropertySheetLocalizedChrome(t *testing.T) {
	old := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocale(old) })
	for _, locale := range []string{"en", "zh-CN"} {
		i18n.SetLocale(locale)
		sheet := NewPropertySheet()
		want := "Filter properties..."
		layout := "Layout"
		binding := "Data Binding"
		if locale == "zh-CN" {
			want = "过滤属性..."
			layout = "布局"
			binding = "数据绑定"
		}
		if got := sheet.searchBox.Placeholder(); got != want {
			t.Fatalf("%s placeholder=%q", locale, got)
		}
		if got := i18n.T(sheet.categories["layout"].name); got != layout {
			t.Fatalf("%s category=%q", locale, got)
		}
		if got := i18n.T(sheet.categories["binding"].name); got != binding {
			t.Fatalf("%s binding=%q", locale, got)
		}
		sheet.SetFilter("Width")
		if sheet.Filter() != "Width" || sheet.searchBox.Text() != "Width" {
			t.Fatal("localization changed filter input")
		}
	}
}

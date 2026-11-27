package prop

import (
	"testing"

	"github.com/uk0/silk/i18n"
)

func TestGridPlacementLabelsAreBilingualAndKeepStableIDs(t *testing.T) {
	previous := i18n.Locale()
	t.Cleanup(func() { i18n.SetLocale(previous) })
	for _, tc := range []struct{ id, en string }{
		{"布局行", "Layout Row"},
		{"布局列", "Layout Column"},
		{"跨行数", "Row Span"},
		{"跨列数", "Column Span"},
	} {
		t.Run(tc.en, func(t *testing.T) {
			p := addTestProp(newTestSheet(), tc.id, func() int { return 3 }, nil)
			for _, locale := range []string{"en", "zh-CN"} {
				i18n.SetLocale(locale)
				want := tc.en
				if locale == "zh-CN" {
					want = tc.id
				}
				if p.Label() != want || p.Id() != tc.id {
					t.Errorf("locale %s: got ID %q, label %q; want ID %q, label %q",
						locale, p.Id(), p.Label(), tc.id, want)
				}
				if !inspectorMatchesFilter(p.Label(), p.Id(), "layout", tc.en) ||
					!inspectorMatchesFilter(p.Label(), p.Id(), "layout", tc.id) {
					t.Errorf("locale %s: can't find %q by both display languages", locale, tc.en)
				}
			}
		})
	}
}

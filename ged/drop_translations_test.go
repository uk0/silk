package ged

import (
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
)

func TestDesignerDragFeedbackUsesLocale(t *testing.T) {
	old := i18n.Default
	t.Cleanup(func() { i18n.Default = old })
	i18n.Default = i18n.NewTranslator()
	registerDropTranslations()
	for _, tt := range []struct {
		locale, move, insert, invalid, command string
	}{
		{"en", "Layout · Move to position 3", "Button → linear · Insert at position 2", "Drop inside the form", "Reorder layout"},
		{"zh-CN", "自动布局 · 移到第 3 位", "Button → linear（自动布局 · 插入第 2 位）", "请移入表单范围", "调整自动布局顺序"},
	} {
		t.Run(tt.locale, func(t *testing.T) {
			i18n.SetLocale(tt.locale)
			v, _, items := reorderLayoutFixture(t, LayoutVertical)
			x, y := layoutBodyPoint(v, items[0], LayoutVertical, .5)
			v.OnLeftDown(x, y)
			x, y = layoutBodyPoint(v, items[2], LayoutVertical, .8)
			v.OnMouseMove(x, y)
			if got := v.managedHint; !got.insertion.valid || got.hint != tt.move {
				t.Fatalf("move hint = %q; want %q", got.hint, tt.move)
			}
			v.OnPointerCancel()
			x, y = layoutBodyPoint(v, items[1], LayoutVertical, .25)
			ctx := &dropCtx{pa: gui.DndCopy, payload: "gui.Button"}
			v.OnDragMove(x, y, ctx)
			if got := v.paletteDrop; !got.insertion.valid || got.hint != tt.insert {
				t.Fatalf("insert hint = %q; want %q", got.hint, tt.insert)
			}
			x, y = v.MapFromScene(200, 150)
			v.OnDragMove(x, y, ctx)
			if got := v.paletteDrop; got.valid || got.hint != tt.invalid {
				t.Fatalf("rejection hint = %q; want %q", got.hint, tt.invalid)
			}
			if got := (&linearLayoutOrderCommand{}).Text(); got != tt.command {
				t.Fatalf("Undo label = %q; want %q", got, tt.command)
			}
		})
	}
}

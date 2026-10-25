package gui

import "testing"

func TestEditErrorFramePreservesThemeContrast(t *testing.T) {
	orig := CurrentThemeMode()
	t.Cleanup(func() { SetThemeMode(orig) })
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		for _, readonly := range []bool{false, true} {
			e := NewEdit()
			e.SetReadOnly(readonly)
			e.SetSize(160, 30)
			rec := newThemeFillRecorder()
			e.drawErrorFrame(rec)
			want := Theme().ViewBGColor
			if readonly {
				want = Theme().FormColor
			}
			if rec.fill != want || rec.fills == 0 {
				t.Errorf("mode=%v readonly=%v: error fill %v, want %v", mode, readonly, rec.fill, want)
			}
			if ratio := contrastRatio(Theme().TextColor, rec.fill); ratio < 4.5 {
				t.Errorf("mode=%v readonly=%v: contrast %.2f, want >=4.5", mode, readonly, ratio)
			}
			if rec.pen != errorFrameColor || rec.penW != 2 {
				t.Error("error border was lost")
			}
		}
	}
}

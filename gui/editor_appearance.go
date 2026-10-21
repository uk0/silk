package gui

import "github.com/uk0/silk/paint"

// EditorColors overrides only supplied colors; nil restores the theme palette.
// SetColors copies values, so callers cannot mutate cached paint state.
type EditorColors struct {
	Background, Foreground, Keyword, Comment, String *paint.Color
}

func (e *CodeEditor) SetColors(c EditorColors) {
	e.customColors = c
	for _, p := range []**paint.Color{&e.customColors.Background, &e.customColors.Foreground, &e.customColors.Keyword, &e.customColors.Comment, &e.customColors.String} {
		if *p != nil {
			v := **p
			*p = &v
		}
	}
	e.paletteRev = ^uint64(0)
	e.Self().Update()
}

func (e *CodeEditor) editorColors() *editorPalette {
	if e.customPalette != nil && e.paletteRev == ThemeRev() {
		return e.customPalette
	}
	p := *editorColorsFor(CurrentThemeMode())
	p.tokens = make(map[tokenType]paint.Color, len(p.tokens))
	for k, v := range editorColorsFor(CurrentThemeMode()).tokens {
		p.tokens[k] = v
	}
	c := e.customColors
	if c.Background != nil {
		p.bg = *c.Background
	}
	if c.Foreground != nil {
		p.tokens[tokNormal] = *c.Foreground
		p.caret = *c.Foreground
	}
	if c.Keyword != nil {
		p.tokens[tokKeyword] = *c.Keyword
	}
	if c.Comment != nil {
		p.tokens[tokComment] = *c.Comment
	}
	if c.String != nil {
		p.tokens[tokString] = *c.String
	}
	e.customPalette = &p
	e.paletteRev = ThemeRev()
	return e.customPalette
}

func (e *CodeEditor) EditorFont() paint.Font { return e.font.Dup() }

// SetUIFont invalidates theme-dependent size hints as well as painting. Font
// sizes are physical pixels in the Win32 backend; no bitmap upscaling is used.
func SetUIFont(f paint.Font) {
	if f == nil || f.Size() < 8 || f.Size() > 40 {
		return
	}
	t := Theme()
	t.Font = f.Dup()
	h := f.FontExtents().Height
	t.ItemHeight = h + 10
	t.TabBarHeight = h + 12
	themeRev++
}

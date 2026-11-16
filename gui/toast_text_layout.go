package gui

import (
	"strings"
	"unicode/utf8"

	"github.com/uk0/silk/paint"
)

// One bounded preview/layout per entry; no width map, idle worker or per-frame
// font construction. The source belongs to the toast and is cleared on dismiss.
type toastTextLayout struct {
	source  string
	width   float64
	level   ToastLevel
	theme   uint64
	family  string
	font    paint.Font
	text    string
	extents paint.TextExtents
}

func (entry *toastEntry) layoutText(width float64) *toastTextLayout {
	cache := &entry.textLayout
	family := Theme().Font.Family()
	if cache.font != nil && cache.source == entry.message && cache.width == width &&
		cache.level == entry.level && cache.theme == themeRev && cache.family == family {
		return cache
	}
	font := cache.font
	if font == nil || cache.theme != themeRev || cache.family != family {
		font = paint.NewFont(family, 13, false, false)
	}
	text := ""
	if entry.level == ToastWarning || entry.level == ToastError {
		text = ElideTextMiddle(font, notificationPreviewEnds(entry.message), width)
	} else {
		text = ElideText(font, notificationPreview(entry.message), width)
	}
	*cache = toastTextLayout{source: entry.message, width: width, level: entry.level,
		theme: themeRev, family: family, font: font, text: text, extents: *font.TextExtents(text)}
	return cache
}

// Keep at most 128 runes from each end. Scan only the bounded prefix and suffix,
// not an entire megabyte error, and normalize controls like notificationPreview.
// Crucially the old first-256 preview discarded the reason BEFORE elision.
func notificationPreviewEnds(source string) string {
	var preview strings.Builder
	n, headEnd := 0, 0
	for offset, r := range source {
		if n == 128 {
			headEnd = offset
		}
		if n >= 256 {
			tailStart := len(source)
			for count := 0; count < 128 && tailStart > 0; count++ {
				_, size := utf8.DecodeLastRuneInString(source[:tailStart])
				tailStart -= size
			}
			return notificationPreview(source[:headEnd]) + elideMark + notificationPreview(source[tailStart:])
		}
		if r < ' ' || r == '\u007f' {
			r = ' '
		}
		preview.WriteRune(r)
		n++
	}
	return preview.String()
}

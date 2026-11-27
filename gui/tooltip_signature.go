package gui

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/uk0/silk/paint"
)

const (
	signatureTipPadX   = 8.0
	signatureTipPadY   = 4.0
	maxSignatureRunes  = 4000
	maxSignatureRows   = 16
	maxSignatureRowCap = 100
)

type signatureTipSpan struct{ start, end int }

type signatureTipRow struct {
	text                 string
	advance              float64
	emphasisX, emphasisW float64
}

// Compute layout and text metrics when the hint changes, not on every frame.
type signatureTipLayout struct {
	font               paint.Font
	rows               []signatureTipRow
	ascent, lineHeight float64
	size               SizeHints
}

func layoutSignatureTip(label string, span signatureTipSpan, clientW, clientH float64) *signatureTipLayout {
	font := paint.NewFont(Theme().Font.Family(), 12, false, false)
	metrics := font.FontExtents()
	height := max(1, metrics.Height+2)
	limit := max(1, int(min(float64(maxSignatureRows), (clientH-2*signatureTipPadY)/height)))
	width := max(1, clientW-2*signatureTipPadX-4)
	normal, span := normalizeSignatureTip(label, span)
	rows := splitSignatureTip(normal, span, width, font)
	rows = clipSignatureTipRows(rows, limit, font)
	w := 0.0
	for _, r := range rows {
		w = max(w, r.advance)
	}
	return &signatureTipLayout{
		font: font, rows: rows, ascent: metrics.Ascent, lineHeight: height,
		size: SizeHints{Width: w + 2*signatureTipPadX + 4, Height: float64(len(rows))*height + 2*signatureTipPadY},
	}
}

// Rewrite whitespace once while translating byte boundaries. Never insert
// styling markup into server-controlled text.
func normalizeSignatureTip(text string, span signatureTipSpan) (string, signatureTipSpan) {
	isBoundary := func(i int) bool {
		return i >= 0 && (i == len(text) || i < len(text) && utf8.RuneStart(text[i]))
	}
	if span.start >= span.end || !isBoundary(span.start) || !isBoundary(span.end) {
		span = signatureTipSpan{}
	}
	var b strings.Builder
	dst := signatureTipSpan{-1, -1}
	count := 0
	end := 0
	for i, r := range text {
		if i == span.start {
			dst.start = b.Len()
		}
		if i == span.end {
			dst.end = b.Len()
		}
		if count == maxSignatureRunes {
			b.WriteRune('…')
			break
		}
		_, n := utf8.DecodeRuneInString(text[i:])
		end = i + n
		count++
		switch r {
		case '\t':
			b.WriteString("    ")
		case '\r':
			if end == len(text) || text[end] != '\n' {
				b.WriteByte('\n')
			}
		case '\n':
			b.WriteByte('\n')
		default:
			if unicode.IsControl(r) {
				r = ' '
			}
			b.WriteRune(r)
		}
	}
	if end == len(text) && span.end == len(text) {
		dst.end = b.Len()
	}
	if dst.start < 0 || dst.end <= dst.start {
		dst = signatureTipSpan{}
	}
	return b.String(), dst
}

func splitSignatureTip(text string, span signatureTipSpan, width float64, font paint.Font) []signatureTipRow {
	rows := make([]signatureTipRow, 0, 3)
	for base := 0; ; {
		line, next, more := strings.Cut(text[base:], "\n")
		start := base
		if line == "" {
			rows = append(rows, signatureTipRow{})
		}
		for len(line) > 0 {
			n := fitSignatureTipPrefix(line, width, font)
			part := line[:n]
			row := signatureTipRow{text: part, advance: font.TextExtents(part).XAdvance}
			left := max(0, span.start-start)
			right := min(n, span.end-start)
			if left < right {
				row.emphasisX = font.TextExtents(part[:left]).XAdvance
				row.emphasisW = font.TextExtents(part[:right]).XAdvance - row.emphasisX
			}
			rows = append(rows, row)
			line = line[n:]
			start += n
		}
		if !more {
			return rows
		}
		base = len(text) - len(next)
	}
}

func fitSignatureTipPrefix(s string, width float64, font paint.Font) int {
	ends := [maxSignatureRowCap]int{}
	n := 0
	for at := 0; n < len(ends) && at < len(s); n++ {
		_, bytes := utf8.DecodeRuneInString(s[at:])
		at += bytes
		ends[n] = at
	}
	// Always make progress under a very narrow viewport. With an adequate
	// viewport, try a soft break without measuring every prefix.
	limit := n
	if font.TextExtents(s[:ends[n-1]]).XAdvance > width {
		limit = max(1, sort.Search(n, func(i int) bool {
			return font.TextExtents(s[:ends[i]]).XAdvance > width
		}))
	}
	if ends[limit-1] < len(s) {
		for j := limit - 1; j > limit/2; j-- {
			r, _ := utf8.DecodeLastRuneInString(s[:ends[j]])
			if unicode.IsSpace(r) || r == ',' {
				return ends[j]
			}
		}
	}
	return ends[limit-1]
}

func clipSignatureTipRows(rows []signatureTipRow, limit int, font paint.Font) []signatureTipRow {
	if len(rows) <= limit {
		return rows
	}
	selected := -1
	for i, r := range rows {
		if r.emphasisW > 0 {
			selected = i
			break
		}
	}
	if limit == 1 {
		return rows[max(0, selected) : max(0, selected)+1]
	}
	dots := signatureTipRow{text: "…", advance: font.TextExtents("…").XAdvance}
	if selected < limit-1 {
		return append(rows[:limit-1:limit-1], dots)
	}
	// Keep the first visible part of the active parameter even when the
	// signature is longer than the entire client.
	start := max(0, selected-1)
	end := min(len(rows), start+limit-1)
	if selected >= end {
		start, end = selected, min(len(rows), selected+limit-1)
	}
	return append([]signatureTipRow{dots}, rows[start:end]...)
}

func (l *signatureTipLayout) draw(g paint.Painter) {
	g.SetFont(l.font)
	for i, row := range l.rows {
		top := signatureTipPadY + float64(i)*l.lineHeight
		if row.emphasisW > 0 {
			g.SetBrush1(paint.Color{R: 34, G: 82, B: 130, A: 255})
			g.Rectangle(signatureTipPadX+row.emphasisX, top, row.emphasisW, l.lineHeight-1)
			g.Fill()
		}
		g.SetBrush1(paint.Color{R: 240, G: 240, B: 240, A: 255})
		g.DrawText1(signatureTipPadX, top+l.ascent, row.text)
	}
}

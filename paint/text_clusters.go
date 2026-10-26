package paint

import (
	"math"
	"sort"
	"unicode"
	"unicode/utf8"
)

// TextCluster is an indivisible visual span; offsets count source runes.
// Logical document offsets remain code-point based, including internal marks.
type TextCluster struct{ Begin, End int }

// NeedsClusterLayout reports whether the active backend can shape a sequence
// that needs composed-character layout. ASCII and ordinary CJK retain their
// existing fast path. This is not a full bidirectional-text capability query.
func NeedsClusterLayout(text string) bool {
	if !nativeClusterLayoutAvailable {
		return false
	}
	for _, r := range text {
		if r < utf8.RuneSelf {
			continue
		}
		if unicode.IsMark(r) || r == '\u200d' || r == '\u200c' ||
			r >= 0x1f1e6 && r <= 0x1f1ff || r >= 0x1f3fb && r <= 0x1f3ff ||
			r >= 0xe0020 && r <= 0xe007f || r >= 0x1100 && r <= 0x11ff {
			return true
		}
	}
	return false
}

// WrapEnd moves a proposed row break out of a shaped cluster. An over-wide
// first cluster stays intact on its own row rather than duplicating its glyphs.
func (l TextLayout) WrapEnd(start, end int) int {
	i := sort.Search(len(l.Clusters), func(i int) bool { return l.Clusters[i].End > end })
	if i < len(l.Clusters) && l.Clusters[i].Begin < end {
		if l.Clusters[i].Begin > start {
			return l.Clusters[i].Begin
		}
		return l.Clusters[i].End
	}
	return end
}

// Boundary snaps an internal cluster offset to its leading/trailing edge.
func (l TextLayout) Boundary(pos int, trailing bool) int {
	i := sort.Search(len(l.Clusters), func(i int) bool { return l.Clusters[i].End > pos })
	if i < len(l.Clusters) && l.Clusters[i].Begin < pos {
		if trailing {
			return l.Clusters[i].End
		}
		return l.Clusters[i].Begin
	}
	return pos
}

type complexLayoutKey struct {
	sf   *scaledFont
	text string
}
type complexLayoutValue struct {
	layout  TextLayout
	extents TextExtents
	cost    int
}

// Like the existing paint font caches, these are used by the UI/render thread.
// Bound both entry count and retained text/glyph/caret bytes, not just strings.
var complexLayouts = make(map[complexLayoutKey]*complexLayoutValue)
var complexLayoutBytes int

const maxComplexLayoutBytes = 1 << 20
const maxComplexLayouts = 128

func (sf *scaledFont) complexLayout(text string) *complexLayoutValue {
	if !NeedsClusterLayout(text) {
		return nil
	}
	key := complexLayoutKey{sf, text}
	if cached, ok := complexLayouts[key]; ok {
		return cached
	}
	spans := nativeComplexSpans(text)
	if len(spans) == 0 {
		return nil
	}
	out := TextLayout{Carets: []float64{0}}
	last := 0
	for _, span := range spans {
		if span[0] > last {
			out.appendLayout(layoutSimpleText(sf, text[last:span[0]]))
		}
		part := text[span[0]:span[1]]
		cluster, ok := nativeShapeCluster(sf, part)
		if !ok {
			cluster = layoutSimpleText(sf, part)
		}
		out.appendLayout(cluster)
		last = span[1]
	}
	if last < len(text) {
		out.appendLayout(layoutSimpleText(sf, text[last:]))
	}
	value := &complexLayoutValue{layout: out}
	value.extents.XAdvance = out.Carets[len(out.Carets)-1]
	ink := false
	value.cost = len(text) + 8*len(out.Carets) + 16*len(out.Clusters)
	for _, run := range out.Runs {
		value.cost += len(run.Glyphs) * 24
		e := run.Font.ScaledFont(&sf.ctm).GlyphExtents(run.Glyphs)
		if e.Width == 0 || e.Height == 0 {
			continue
		}
		if !ink {
			value.extents.XBearing, value.extents.YBearing, value.extents.Width, value.extents.Height = e.XBearing, e.YBearing, e.Width, e.Height
			ink = true
			continue
		}
		right, bottom := math.Max(value.extents.XBearing+value.extents.Width, e.XBearing+e.Width), math.Max(value.extents.YBearing+value.extents.Height, e.YBearing+e.Height)
		value.extents.XBearing, value.extents.YBearing = math.Min(value.extents.XBearing, e.XBearing), math.Min(value.extents.YBearing, e.YBearing)
		value.extents.Width, value.extents.Height = right-value.extents.XBearing, bottom-value.extents.YBearing
	}
	if value.cost <= maxComplexLayoutBytes {
		if len(complexLayouts) >= maxComplexLayouts || complexLayoutBytes+value.cost > maxComplexLayoutBytes {
			complexLayouts = make(map[complexLayoutKey]*complexLayoutValue)
			complexLayoutBytes = 0
		}
		complexLayouts[key] = value
		complexLayoutBytes += value.cost
	}
	return value
}

func (l *TextLayout) appendLayout(next TextLayout) {
	begin := len(l.Carets) - 1
	x := l.Carets[begin]
	for _, c := range next.Carets[1:] {
		l.Carets = append(l.Carets, x+c)
	}
	for _, run := range next.Runs {
		run.Begin += begin
		run.End += begin
		for i := range run.Glyphs {
			run.Glyphs[i].X += x
		}
		l.Runs = append(l.Runs, run)
	}
	for _, cluster := range next.Clusters {
		l.Clusters = append(l.Clusters, TextCluster{begin + cluster.Begin, begin + cluster.End})
	}
}

func cloneTextLayout(src TextLayout) TextLayout {
	out := TextLayout{Carets: append([]float64(nil), src.Carets...), Clusters: append([]TextCluster(nil), src.Clusters...)}
	for _, run := range src.Runs {
		run.Font = run.Font.Dup()
		run.Glyphs = append([]Glyph(nil), run.Glyphs...)
		out.Runs = append(out.Runs, run)
	}
	return out
}

func isShapeableCluster(text string) bool {
	if utf8.RuneCountInString(text) < 2 {
		return false
	}
	for _, r := range text {
		if unicode.IsControl(r) || unicode.Is(unicode.Co, r) {
			return false
		}
	}
	return true
}

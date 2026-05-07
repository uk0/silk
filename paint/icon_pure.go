//go:build silk_no_cairo

package paint

// genMissingSubIcon returns a blank pixmap in the pure-Go build.
// The real "red cross" rendering used the Cairo Context directly;
// reproducing it via the pure-Go imagePixmap would require either
// a software rasteriser (substantial code) or a glui callback (cycle).
// A blank tile is acceptable degradation — the visual effect is
// "icon present but blank" rather than "panic" or "missing".
func genMissingSubIcon(size int) Pixmap {
	return NewPixmap(size, size)
}

//go:build !windows || silk_pure_go

package paint

// Preserve the other native backends and pure-Go font discovery unchanged.
func needsTextFallback(string) bool               { return false }
func (sf *scaledFont) fallbackFamily(rune) string { return "" }

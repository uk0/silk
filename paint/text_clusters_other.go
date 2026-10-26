//go:build !darwin || !cgo || silk_pure_go

package paint

const nativeClusterLayoutAvailable = false

func nativeComplexSpans(string) [][2]int                        { return nil }
func nativeShapeCluster(*scaledFont, string) (TextLayout, bool) { return TextLayout{}, false }

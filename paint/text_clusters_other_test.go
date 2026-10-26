//go:build !darwin || !cgo || silk_pure_go

package paint

import "testing"

func TestClusterBackendKeepsUnsupportedPath(t *testing.T) {
	if NeedsClusterLayout("🙂‍↕️e\u0301") {
		t.Fatal("backend advertised unimplemented shaping")
	}
	sf := NewFont("sans", 16, false, false).ScaledFont(nil)
	if len(LayoutText(sf, "Ae\u0301Z").Clusters) != 0 {
		t.Fatal("unsupported backend changed layout")
	}
}

package gui

import (
	"testing"

	"github.com/uk0/silk/core"
)

// TestSliderFactoryDefaultRange: a Slider built through the registered factory
// — the path the designer palette and the .silkui form loader take — must
// arrive with a usable range. NewSlider takes its bounds as arguments, so a
// factory-built slider used to come up with min == max == 0, which clamps every
// SetValue to 0 and leaves the thumb pinned.
func TestSliderFactoryDefaultRange(t *testing.T) {
	s, ok := core.New("gui.Slider").(*Slider)
	if !ok {
		t.Fatal("gui.Slider factory did not produce a *Slider")
	}
	if s.Min() != 0 || s.Max() != 100 {
		t.Errorf("factory range = [%v, %v], want [0, 100]", s.Min(), s.Max())
	}
	s.SetValue(50)
	if s.Value() != 50 {
		t.Errorf("SetValue(50) on a factory-built slider = %v, want 50", s.Value())
	}
}

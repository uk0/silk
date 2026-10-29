package glrender

import (
	"errors"
	"testing"
)

func TestInstancingInvalidOptionsRejectedBeforeContext(t *testing.T) {
	if r, err := NewCurrentWithOptions(Options{Instancing: InstancingMode(255)}); r != nil || !errors.Is(err, ErrOptions) {
		t.Fatalf("invalid options r=%v err=%v", r, err)
	}
}

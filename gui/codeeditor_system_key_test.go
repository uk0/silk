package gui

import "testing"

func TestEditorSystemKeyOptIn(t *testing.T) {
	for _, key := range []int{KeyUp, KeyDown, KeyLeft, KeyRight, KeyF4, KeySpace, 'A'} {
		for _, ctrl := range []bool{false, true} {
			for _, alt := range []bool{false, true} {
				want := ctrl && alt && (key == KeyUp || key == KeyDown)
				if got := editorSystemKey(key, ctrl, alt); got != want {
					t.Fatalf("key=%d ctrl=%v alt=%v got=%v", key, ctrl, alt, got)
				}
			}
		}
	}
	e := NewCodeEditor()
	if e.OnSystemKeyDown(KeyF4, false) || e.OnSystemKeyDown(KeySpace, false) {
		t.Fatal("native window keys consumed")
	}
}

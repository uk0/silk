package gui

import (
	"testing"
)

// TestButtonSizeHintsCacheInvalidatesOnSetText verifies the SizeHints cache
// updates when the button's text changes. A stale cache would freeze the
// button's reported width at the original-text width regardless of new text.
func TestButtonSizeHintsCacheInvalidatesOnSetText(t *testing.T) {
	b := NewButton1("A", nil)
	short := b.SizeHints().Width

	b.SetText("AAAAAAAAAAAAAAAAAAAA")
	long := b.SizeHints().Width

	if long <= short {
		t.Errorf("expected long-text width > short-text width, got short=%v long=%v", short, long)
	}

	// Restore short text and verify width contracts again.
	b.SetText("A")
	again := b.SizeHints().Width
	if again != short {
		t.Errorf("expected width to revert to %v after restoring text, got %v", short, again)
	}
}

// TestLabelSizeHintsCacheInvalidatesOnSetText mirrors the Button test for
// Label, which uses a separate cache key based on (text, font, themeRev).
func TestLabelSizeHintsCacheInvalidatesOnSetText(t *testing.T) {
	l := NewLabel("hi")
	short := l.SizeHints().Width

	l.SetText("hi everyone, this is a much longer string")
	long := l.SizeHints().Width

	if long <= short {
		t.Errorf("expected long-text width > short-text width, got short=%v long=%v", short, long)
	}

	l.SetText("hi")
	if l.SizeHints().Width != short {
		t.Errorf("expected width to revert to %v after restoring text, got %v", short, l.SizeHints().Width)
	}
}

// TestSizeHintsCacheInvalidatesOnThemeChange verifies that bumping the theme
// revision forces cached hints to recompute. The actual font may resolve to
// the same metrics in this test environment, so we verify by inspecting the
// captured themeRev rather than width values.
func TestSizeHintsCacheInvalidatesOnThemeChange(t *testing.T) {
	orig := CurrentThemeMode()
	defer SetThemeMode(orig)

	b := NewButton1("X", nil)
	_ = b.SizeHints()
	captured := b.hintThemeRev

	// Toggle to a different mode (or back, then forward) to bump themeRev.
	SetThemeMode(ThemeDark)
	SetThemeMode(ThemeLight)
	_ = b.SizeHints()
	if b.hintThemeRev == captured {
		t.Errorf("expected hintThemeRev to advance after theme changes, got %d (was %d)", b.hintThemeRev, captured)
	}

	l := NewLabel("X")
	_ = l.SizeHints()
	captured2 := l.hintThemeRev
	SetThemeMode(ThemeDark)
	_ = l.SizeHints()
	if l.hintThemeRev == captured2 {
		t.Errorf("Label hintThemeRev did not advance after theme change")
	}
}

// TestButtonSizeHintsCacheReusedOnIdenticalState verifies that repeated
// SizeHints calls on an unchanged button return the cached value without
// recomputing. This is the optimization's whole point — tested by checking
// the underlying cache flag stays valid after multiple calls.
func TestButtonSizeHintsCacheReusedOnIdenticalState(t *testing.T) {
	b := NewButton1("test", nil)
	first := b.SizeHints()
	if !b.hintsValid {
		t.Fatal("expected hintsValid=true after first SizeHints call")
	}

	// Capture mtime, call again — cache must hit.
	mtime := b.hintActionMTime
	second := b.SizeHints()
	if first != second {
		t.Errorf("cached hint mismatch: %+v vs %+v", first, second)
	}
	if b.hintActionMTime != mtime {
		t.Error("cache key changed despite no state mutation")
	}
}

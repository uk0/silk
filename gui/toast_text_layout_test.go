package gui

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/uk0/silk/paint"
)

func TestToastErrorPreviewKeepsFinalReason(t *testing.T) {
	entry := new(toastEntry)
	entry.Init(entry)
	entry.level = ToastError
	entry.message = "Save failed: atomic-write C:/" + strings.Repeat("long-project-component/", 16) + "source.txt: file is read-only"
	entry.SetBounds(0, 0, 320, 48)
	p := newElideRecorder()
	entry.Draw(p)
	if len(p.texts) != 1 || !strings.HasSuffix(p.texts[0], "file is read-only") || !strings.HasPrefix(p.texts[0], "Save") {
		t.Fatalf("toast elided the actual failure behind its path: %q", p.texts)
	}
	if !strings.HasSuffix(entry.message, "source.txt: file is read-only") {
		t.Fatal("preview changed the retained full diagnostic")
	}
}

func TestToastTextLayoutCachesAndInvalidates(t *testing.T) {
	restoreToastTestTheme(t)
	entry := new(toastEntry)
	entry.Init(entry)
	entry.level = ToastError
	entry.message = "Save failed: " + strings.Repeat("path/中文/", 100) + "source.txt: file is read-only"
	entry.SetBounds(0, 0, 320, 48)
	p := newElideRecorder()
	entry.Draw(p)
	font := &labelCountingFont{Font: entry.textLayout.font}
	entry.textLayout.font = font
	allocs := testing.AllocsPerRun(100, func() {
		p.texts = p.texts[:0]
		entry.Draw(p)
	})
	if font.measurements != 0 || allocs != 0 {
		t.Fatal("warm toast rebuilt font/preview/measurement", font.measurements, allocs)
	}
	entry.SetSize(250, 48)
	entry.Draw(p)
	if font.measurements == 0 || entry.textLayout.width != 174 {
		t.Fatal("resized toast retained a stale text budget")
	}
	entry.message = "Save failed: " + strings.Repeat("other/", 200) + "source.txt: Access is denied."
	entry.SetSize(320, 48)
	entry.Draw(p)
	if !strings.HasSuffix(entry.textLayout.text, "Access is denied.") {
		t.Fatal("new source reused old cached reason", entry.textLayout.text)
	}
	entry.level = ToastInfo
	entry.Draw(p)
	if !strings.HasSuffix(entry.textLayout.text, elideMark) {
		t.Fatal("ordinary informational toast lost legacy end-elision", entry.textLayout.text)
	}
	old := CurrentThemeMode()
	if old == ThemeDark {
		SetThemeMode(ThemeLight)
	} else {
		SetThemeMode(ThemeDark)
	}
	entry.Draw(p)
	if entry.textLayout.font == font || entry.textLayout.theme != themeRev {
		t.Fatal("theme change reused obsolete toast text layout")
	}
	owner := NewForm()
	owner.SetSize(500, 400)
	manager := &ToastManager{toasts: []*toastEntry{entry}}
	entry.manager, entry.parent = manager, owner
	manager.dismiss(entry)
	if entry.textLayout.font != nil || entry.textLayout.source != "" || entry.message != "" {
		t.Fatal("dismissed toast retained text/font cache")
	}
}

func TestToastPreviewEndsBoundedAndRuneSafe(t *testing.T) {
	for _, text := range []string{"", "short\r\nmessage", strings.Repeat("中文😀", 100000) + ": actual failure"} {
		preview := notificationPreviewEnds(text)
		if !utf8.ValidString(preview) || utf8.RuneCountInString(preview) > 257 || strings.ContainsAny(preview, "\r\n") {
			t.Fatal("unbounded or malformed preview", len(preview))
		}
		if strings.HasSuffix(text, ": actual failure") && !strings.HasSuffix(preview, ": actual failure") {
			t.Fatal("bounded preview removed the terminal diagnostic")
		}
	}
}

func TestElideTextMiddleFitsAndPreservesEnds(t *testing.T) {
	font := Theme().Font
	for _, text := range []string{"", "fitting 中文😀", strings.Repeat("C:/very-long/中文/", 30) + "file.go: readonly", strings.Repeat("e\u0301👩\u200d💻 flag 🇨🇳 ", 15)} {
		if ElideTextMiddle(nil, text, 500) != "" {
			t.Fatal("nil font accepted")
		}
		for _, width := range []float64{-1, 0, math.NaN()} {
			if ElideTextMiddle(font, text, width) != "" {
				t.Fatal("invalid width accepted")
			}
		}
		if got := ElideTextMiddle(font, text, math.Inf(1)); got != text {
			t.Fatal("unlimited width changed the source")
		}
		for _, width := range []float64{1, 15, 60, 120, 250} {
			got := ElideTextMiddle(font, text, width)
			if !utf8.ValidString(got) || font.TextExtents(got).Width > width+1e-7 {
				t.Fatal("middle elision broke UTF-8 or overflowed its budget", got, width)
			}
			if got != text && got != "" && !strings.Contains(got, elideMark) {
				t.Fatal("silent truncation without an ellipsis")
			}
		}
	}
}

func BenchmarkToastLongErrorDraw(b *testing.B) {
	entry := new(toastEntry)
	entry.Init(entry)
	entry.level = ToastError
	entry.message = "Save failed: " + strings.Repeat("deep/project/", 5000) + "source.txt: file is read-only"
	entry.SetBounds(0, 0, 320, 48)
	p := newElideRecorder()
	entry.Draw(p)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.texts = p.texts[:0]
		entry.Draw(p)
	}
}

// Unlike the fake-painter cache probe, this includes actual Cairo raster work.
// Neither benchmark is an entire-window/Windows FPS or input-latency verdict.
func BenchmarkToastLongErrorRaster(b *testing.B) {
	entry := new(toastEntry)
	entry.Init(entry)
	entry.level = ToastError
	entry.message = "Save failed: " + strings.Repeat("deep/project/", 5000) + "source.txt: file is read-only"
	entry.SetBounds(0, 0, 320, 48)
	pixmap := paint.NewPixmap(320, 48)
	painter := pixmap.NewPainter()
	entry.Draw(painter)
	b.Cleanup(func() {
		img, err := pixmap.Image()
		if err != nil {
			b.Fatal(err)
		}
		_, _, _, alpha := img.At(200, 20).RGBA()
		if alpha != 65535 {
			b.Fatal("raster benchmark did not paint an opaque toast")
		}
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.Draw(painter)
	}
}

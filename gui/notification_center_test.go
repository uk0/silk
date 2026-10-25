package gui

import (
	"github.com/uk0/silk/paint"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNotificationSnapshotDoesNotMutateControl(t *testing.T) {
	p := NewNotificationPanel()
	p.AddNotification(NotificationItem{Title: "Saved", Message: "exact message"})
	rows := p.Items()
	rows[0].Message = "unexpected external mutation"
	if p.Items()[0].Message != "exact message" {
		t.Fatal("Items exposes the internal model")
	}
}
func TestNotificationRejectsOutsideRow(t *testing.T) {
	p := NewNotificationPanel()
	p.SetSize(200, 100)
	p.AddNotification(NotificationItem{Title: "First"})
	clicks := 0
	p.SigClick(func(int) { clicks++ })
	p.OnLeftDown(10, -1)
	p.OnLeftDown(-1, 10)
	p.OnLeftDown(201, 10)
	if clicks != 0 || p.Items()[0].Read {
		t.Fatal("outside click activated notification", clicks)
	}
}

func TestNotificationLimitSelectionKeyboardAndEvents(t *testing.T) {
	p := NewNotificationPanel()
	p.SetSize(280, 100)
	p.SetLimit(3)
	changed := 0
	var selections []int
	p.SigItemsChanged(func() { changed++ })
	p.SigSelectionChanged(func(i int) { selections = append(selections, i) })
	for _, name := range []string{"a", "b", "c"} {
		p.AddNotification(NotificationItem{Title: name})
	}
	if p.UnreadCount() != 3 || p.SelectedIndex() != -1 {
		t.Fatal("initial state")
	}
	p.OnKeyDown(KeyEnd, false)
	if p.SelectedIndex() != 2 || p.UnreadCount() != 2 || p.scrollY <= 0 {
		t.Fatal("keyboard reveal")
	}
	p.AddNotification(NotificationItem{Title: "d"})
	item, ok := p.SelectedItem()
	if !ok || item.Title != "c" || p.SelectedIndex() != 1 || p.Items()[0].Title != "b" {
		t.Fatal("overflow selection")
	}
	p.RemoveNotification(0)
	if p.SelectedIndex() != 0 {
		t.Fatal("remove before selection")
	}
	p.SetLimit(1)
	if p.SelectedIndex() != -1 || p.Count() != 1 || p.Items()[0].Title != "d" {
		t.Fatal("selected row eviction")
	}
	p.OnKeyDown(KeyHome, false)
	if p.SelectedIndex() != 0 || p.UnreadCount() != 0 {
		t.Fatal("home/read")
	}
	before := changed
	p.MarkAllRead()
	p.Select(0)
	p.Select(-2)
	p.Select(9)
	if changed != before {
		t.Fatal("no-op emitted updates")
	}
	p.ClearAll()
	p.ClearAll()
	if p.Count() != 0 || p.SelectedIndex() != -1 || p.scrollY != 0 {
		t.Fatal("clear")
	}
	if len(selections) < 5 || selections[len(selections)-1] != -1 {
		t.Fatal("selection notifications", selections)
	}
}
func TestNotificationCallbacksCannotActivateRemovedRows(t *testing.T) {
	p := NewNotificationPanel()
	p.SetSize(200, 100)
	p.AddNotification(NotificationItem{Title: "a"})
	p.SigItemsChanged(func() { p.ClearAll() })
	calls := 0
	p.SigClick(func(int) { calls++ })
	p.OnLeftDown(20, 20)
	if calls != 0 || p.Count() != 0 || p.SelectedIndex() != -1 {
		t.Fatal("activated stale row")
	}
}

type notificationDrawRecord struct {
	paint.Painter
	texts []struct {
		x, y float64
		text string
	}
}

func (r *notificationDrawRecord) DrawText1(x, y float64, text string) {
	r.texts = append(r.texts, struct {
		x, y float64
		text string
	}{x, y, text})
	r.Painter.DrawText1(x, y, text)
}
func TestNotificationThemeFontAndClippedPreview(t *testing.T) {
	restoreToastTestTheme(t)
	for _, mode := range []ThemeMode{ThemeLight, ThemeDark} {
		SetThemeMode(mode)
		for _, fontSize := range []int{13, 20} {
			Theme().Font = paint.NewFont(Theme().Font.Family(), fontSize, false, false)
			p := NewNotificationPanel()
			p.SetSize(280, 145)
			p.AddNotification(NotificationItem{Title: strings.Repeat("告警", 50), Message: strings.Repeat("中🙂 long path\r\n", 3000), Time: "12:34:56", Level: NotifyWarning})
			original := p.Items()[0].Message
			p.Select(0)
			for _, scale := range []int{1, 2} {
				surface := paint.NewPixmap(280*scale, 145*scale)
				g := surface.NewPainter()
				g.Scale(float64(scale), float64(scale))
				rec := &notificationDrawRecord{Painter: g}
				p.Draw(rec)
				if len(rec.texts) != 3 {
					t.Fatal("missing visible fields", len(rec.texts))
				}
				for _, text := range rec.texts {
					if !utf8.ValidString(text.text) || strings.ContainsAny(text.text, "\r\n") || len([]rune(text.text)) > 257 {
						t.Fatal("unbounded preview")
					}
					if text.x+Theme().Font.TextExtents(text.text).Width > 272.01 {
						t.Fatal("painted beyond column", text)
					}
				}
				title, stamp := rec.texts[0], rec.texts[1]
				if title.x+Theme().Font.TextExtents(title.text).Width > stamp.x-11.99 {
					t.Fatal("title/time overlap")
				}
				paint.ClosePainter(g)
				img, err := surface.Image()
				if err != nil {
					t.Fatal(err)
				}
				got := paint.ColorModel.Convert(img.At(40*scale, 4*scale)).(paint.Color)
				if got != Theme().MenuActiveBGColor {
					t.Fatal("selection theme", got, Theme().MenuActiveBGColor)
				}
				paint.CloseSurface(surface)
			}
			if p.Items()[0].Message != original {
				t.Fatal("preview replaced full details")
			}
		}
	}
}
func BenchmarkNotificationPreviewLargeMessage(b *testing.B) {
	message := strings.Repeat("中🙂 log data\n", 100000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		notificationPreview(message)
	}
}
func BenchmarkNotificationBoundedAppend(b *testing.B) {
	p := NewNotificationPanel()
	p.SetLimit(200)
	for i := 0; i < 200; i++ {
		p.AddNotification(NotificationItem{Title: "Saved", Message: "main.go"})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.AddNotification(NotificationItem{Title: "Saved", Message: "main.go"})
	}
}

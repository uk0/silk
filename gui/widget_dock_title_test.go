package gui

import "testing"

func TestDockTitleOverridesWithoutChangingContentTitle(t *testing.T) {
	w := NewTabWidget()
	w.SetTitle("source title")
	tab := &_Tab{data: w}
	w.SetDockTitle("Files")
	if tab.Text() != "Files" || w.Title() != "source title" {
		t.Fatalf("dock=%q source=%q", tab.Text(), w.Title())
	}
	w.SetDockTitle("文件")
	if tab.Text() != "文件" {
		t.Fatal("caption cache not refreshed")
	}
	w.SetDockTitle("")
	if tab.Text() != "source title" {
		t.Fatal("caption fallback lost")
	}
}

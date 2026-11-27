//go:build linux && !wayland

package gui

import "testing"

func TestFileDialogPortalOptions(t *testing.T) {
	save := linuxFilePortalOptions(true, "pdf")
	if !save.Save || save.Title != "Save PDF File" || save.SuggestName != "document.pdf" ||
		len(save.Filters) != 2 || save.Filters[0].Patterns[0].Value != "*.pdf" {
		t.Fatalf("PDF save request: %+v", save)
	}
	open := linuxFilePortalOptions(false, "")
	if open.Save || len(open.Filters) != 3 || open.Filters[0].Patterns[0].Value != "*.silkui" ||
		len(open.Filters[1].Patterns) != 3 {
		t.Fatalf("SilkUI open request: %+v", open)
	}
}

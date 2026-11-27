//go:build linux && !wayland

package gui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/internal/fileportal"
)

var filePortalActive bool // UI thread only

func desktopFilePortal(save bool, ext string) (string, bool) {
	if filePortalActive || shouldQuit {
		return "", true
	}
	owner := linuxFilePortalOwner()
	if owner == nil {
		return "", false
	}
	window := owner.glfwWin
	xid := window.GetX11Window()
	if xid == 0 {
		return "", false
	}
	parent := fmt.Sprintf("x11:%x", uint64(xid))
	options := linuxFilePortalOptions(save, ext)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	filePortalActive = true
	defer func() { filePortalActive = false }()
	owner.dismissOwnedPopups()
	enable := disableForExternalFileDialog()
	defer enable()

	type choice struct {
		path    string
		handled bool
		err     error
	}
	done := make(chan choice, 1)
	go func() {
		path, handled, err := fileportal.Choose(ctx, parent, options)
		done <- choice{path, handled, err}
	}()

	for {
		if shouldQuit || owner.glfwWin != window || !owner.IsVisible() {
			// The background request uses no widget state; cancel closes only
			// this dialog and the worker can finish after the owner retires.
			return "", true
		}
		select {
		case c := <-done:
			if c.handled && c.err != nil && !errors.Is(c.err, context.Canceled) {
				core.Warn("desktop file dialog: ", c.err)
			}
			return c.path, c.handled
		default:
			nativeDialogFrame()
		}
	}
}

func linuxFilePortalOwner() *Window {
	var focused *Window
	if focusWidget != nil {
		focused = focusWidget.OwnerWindow()
	}
	candidates := []*Window{focused, activeForm, pointerWindow}
	for _, w := range winMap {
		candidates = append(candidates, w)
	}
	for _, w := range candidates {
		for w != nil && w.wt == WtPopup && w.popupOwner != w {
			w = w.popupOwner
		}
		if w != nil && w.wt == WtForm && w.glfwWin != nil && w.IsVisible() {
			return w
		}
	}
	return nil
}

func linuxFilePortalOptions(save bool, ext string) fileportal.Options {
	op := "Open"
	if save {
		op = "Save"
	}
	if ext == "" {
		filters := []fileportal.Filter{
			{Name: "SilkUI Files (*.silkui)", Patterns: []fileportal.Pattern{{Value: "*.silkui"}}},
		}
		if !save {
			filters = append(filters, fileportal.Filter{
				Name: "Legacy Design Files", Patterns: []fileportal.Pattern{
					{Value: "*.cml"}, {Value: "*.silk"}, {Value: "*.form"},
				},
			})
		}
		return fileportal.Options{
			Save: save, Title: op + " SilkUI File", SuggestName: "design.silkui",
			Filters: append(filters, fileportal.Filter{Name: "All Files", Patterns: []fileportal.Pattern{{Value: "*"}}}),
		}
	}
	label := strings.ToUpper(ext)
	return fileportal.Options{
		Save: save, Title: op + " " + label + " File", SuggestName: "document." + ext,
		Filters: []fileportal.Filter{
			{Name: label + " Files (*." + ext + ")", Patterns: []fileportal.Pattern{{Value: "*." + ext}}},
			{Name: "All Files", Patterns: []fileportal.Pattern{{Value: "*"}}},
		},
	}
}

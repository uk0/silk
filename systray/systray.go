// Package systray is a thin wrapper over fyne.io/systray that places the
// application in the OS system tray with clickable menu items, giving the
// desktop build parity with native apps (for example, surfacing alarms from a
// tray menu).
//
// fyne.io/systray exposes a package-level, singleton API; Tray collects the
// ready hook and menu items behind a small object so a host can configure the
// tray, then drive its event loop with Run.
package systray

import fynesystray "fyne.io/systray"

// Tray wraps the fyne.io/systray package-level API. The zero value is not
// usable; construct one with New.
type Tray struct {
	onReady func()
}

// New returns a Tray with no ready hook or menu items registered yet.
func New() *Tray {
	return &Tray{}
}

// OnReady registers a callback invoked once, when the tray is initialised and
// ready. Icon and menu-item calls should be made from within it (or after it
// has fired).
func (t *Tray) OnReady(fn func()) {
	t.onReady = fn
}

// SetIcon sets the tray icon from raw image bytes (PNG on macOS/Linux, ICO on
// Windows). Call it from the OnReady callback, once the tray exists.
func (t *Tray) SetIcon(icon []byte) {
	fynesystray.SetIcon(icon)
}

// AddItem adds a clickable menu item. onClick is invoked every time the item is
// selected, on a goroutine that ranges over the item's click channel; the
// goroutine exits when the tray shuts down and the channel is closed.
func (t *Tray) AddItem(title, tooltip string, onClick func()) {
	item := fynesystray.AddMenuItem(title, tooltip)
	go func() {
		for range item.ClickedCh {
			onClick()
		}
	}()
}

// Run starts the tray event loop and blocks until Quit is called, so hosts
// typically invoke it on the main goroutine.
func (t *Tray) Run() {
	fynesystray.Run(t.onReady, nil)
}

// Quit stops the tray and unblocks Run.
func (t *Tray) Quit() {
	fynesystray.Quit()
}

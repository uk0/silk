package main

import (
	"os"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/scada"
)

// previewController hosts the IDE's live preview: it runs the screen currently
// on the design canvas against a throwaway, in-memory scada.Services so an
// operator screen can be exercised end to end — tags animating, alarms firing
// into the panels — without a real field device behind it. Exactly one preview
// runs at a time; starting another (Ctrl+R) tears the previous one down first,
// so the historian / alarm / stat goroutines a Services spins up in Start never
// pile up across runs.
//
// Preview safety is the whole point of the throwaway Services: it always comes
// up with EnableDrivers off (so BindScreen never dials a PLC) and a no-op
// Notifier (so a designed alarm can't raise a desktop notification while the
// user is only previewing).
type previewController struct {
	services *scada.Services // backend the live screen is bound to (nil when idle)
	stop     func()          // scada.BindScreen unwind for the live screen (nil when idle)
	frame    *gui.Frame      // window hosting the generated form (nil when idle)
	tmpDir   string          // temp dir holding the preview's throwaway SQLite stores (empty when idle)
}

func newPreviewController() *previewController {
	return &previewController{}
}

// preview runs scene's design against a fresh in-memory Services and shows it in
// its own window. Any previous preview is torn down first (stopPreview), so
// re-triggering leaks nothing. It runs on the UI thread — it is invoked from the
// design canvas's Ctrl+R handler — while BindScreen marshals its own async tag
// updates back onto the UI thread with gui.Post.
func (pc *previewController) preview(scene *ged.GedScene) {
	if scene == nil {
		return
	}
	// Deterministic teardown of any prior preview before a new one starts.
	pc.stopPreview()

	design := scene.Generate()
	if design == nil {
		return
	}
	form := design.Form()
	if form == nil {
		return
	}

	dir, err := os.MkdirTemp("", "silkide-preview-")
	if err != nil {
		core.Warn("preview: temp store: ", err)
		return
	}

	services, err := scada.New(previewConfig(dir))
	if err != nil {
		os.RemoveAll(dir)
		core.Warn("preview: scada.New: ", err)
		return
	}

	// EnableDrivers off: a preview binds tag setters and operator panels but
	// never starts a DeviceComponent, so it can never open a device socket.
	stop, err := scada.BindScreen(services, form, scada.ScreenOptions{EnableDrivers: false})
	if err != nil {
		services.Stop()
		os.RemoveAll(dir)
		core.Warn("preview: BindScreen: ", err)
		return
	}
	if err := services.Start(); err != nil {
		stop()
		services.Stop()
		os.RemoveAll(dir)
		core.Warn("preview: Services.Start: ", err)
		return
	}

	// Publish state only once the backend is fully up, so stopPreview never sees
	// a half-built preview.
	pc.services = services
	pc.stop = stop
	pc.tmpDir = dir

	pc.showForm(scene.FormTitle(), form)
}

// showForm hosts the generated form in a dedicated preview window whose close
// releases the preview (SetClosedCallback -> stopPreview). It is kept apart from
// the backend wiring in preview so that lifecycle logic stays free of window
// handling. Must run on the UI thread.
func (pc *previewController) showForm(title string, form *gui.Form) {
	frame := gui.NewFrameWindow()
	frame.SetTitle("Preview — " + title)
	frame.SetClosedCallback(func(*gui.Frame) {
		// This callback fires because the window is already closing; forget the
		// frame first so the stopPreview below doesn't try to re-close it, then
		// release the backend and temp stores.
		pc.frame = nil
		pc.stopPreview()
	})
	if dock, ok := frame.SuggestDocDock().(*gui.Dock); ok && dock != nil {
		dock.AddView(form)
	}
	pc.frame = frame

	// Match the standalone designer's preview: give the window a sensible floor
	// so a tiny form still opens with a usable title bar and borders.
	w, h := form.Size()
	if w < 320 {
		w = 320
	}
	if h < 240 {
		h = 240
	}
	if win := frame.Window(); win != nil {
		win.SetSize(w, h)
		win.MoveToCenter()
	}
	frame.Show()
}

// stopPreview tears the current preview down: it unwires the live screen, stops
// the backend (which closes its stores and stops its goroutines), closes the
// preview window, and deletes the throwaway stores. It is idempotent — a second
// call, or a call racing the window's own close callback, is a no-op — so the
// re-preview path and the user closing the window both land here safely. Each
// field is cleared as it is released, so nothing is torn down twice.
func (pc *previewController) stopPreview() {
	if pc.stop != nil {
		pc.stop()
		pc.stop = nil
	}
	if pc.services != nil {
		pc.services.Stop()
		pc.services = nil
	}
	if pc.frame != nil {
		f := pc.frame
		pc.frame = nil
		// Window().Close() runs the frame's own close (firing the closed callback,
		// which finds pc.frame already nil and does not recurse) and destroys the
		// GLFW window; frame.Close() alone would leave the window open.
		if win := f.Window(); win != nil {
			win.Close()
		}
	}
	if pc.tmpDir != "" {
		os.RemoveAll(pc.tmpDir)
		pc.tmpDir = ""
	}
}

// previewConfig builds a throwaway scada.Config whose three SQLite stores live
// under dir (a temp dir the caller deletes on teardown) so a preview never
// touches the project's real history / events / recipes, and whose Notifier is a
// silent no-op so a previewed alarm can't fire a desktop notification. Every
// other field follows scada.DefaultConfig.
func previewConfig(dir string) scada.Config {
	cfg := scada.DefaultConfig(dir)
	cfg.Notifier = func(title, body string) error { return nil }
	return cfg
}

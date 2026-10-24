package gui

import "testing"

func lifecycleDock(t *testing.T) (*Frame, *Dock, *Dock) {
	t.Helper()
	f := NewFrame()
	main := f.SuggestDocDock().(*Dock)
	main.AddView(NewLabel("document"))
	side := main.SplitNewDock(true, false).(*Dock)
	side.AddView(NewLabel("tool"))
	f.SetBounds(0, 0, 1000, 700)
	f.Layout()
	return f, main, side
}

func TestDockMinimizeSurvivesLayout(t *testing.T) {
	f, main, side := lifecycleDock(t)
	before := main.Width()
	side.toggleMinimize()
	f.Layout()
	if side.IsVisible() || main.Width() <= before {
		t.Fatal("minimized panel is visible and still occupies document space")
	}
	side.toggleMinimize()
	if !side.IsVisible() || main.Width() != before {
		t.Fatal("restore lost the original split")
	}
}

func TestDockMaximizeSurvivesLayout(t *testing.T) {
	f, main, side := lifecycleDock(t)
	before := side.Bounds1()
	side.toggleMaximize()
	f.Layout()
	if main.IsVisible() || side.Bounds1() == before {
		t.Fatal("maximized dock overlaps siblings or loses its layout")
	}
	side.toggleMaximize()
	if !main.IsVisible() || side.Bounds1() != before {
		t.Fatal("restore did not preserve the split")
	}
}

func TestFrameUnknownToolIsNotSuccessful(t *testing.T) {
	if NewFrame().ShowToolView("missing.lifecycle.tool") {
		t.Fatal("unknown tool reported success")
	}
}

type lifecycleView struct {
	Widget
	closes int
	title  string
}

func newLifecycleView() *lifecycleView { p := &lifecycleView{title: "retained"}; p.Init(p); return p }
func (p *lifecycleView) Title() string { return p.title }
func (p *lifecycleView) Close()        { p.closes++ }

func TestDockRetainedToolLifecycle(t *testing.T) {
	f, main, side := lifecycleDock(t)
	side.CloseAllViews()
	side = main.SplitNewDock(true, false).(*Dock)
	side.SetRetainClosedViews(true)
	view := newLifecycleView()
	side.AddView(view)
	f.toolViews["lifecycle"] = &_ToolViewInfo{ToolViewDef: ToolViewDef{Id: "lifecycle"}, Widget: view, Action: NewAction()}
	f.Layout()
	before := main.Width()
	for i := 0; i < 25; i++ {
		if !f.CloseToolView("lifecycle") || !f.CloseToolView("lifecycle") {
			t.Fatal("close is not idempotent")
		}
		if view.closes != 0 || view.Parent() != side || side.Frame() != f || side.ViewCount() != 0 || len(side.AvailableViews()) != 1 {
			t.Fatal("hide destroyed the configured tool or detached the dock")
		}
		if f.IsToolViewVisible("lifecycle") || side.IsVisible() || main.Width() <= before {
			t.Fatal("hidden panel occupies space")
		}
		if !f.ShowToolView("lifecycle") || !f.IsToolViewVisible("lifecycle") || side.ActiveView() != view || main.Width() != before {
			t.Fatal("show did not restore original instance and split")
		}
	}
	side.HideView(view)
	f.CloseAllViews()
	if view.closes != 1 || view.Parent() != nil || side.Frame() != nil || f.ToolViewById("lifecycle") != nil || len(side.AvailableViews()) != 0 {
		t.Fatal("frame teardown leaked a retained tool")
	}
}

func TestDockCloseAndRemoveHiddenViews(t *testing.T) {
	for _, operation := range []string{"close", "remove"} {
		t.Run(operation, func(t *testing.T) {
			_, _, side := lifecycleDock(t)
			side.SetRetainClosedViews(true)
			view := newLifecycleView()
			side.AddView(view)
			side.HideView(view)
			if operation == "close" {
				if !side.CloseView(view) {
					t.Fatal("close failed")
				}
			} else {
				side.RemoveView(view)
			}
			expected := 0
			if operation == "close" {
				expected = 1
			}
			if side.HasView(view) || view.Parent() != nil || view.closes != expected || side.RevealView(view) {
				t.Fatal("hidden removal ownership broken")
			}
		})
	}
}

func TestDockDocumentsStillDispose(t *testing.T) {
	_, main, side := lifecycleDock(t)
	for _, dock := range []*Dock{main, side} {
		// Main documents must not acquire tool-hide behavior, even if opted in.
		dock.SetRetainClosedViews(dock == main)
		view := newLifecycleView()
		dock.AddView(view)
		if !dock.PromptSaveCloseView(view) || view.closes != 1 || dock.HasView(view) || view.Parent() != nil {
			t.Fatal("document close was intercepted")
		}
	}
}

func TestDockMaximizeKeepsHiddenDocksAndResizes(t *testing.T) {
	f, main, side := lifecycleDock(t)
	bottom := main.SplitNewDock(false, true).(*Dock)
	bottom.AddView(NewLabel("bottom"))
	bottom.SetMinimized(true)
	side.SetMaximized(true)
	f.SetBounds(0, 0, 800, 500)
	f.Layout()
	if side.Width() != f.Width() || bottom.IsVisible() || main.IsVisible() {
		t.Fatal("maximized resize overlaps other panels")
	}
	// The old splitter must not become a hidden hit target inside the full panel.
	for _, x := range []float64{100, 400, 600} {
		if f.RootBrick().FindSplitter(x, 200) != nil {
			t.Fatal("invisible splitter intercepted content")
		}
	}
	side.SetMaximized(false)
	if !main.IsVisible() || bottom.IsVisible() || !bottom.IsMinimized() {
		t.Fatal("restore changed user visibility")
	}
	side.SetMaximized(true)
	if !main.RevealView(main.ActiveView()) || side.IsMaximized() {
		t.Fatal("explicit document reveal left it covered")
	}
}

func TestDockMinimizeRestoresViaRevealAndClearsMaximize(t *testing.T) {
	f, main, side := lifecycleDock(t)
	side.SetMaximized(true)
	side.SetMinimized(true)
	if side.IsMaximized() || !side.IsMinimized() || !main.IsVisible() {
		t.Fatal("minimized maximized panel stranded workspace")
	}
	f.Layout()
	if !side.RevealView(side.ActiveView()) || side.IsMinimized() || !side.IsVisible() {
		t.Fatal("reveal did not unminimize")
	}
	side.SetMaximized(true)
	side.CloseAllViews()
	if f.maximizedDock != nil || !main.IsVisible() || side.Frame() != nil {
		t.Fatal("closed maximized dock retained by frame")
	}
}

func TestDockDetachedAndInvalidOperations(t *testing.T) {
	d := NewDock()
	view := newLifecycleView()
	d.AddView(view)
	if d.RemoveIndex(-1) != nil || d.RemoveIndex(1) != nil || d.PromptSaveCloseIndex(-1) || d.RevealView(NewLabel("foreign")) {
		t.Fatal("invalid index/view accepted")
	}
	d.AddView(view)
	if d.ViewCount() != 1 {
		t.Fatal("duplicate view")
	}
	if !d.CloseView(view) || view.closes != 1 {
		t.Fatal("detached dock lifecycle failed")
	}
}

package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
)

type contentPlacementMode uint8

const (
	contentLegacy contentPlacementMode = iota
	contentFree
	contentManaged
)

type contentGeometry interface {
	ContentBounds() geom.Rect
}

func (f *FakeWidget) supportsContentLayout() bool {
	_, geometry := f.widget.(contentGeometry)
	return geometry && (f.factoryName == "gui.Card" || f.factoryName == "gui.GroupBox")
}

func (f *FakeWidget) activeContentLayout() *FakeWidget {
	if f.contentPlacement != contentManaged || !f.supportsContentLayout() {
		return nil
	}
	return contentLayoutIn(f.Children())
}

func contentLayoutIn(children []graph.IItem) *FakeWidget {
	if len(children) != 1 {
		return nil
	}
	return managedLayoutTarget(children[0])
}

func isOwnedContentRoot(item graph.IItem) bool {
	if item == nil {
		return false
	}
	owner, ok := item.Parent().(*FakeWidget)
	return ok && owner.activeContentLayout() == item
}

func contentDropTarget(item graph.IItem) graph.IItem {
	if owner, ok := item.(*FakeWidget); ok {
		if root := owner.activeContentLayout(); root != nil {
			return root
		}
	}
	return item
}

func (f *FakeWidget) syncContentLayout() {
	box := f.activeContentLayout()
	if box == nil {
		return
	}
	client := f.widget.(contentGeometry).ContentBounds()
	r := geom.Rect{
		X: gui.PixelToMm(client.X), Y: gui.PixelToMm(client.Y),
		Width: gui.PixelToMm(client.Width), Height: gui.PixelToMm(client.Height),
	}
	if !f.HasLocalCoord() {
		r.X, r.Y = r.X+f.X(), r.Y+f.Y()
	}
	if box.Bounds1() != r {
		box.SetBounds1(r)
	} else {
		box.Layout()
	}
}

func (v *GedView) selectedContentHost() *FakeWidget {
	items := v.Selection().ItemList()
	if len(items) != 1 {
		return nil
	}
	f, ok := items[0].(*FakeWidget)
	if !ok || f.Scene() != v.Scene() || !f.supportsContentLayout() ||
		f.activeContentLayout() != nil || f.IsLockPos() || f.IsLockSize() || !layoutChildrenUnlocked(f) {
		return nil
	}
	return f
}

func (v *GedView) applyContentLayout(owner *FakeWidget, kind LayoutKind) bool {
	s := v.GedScene()
	if err := s.CommitPendingEdits(); err != nil {
		core.Warn(err)
		return false
	}
	if v.selectedContentHost() != owner {
		return false
	}
	cmd := v.prepareOwnedRootLayout(owner, kind, "Lay Out Container")
	if cmd == nil {
		return false
	}
	prior := owner.contentPlacement
	cmd.bind = func(applied bool) {
		owner.contentPlacement = prior
		if applied {
			owner.contentPlacement = contentManaged
			owner.Layout()
		}
	}
	s.PushCommand(cmd)
	return true
}

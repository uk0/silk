package main

import (
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"testing"
)

type context struct{ action gui.DndAction }

func (c *context) Formats() []string             { return []string{"text/plain"} }
func (c *context) HasFormat(f string) bool       { return f == "text/plain" }
func (c *context) Data(f string) interface{}     { return "Silk drag payload" }
func (c *context) PosibleActions() gui.DndAction { return gui.DndCopy | gui.DndMove | gui.DndLink }
func (c *context) Action() gui.DndAction         { return c.action }
func (c *context) SetAction(a gui.DndAction)     { c.action = a }
func (c *context) From() interface{}             { return nil }
func TestDragDemoActionsAndPayload(t *testing.T) {
	f := newDragDemo()
	defer f.closeAssets()
	c := new(context)
	for i, target := range f.targets {
		if target.icon.Pixmap == nil {
			t.Fatal("missing feedback artwork")
		}
		target.OnDragEnter(10, 10, c)
		target.OnDrop(10, 10, c)
		if f.drops != i {
			t.Fatal("invalid drop count", i, f.drops)
		}
		target.OnDragLeave()
		if target.over {
			t.Fatal("stale highlight")
		}
	}
	if f.lastAction != gui.DndLink || f.lastData != "Silk drag payload" {
		t.Fatal("link payload lost")
	}
}

func TestDragDemoLayoutRemainsInsideOwner(t *testing.T) {
	f := newDragDemo()
	defer f.closeAssets()
	for _, size := range [][2]float64{{760, 370}, {320, 240}, {80, 60}, {0, 0}} {
		f.SetSize(size[0], size[1])
		f.OnResize()
		items := []gui.IWidget{f.source, f.status, f.snapshot}
		for _, target := range f.targets {
			items = append(items, target)
		}
		for _, item := range items {
			x, y, w, h := item.Bounds()
			if x < 0 || y < 0 || w < 0 || h < 0 || x+w > size[0]+0.01 || y+h > size[1]+0.01 {
				t.Fatalf("size %v child outside owner: %v", size, []float64{x, y, w, h})
			}
		}
	}
}

func TestDragDemoLabelsHaveVisibleInk(t *testing.T) {
	f := newDragDemo()
	defer f.closeAssets()
	f.SetSize(760, 370)
	f.OnResize()
	pix := paint.NewPixmap(720, 105)
	defer paint.CloseSurface(pix)
	painter := pix.NewPainter()
	f.source.Draw(painter)
	paint.ClosePainter(painter)
	img, err := pix.Image()
	if err != nil {
		t.Fatal(err)
	}
	dark := 0
	for y := 50; y < 90; y++ {
		for x := 12; x < 250; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0 && r < 32768 && g < 32768 && b < 32768 {
				dark++
			}
		}
	}
	if dark < 20 {
		t.Fatal("drag title is invisible against its tile", dark)
	}
}

package main

import (
	"context"
	"testing"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/httpx"
)

func TestDemoDataAndCompactLayout(t *testing.T) {
	url, stop, err := demoServer()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	r, err := httpx.Default().Get(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	d, err := r.Document()
	if err != nil {
		t.Fatal(err)
	}
	if n, err := d.Int("/data/items/1999/id"); err != nil || n != 1999 {
		t.Fatal(n, err)
	}
	if s, err := d.Str("/data/name"); err != nil || s != "Silk 中文😀" {
		t.Fatal(s, err)
	}
	f := newInspectorForm(url)
	defer f.dispose()
	for _, size := range [][2]float64{{840, 600}, {400, 350}, {180, 160}} {
		f.SetSize(size[0], size[1])
		f.OnResize()
		for _, w := range []gui.IWidget{f.url, f.path, f.fetch, f.locate, f.copyPath, f.copyValue, f.status, f.viewer} {
			x, y, width, height := w.Bounds()
			if x < 0 || y < 0 || x+width > size[0] || y+height > size[1] {
				t.Fatal("control outside bounds", size, x, y, width, height)
			}
		}
	}
}
